// Package app coordina todo: consentimiento, vinculación, detección de WoW, addon y la subida de
// los registros de combate de bandas cada pocos minutos.
package app

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/mgaliano-lab/rebirth-track-uploader/internal/addon"
	"github.com/mgaliano-lab/rebirth-track-uploader/internal/api"
	"github.com/mgaliano-lab/rebirth-track-uploader/internal/logs"
	"github.com/mgaliano-lab/rebirth-track-uploader/internal/state"
	"github.com/mgaliano-lab/rebirth-track-uploader/internal/update"
	"github.com/mgaliano-lab/rebirth-track-uploader/internal/wow"
)

// Qué se sube por defecto: bandas y mazmorras (el sitio manda las listas vigentes en
// /api/uploader/config; ver shared/instances.ts del sitio).
var (
	defaultRaid    = []int{3, 4, 5, 6, 7, 9, 14, 15, 16, 17, 33, 148, 151, 175, 176, 185, 186, 193, 194, 215}
	defaultDungeon = []int{1, 2, 8, 23, 24, 150, 173, 174, 201, 205}
	defaultMaps    = []int{
		249, 309, 409, 469, 509, 531, 533, 532, 534, 544, 548, 550, 564, 565, 568, 580, // bandas Classic/TBC
		33, 34, 36, 43, 47, 48, 70, 90, 109, 129, 189, 209, 229, 230, 289, 329, 349, 389, 429, // mazmorras Classic
		269, 540, 542, 543, 545, 546, 547, 552, 553, 554, 555, 556, 557, 558, 560, 585, // mazmorras TBC
	}
)

const defaultAddonURL = "https://ast.rebirthknights.fun/addon/latest.json"

type Activity struct {
	At   time.Time `json:"at"`
	Kind string    `json:"kind"` // info | upload | error
	Text string    `json:"text"`
}

type Linking struct {
	UserCode  string    `json:"userCode"`
	VerifyURL string    `json:"verifyUrl"`
	ExpiresAt time.Time `json:"expiresAt"`
}

type App struct {
	Version   string
	BaseURL   string
	Store     *state.Store
	OpenURL   func(string)
	mu        sync.Mutex
	activity  []Activity
	linking   *Linking
	cfg       *api.Config
	latest    *addon.Release
	lastRun   time.Time
	running   bool
	uploaded  int64
	batches   int
	wake      chan struct{}
	appLatest *update.Release
	updating  bool
	Quit      func()          // cierra la app (la pone main); se usa al actualizarse
	imports   map[string]bool // archivos viejos elegidos para importar; se aplican al empezar el ciclo
	scans     map[string]scanCache
}

func New(version, baseURL string, st *state.Store, open func(string)) *App {
	return &App{Version: version, BaseURL: strings.TrimRight(baseURL, "/"), Store: st, OpenURL: open, wake: make(chan struct{}, 1)}
}

func (a *App) log(kind, format string, args ...any) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.activity = append(a.activity, Activity{At: time.Now(), Kind: kind, Text: fmt.Sprintf(format, args...)})
	if len(a.activity) > 200 {
		a.activity = a.activity[len(a.activity)-200:]
	}
}

func (a *App) client() *api.Client { return api.New(a.BaseURL, a.Store.Read().Token, a.Version) }

// Límites de la frecuencia de subida que se puede elegir en el panel.
const (
	MinEvery = time.Minute
	MaxEvery = 24 * time.Hour
)

func (a *App) batchEvery() time.Duration {
	if s := a.Store.Read().Every; s > 0 {
		return time.Duration(s) * time.Second
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.cfg != nil && a.cfg.BatchSeconds >= 30 {
		return time.Duration(a.cfg.BatchSeconds) * time.Second
	}
	return 3 * time.Minute
}

func (a *App) instanceSet() logs.Instances {
	a.mu.Lock()
	raid, dungeon, maps := defaultRaid, defaultDungeon, defaultMaps
	if a.cfg != nil && len(a.cfg.RaidDifficulties) > 0 {
		raid = a.cfg.RaidDifficulties
	}
	if a.cfg != nil && len(a.cfg.DungeonDifficulties) > 0 {
		dungeon = a.cfg.DungeonDifficulties
	}
	if a.cfg != nil && len(a.cfg.InstanceMaps) > 0 {
		maps = a.cfg.InstanceMaps
	}
	a.mu.Unlock()
	in := logs.Instances{Difficulties: map[int]bool{}, Maps: map[int]bool{}}
	for _, d := range append(append([]int{}, raid...), dungeon...) {
		in.Difficulties[d] = true
	}
	for _, m := range maps {
		in.Maps[m] = true
	}
	return in
}

// Run es el bucle principal: sube cada batchEvery y revisa el addon y la app cada 6 horas.
func (a *App) Run(ctx context.Context) {
	a.log("info", "Rebirth Track Uploader %s iniciado", a.Version)
	a.refreshConfig()
	a.CheckAddon(false)
	go a.CheckApp(false)
	tick := time.NewTicker(10 * time.Second)
	defer tick.Stop()
	lastAddon := time.Now()
	for {
		select {
		case <-ctx.Done():
			a.Cycle() // último lote antes de cerrar
			return
		case <-a.wake:
			a.Cycle()
		case <-tick.C:
			if time.Since(a.lastRun) >= a.batchEvery() {
				a.Cycle()
			}
			if time.Since(lastAddon) > 6*time.Hour {
				lastAddon = time.Now()
				a.CheckAddon(false)
				go a.CheckApp(false)
			}
		}
	}
}

// UploadNow fuerza un ciclo (botón "Subir ahora").
func (a *App) UploadNow() {
	select {
	case a.wake <- struct{}{}:
	default:
	}
}

func (a *App) refreshConfig() {
	if a.Store.Read().Token == "" {
		return
	}
	cfg, err := a.client().Config()
	if errors.Is(err, api.ErrUnauthorized) {
		a.unlinked()
		return
	}
	if err != nil {
		a.log("error", "No pude hablar con Rebirth Track: %v", err)
		return
	}
	a.mu.Lock()
	a.cfg = &cfg
	a.mu.Unlock()
	a.Store.Update(func(s *state.State) { s.UserName = cfg.UserName; s.DeviceName = cfg.DeviceName })
}

func (a *App) unlinked() {
	a.Store.Update(func(s *state.State) { s.Token = "" })
	a.log("error", "Esta PC quedó desvinculada de Rebirth Track. Volvé a vincularla para seguir subiendo.")
}

func (a *App) Installs() (string, []wow.Install) {
	return wow.Detect(a.Store.Read().WowRoot)
}

// Cycle lee lo nuevo de cada log y sube los tramos de banda.
func (a *App) Cycle() {
	a.mu.Lock()
	if a.running {
		a.mu.Unlock()
		return
	}
	a.running = true
	a.mu.Unlock()
	defer func() {
		a.mu.Lock()
		a.running = false
		a.lastRun = time.Now()
		a.mu.Unlock()
	}()

	s := a.Store.Read()
	if s.ConsentAt == nil || s.Token == "" || s.Paused {
		return
	}
	a.applyImports()
	if a.cfg == nil {
		a.refreshConfig()
	}
	raid := a.instanceSet()
	_, installs := a.Installs()
	for _, in := range installs {
		if s.Disabled[in.Flavor] {
			continue
		}
		files, err := logs.List(in.LogsDir())
		if err != nil {
			continue // sin carpeta Logs todavía
		}
		for _, f := range files {
			if err := a.processFile(in, f, *s.ConsentAt, raid); err != nil {
				if errors.Is(err, api.ErrUnauthorized) {
					a.unlinked()
					return
				}
				a.log("error", "%s · %s: %v (se reintenta en el próximo ciclo)", in.Label, f.Name, err)
			}
		}
	}
}

func (a *App) processFile(in wow.Install, f logs.File, consentAt time.Time, raid logs.Instances) error {
	key := in.Flavor + "/" + f.Name
	p := a.Store.File(key)
	isNew := p.Offset == 0 && p.Seq == 0 && p.SessionID == 0
	if isNew {
		if _, known := a.Store.Read().Files[key]; !known && f.ModTime.Before(consentAt) {
			// Log viejo (anterior al consentimiento): solo se sube lo que se escriba de acá en adelante
			// (salvo que el usuario lo importe desde el panel).
			p.Offset, p.Skipped = f.Size, true
		}
	}
	if f.Size < p.Offset { // el archivo se truncó o se reemplazó
		p.Offset, p.InRaid, p.Zone = 0, false, ""
	}
	c := a.client()
	for i := 0; i < 20 && f.Size > p.Offset; i++ {
		ch, err := logs.Read(f.Path, p.Offset, p.InRaid, p.Zone, raid)
		if err != nil {
			return err
		}
		if ch.OffsetEnd == p.Offset {
			break // solo hay una línea a medio escribir
		}
		if len(ch.Header) > 0 {
			p.Header = string(ch.Header)
		}
		if len(ch.Data) > 0 {
			if p.SessionID == 0 {
				id, err := c.OpenSession(in.Flavor, f.Name, logs.FirstTimestamp(f.Path))
				if err != nil {
					return err
				}
				p.SessionID = id
			}
			data := ch.Data
			if p.Seq == 0 && p.Header != "" {
				data = append([]byte(p.Header), data...)
			}
			res, err := c.SendBatch(p.SessionID, p.Seq, ch.OffsetStart, ch.OffsetEnd, data)
			if errors.Is(err, api.ErrSessionGone) {
				// La borraron en la web: lo que sigue va a una sesión nueva (lo borrado no se resube).
				p.SessionID, p.Seq = 0, 0
				pp := p
				a.Store.Update(func(s *state.State) { s.Files[key] = &pp })
				a.log("info", "%s se borró en la web; lo nuevo va a un registro nuevo.", f.Name)
				continue
			}
			if err != nil {
				return err
			}
			p.Seq++
			if res.Duplicate {
				// Ya estaba subido (un reintento): seguir exactamente desde donde lo tiene el sitio.
				if res.OffsetEnd != ch.OffsetEnd && res.OffsetEnd > p.Offset && res.OffsetEnd <= f.Size {
					if ch, err = logs.ReadRange(f.Path, p.Offset, res.OffsetEnd, p.InRaid, p.Zone, raid); err != nil {
						return err
					}
				}
			} else {
				a.mu.Lock()
				a.uploaded += int64(res.Sent)
				a.batches++
				a.mu.Unlock()
				zones := strings.Join(ch.Zones, ", ")
				if zones == "" {
					zones = p.Zone
				}
				a.log("upload", "Subí %s de %s (%s, lote %d, %d líneas)", human(int64(res.Sent)), zones, in.Label, p.Seq, ch.Lines)
			}
		}
		p.Offset, p.InRaid, p.Zone = ch.OffsetEnd, ch.InRaid, ch.Zone
		pp := p
		if err := a.Store.Update(func(s *state.State) { s.Files[key] = &pp }); err != nil {
			return err
		}
	}
	if isNew {
		pp := p
		a.Store.Update(func(s *state.State) { s.Files[key] = &pp })
	}
	return nil
}

func human(n int64) string {
	switch {
	case n >= 1<<20:
		return fmt.Sprintf("%.1f MB", float64(n)/(1<<20))
	case n >= 1<<10:
		return fmt.Sprintf("%d KB", n>>10)
	}
	return fmt.Sprintf("%d B", n)
}

// ── Consentimiento, vinculación y ajustes ────────────────────────────────────────────────

func (a *App) Consent() {
	now := time.Now()
	a.Store.Update(func(s *state.State) { s.ConsentAt = &now })
	a.log("info", "Aceptaste lo que hace la app. Desde ahora se suben los tramos de banda de los registros de combate.")
}

func (a *App) StartLink() (*Linking, error) {
	host, _ := os.Hostname()
	if host == "" {
		host = "PC"
	}
	ls, err := api.New(a.BaseURL, "", a.Version).StartLink(host)
	if err != nil {
		return nil, err
	}
	l := &Linking{UserCode: ls.UserCode, VerifyURL: ls.VerifyURL, ExpiresAt: time.Now().Add(time.Duration(ls.ExpiresIn) * time.Second)}
	a.mu.Lock()
	a.linking = l
	a.mu.Unlock()
	a.log("info", "Código de vinculación %s: confirmalo en el navegador.", ls.UserCode)
	if a.OpenURL != nil {
		a.OpenURL(ls.VerifyURL)
	}
	go a.pollLink(ls)
	return l, nil
}

func (a *App) pollLink(ls api.LinkStart) {
	c := api.New(a.BaseURL, "", a.Version)
	every := time.Duration(max(ls.Interval, 2)) * time.Second
	deadline := time.Now().Add(time.Duration(ls.ExpiresIn) * time.Second)
	for time.Now().Before(deadline) {
		time.Sleep(every)
		r, err := c.PollLink(ls.DeviceCode)
		if errors.Is(err, api.ErrPending) {
			continue
		}
		a.mu.Lock()
		a.linking = nil
		a.mu.Unlock()
		if err != nil {
			a.log("error", "No se pudo vincular: %v", err)
			return
		}
		a.Store.Update(func(s *state.State) { s.Token = r.Token; s.UserName = r.UserName })
		a.log("info", "PC vinculada a %s.", r.UserName)
		a.refreshConfig()
		a.UploadNow()
		return
	}
	a.mu.Lock()
	a.linking = nil
	a.mu.Unlock()
	a.log("error", "El código de vinculación venció. Pedí uno nuevo.")
}

func (a *App) Unlink() {
	a.Store.Update(func(s *state.State) { s.Token = ""; s.UserName = ""; s.DeviceName = "" })
	a.log("info", "Desvinculaste esta PC. No se sube nada más hasta que la vuelvas a vincular.")
}

func (a *App) SetPaused(p bool) {
	a.Store.Update(func(s *state.State) { s.Paused = p })
	if p {
		a.log("info", "Subida en pausa.")
	} else {
		a.log("info", "Subida reanudada.")
		a.UploadNow()
	}
}

func (a *App) SetFlavor(flavor string, enabled bool) {
	a.Store.Update(func(s *state.State) { s.Disabled[flavor] = !enabled })
}

func (a *App) SetWowRoot(dir string) error {
	if dir != "" {
		if st, err := os.Stat(dir); err != nil || !st.IsDir() {
			return fmt.Errorf("no existe la carpeta %s", dir)
		}
	}
	a.Store.Update(func(s *state.State) { s.WowRoot = dir })
	return nil
}

// SetEvery fija cada cuánto se sube (0 = el valor del sitio).
func (a *App) SetEvery(seconds int) error {
	d := time.Duration(seconds) * time.Second
	if seconds != 0 && (d < MinEvery || d > MaxEvery) {
		return fmt.Errorf("elegí entre 1 minuto y 24 horas")
	}
	a.Store.Update(func(s *state.State) { s.Every = seconds })
	if seconds == 0 {
		a.log("info", "Subida cada %s (lo que indica el sitio).", human2(a.batchEvery()))
	} else {
		a.log("info", "Subida cada %s.", human2(d))
	}
	return nil
}

func human2(d time.Duration) string {
	switch {
	case d%time.Hour == 0:
		return fmt.Sprintf("%d h", int(d/time.Hour))
	case d%time.Minute == 0:
		return fmt.Sprintf("%d min", int(d/time.Minute))
	}
	return d.String()
}

func (a *App) SetAddonAuto(v bool) { a.Store.Update(func(s *state.State) { s.AddonAuto = v }) }

// CheckAddon busca la última versión del addon y, si corresponde, la instala en cada versión de WoW.
func (a *App) CheckAddon(force bool) {
	url := defaultAddonURL
	a.mu.Lock()
	if a.cfg != nil && a.cfg.AddonLatestURL != "" {
		url = a.cfg.AddonLatestURL
	}
	a.mu.Unlock()
	rel, err := addon.Latest(url)
	if err != nil {
		a.log("error", "No pude consultar la última versión del addon: %v", err)
		return
	}
	a.mu.Lock()
	a.latest = &rel
	a.mu.Unlock()
	s := a.Store.Read()
	if !force && (!s.AddonAuto || s.ConsentAt == nil) {
		return
	}
	_, installs := a.Installs()
	var zipData []byte
	for _, in := range installs {
		if s.Disabled[in.Flavor] || addon.InstalledVersion(in.AddOnsDir()) == rel.Version {
			continue
		}
		if zipData == nil {
			if zipData, err = addon.Download(rel); err != nil {
				a.log("error", "Descarga del addon: %v", err)
				return
			}
		}
		if err := addon.Install(zipData, in.AddOnsDir()); err != nil {
			a.log("error", "No pude instalar el addon en %s: %v", in.Label, err)
			continue
		}
		a.log("info", "Addon Rebirth Track v%s instalado en %s (recargá la interfaz con /reload si el juego está abierto).", rel.Version, in.Label)
	}
}

// ── Estado para el panel ───────────────────────────────────────────────────────────────

type InstallStatus struct {
	wow.Install
	Enabled      bool   `json:"enabled"`
	AddonVersion string `json:"addonVersion"`
	Logs         int    `json:"logs"`
	LastLog      string `json:"lastLog,omitempty"`
}

type Status struct {
	Version   string          `json:"version"`
	SiteURL   string          `json:"siteUrl"`
	Consent   bool            `json:"consent"`
	Linked    bool            `json:"linked"`
	UserName  string          `json:"userName"`
	Device    string          `json:"deviceName"`
	Paused    bool            `json:"paused"`
	AddonAuto bool            `json:"addonAutoUpdate"`
	WowRoot   string          `json:"wowRoot"`
	Installs  []InstallStatus `json:"installs"`
	Latest    *addon.Release  `json:"addonLatest"`
	Linking   *Linking        `json:"linking"`
	LastRun   *time.Time      `json:"lastRun"`
	NextRun   *time.Time      `json:"nextRun"`
	Uploaded  int64           `json:"uploadedBytes"`
	Batches   int             `json:"batches"`
	Activity  []Activity      `json:"activity"`
	Autostart bool            `json:"autostart"`
	Packaged  bool            `json:"packaged"`
	AppUpdate AppUpdate       `json:"appUpdate"`
	Every     int             `json:"uploadEverySeconds"` // elegido (0 = el del sitio)
	EveryNow  int             `json:"uploadEveryEffective"`
}

func (a *App) Status() Status {
	s := a.Store.Read()
	root, installs := a.Installs()
	st := Status{
		Version: a.Version, SiteURL: a.BaseURL, Consent: s.ConsentAt != nil, Linked: s.Token != "",
		UserName: s.UserName, Device: s.DeviceName, Paused: s.Paused, AddonAuto: s.AddonAuto, WowRoot: root,
		Autostart: AutostartEnabled(), Packaged: Packaged(), AppUpdate: a.appUpdateStatus(s),
		Every: s.Every, EveryNow: int(a.batchEvery() / time.Second),
	}
	if s.WowRoot != "" {
		st.WowRoot = s.WowRoot
	}
	for _, in := range installs {
		is := InstallStatus{Install: in, Enabled: !s.Disabled[in.Flavor], AddonVersion: addon.InstalledVersion(in.AddOnsDir())}
		if files, err := logs.List(in.LogsDir()); err == nil {
			is.Logs = len(files)
			if len(files) > 0 {
				is.LastLog = files[len(files)-1].Name
			}
		}
		st.Installs = append(st.Installs, is)
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	st.Latest, st.Linking, st.Uploaded, st.Batches = a.latest, a.linking, a.uploaded, a.batches
	if !a.lastRun.IsZero() {
		lr := a.lastRun
		nr := lr.Add(a.batchEveryLocked())
		st.LastRun, st.NextRun = &lr, &nr
	}
	st.Activity = make([]Activity, len(a.activity))
	for i := range a.activity { // más reciente primero
		st.Activity[i] = a.activity[len(a.activity)-1-i]
	}
	return st
}

func (a *App) batchEveryLocked() time.Duration {
	if s := a.Store.Read().Every; s > 0 {
		return time.Duration(s) * time.Second
	}
	if a.cfg != nil && a.cfg.BatchSeconds >= 30 {
		return time.Duration(a.cfg.BatchSeconds) * time.Second
	}
	return 3 * time.Minute
}
