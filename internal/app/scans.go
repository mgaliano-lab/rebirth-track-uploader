package app

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/mgaliano-lab/rebirth-track-uploader/internal/api"
	"github.com/mgaliano-lab/rebirth-track-uploader/internal/scans"
	"github.com/mgaliano-lab/rebirth-track-uploader/internal/state"
	"github.com/mgaliano-lab/rebirth-track-uploader/internal/update"
	"github.com/mgaliano-lab/rebirth-track-uploader/internal/wow"
)

// Relevamientos: qué archivos de la carpeta de WoW se suben lo dice el manifiesto que publica el
// sitio (firmado; ver internal/scans). La app lo consulta al arrancar y cada pollSeconds, lo guarda
// en caché por si no hay red y, si nunca pudo bajar uno, usa scans.Default. Los registros de combate
// siguen su propio camino (filtrados en la PC).

const scansCacheFile = "scans.json"

type scansCache struct {
	ETag     string         `json:"etag"`
	Envelope scans.Envelope `json:"envelope"`
}

// scanState es lo que la app sabe del manifiesto y de cada archivo (para el panel).
type scanState struct {
	mu        sync.Mutex
	run       sync.Mutex // una pasada de relevamientos a la vez
	manifest  scans.Manifest
	source    string // incluido | caché | sitio
	etag      string
	checkedAt time.Time
	nextCheck time.Time
	lastErr   string
	lastRun   map[string]time.Time
	files     map[string]ScanFile
}

// ScanFile es el estado de un archivo relevado.
type ScanFile struct {
	ScanID     string     `json:"scanId"`
	Flavor     string     `json:"flavor"`
	Install    string     `json:"install"`
	Path       string     `json:"path"`
	Size       int64      `json:"size"`
	UploadedAt *time.Time `json:"uploadedAt,omitempty"`
	State      string     `json:"state"` // subido | sin cambios | grande | error | rechazado
	Detail     string     `json:"detail,omitempty"`
}

type ScanStatus struct {
	scans.Scan
	LastRun *time.Time `json:"lastRun,omitempty"`
	Files   []ScanFile `json:"files"`
}

type ScansStatus struct {
	Version   int          `json:"version"`
	Updated   string       `json:"updated"`
	Source    string       `json:"source"`
	CheckedAt *time.Time   `json:"checkedAt,omitempty"`
	NextCheck *time.Time   `json:"nextCheck,omitempty"`
	Error     string       `json:"error,omitempty"`
	Scans     []ScanStatus `json:"scans"`
}

func (a *App) trustedKeys() []string {
	return append(append([]string{}, scans.TrustedKeys...), a.ExtraScanKeys...)
}

// loadScans arranca con la caché (si su firma sigue siendo válida) o con el manifiesto incluido.
func (a *App) loadScans() {
	sc := &a.scans
	sc.mu.Lock()
	defer sc.mu.Unlock()
	sc.manifest, sc.source = scans.Default, "incluido"
	b, err := os.ReadFile(filepath.Join(a.Store.Dir(), scansCacheFile))
	if err != nil {
		return
	}
	var c scansCache
	if json.Unmarshal(b, &c) != nil {
		return
	}
	m, err := scans.Open(c.Envelope, a.trustedKeys())
	if err != nil || update.Newer(m.MinAppVersion, a.Version) {
		return
	}
	sc.manifest, sc.source, sc.etag = m, "caché", c.ETag
}

func (a *App) currentScans() scans.Manifest {
	a.scans.mu.Lock()
	defer a.scans.mu.Unlock()
	return a.scans.manifest
}

// scansDue: toca volver a consultar el manifiesto.
func (a *App) scansDue() bool {
	a.scans.mu.Lock()
	defer a.scans.mu.Unlock()
	return time.Now().After(a.scans.nextCheck)
}

// SyncScans consulta el manifiesto en el sitio y, si cambió y es válido, lo aplica (sin reiniciar).
func (a *App) SyncScans() {
	sc := &a.scans
	sc.mu.Lock()
	etag := sc.etag
	if sc.source != "sitio" && sc.source != "caché" {
		etag = ""
	}
	cur := sc.manifest
	sc.mu.Unlock()

	body, newEtag, notModified, err := a.client().Scans(etag)
	var m scans.Manifest
	var env scans.Envelope
	if err == nil && !notModified {
		if err = json.Unmarshal(body, &env); err == nil {
			m, err = scans.Open(env, a.trustedKeys())
		}
		switch {
		case err != nil:
		case update.Newer(m.MinAppVersion, a.Version):
			err = fmt.Errorf("la versión %d de los relevamientos necesita la app v%s o más nueva: actualizala", m.Version, m.MinAppVersion)
		case m.Version < cur.Version:
			err = fmt.Errorf("el sitio mandó la versión %d de los relevamientos, anterior a la %d que ya tengo: la ignoro", m.Version, cur.Version)
		}
	}

	sc.mu.Lock()
	now := time.Now()
	sc.checkedAt = now
	errText := ""
	if err != nil {
		errText = err.Error()
	}
	changedErr := errText != sc.lastErr
	sc.lastErr = errText
	applied := false
	if err == nil && !notModified {
		applied = m.Version != cur.Version || sc.source != "sitio"
		sc.manifest, sc.source, sc.etag = m, "sitio", newEtag
	} else if err == nil && sc.source == "caché" {
		sc.source = "sitio" // el sitio confirmó que la caché está al día
	}
	sc.nextCheck = now.Add(sc.manifest.Poll())
	sc.mu.Unlock()

	if err != nil {
		if changedErr {
			a.log("error", "No pude actualizar los relevamientos: %v (sigo con la versión %d)", err, cur.Version)
		}
		return
	}
	if notModified {
		return
	}
	if b, err := json.Marshal(scansCache{ETag: newEtag, Envelope: env}); err == nil {
		_ = os.WriteFile(filepath.Join(a.Store.Dir(), scansCacheFile), b, 0o600)
	}
	if applied && m.Version != cur.Version {
		a.log("info", "Relevamientos actualizados a la versión %d: %s.", m.Version, scanLabels(m))
		a.forgetRemovedScans(m)
		a.wakeScans()
	}
}

func scanLabels(m scans.Manifest) string {
	if len(m.Scans) == 0 {
		return "ninguno"
	}
	var out []string
	for _, s := range m.Scans {
		out = append(out, s.Label)
	}
	return strings.Join(out, " · ")
}

// forgetRemovedScans olvida lo subido de relevamientos que ya no están (si vuelven, se resuben).
func (a *App) forgetRemovedScans(m scans.Manifest) {
	ids := map[string]bool{}
	for _, s := range m.Scans {
		ids[s.ID] = true
	}
	a.Store.Update(func(st *state.State) {
		for k := range st.ScansSent {
			if p := strings.SplitN(k, "|", 3); len(p) == 3 && !ids[p[1]] {
				delete(st.ScansSent, k)
			}
		}
	})
	a.scans.mu.Lock()
	for k, f := range a.scans.files {
		if !ids[f.ScanID] {
			delete(a.scans.files, k)
		}
	}
	a.scans.mu.Unlock()
}

// wakeScans hace que la próxima pasada corra todos los relevamientos (manifiesto nuevo).
func (a *App) wakeScans() {
	a.scans.mu.Lock()
	a.scans.lastRun = nil
	a.scans.mu.Unlock()
}

// anyScanDue: algún relevamiento ya cumplió su intervalo.
func (a *App) anyScanDue() bool {
	m := a.currentScans()
	a.scans.mu.Lock()
	defer a.scans.mu.Unlock()
	for _, s := range m.Scans {
		if time.Since(a.scans.lastRun[s.ID]) >= s.Every() {
			return true
		}
	}
	return false
}

// RunScans lee y sube los archivos de los relevamientos que tocan (todos con force). Solo con
// consentimiento, PC vinculada y sin pausa; nunca en versiones de WoW desactivadas.
func (a *App) RunScans(force bool) {
	sc := &a.scans
	if !sc.run.TryLock() {
		return
	}
	defer sc.run.Unlock()
	s := a.Store.Read()
	if s.ConsentAt == nil || s.Token == "" || s.Paused {
		return
	}
	m := a.currentScans()
	_, installs := a.Installs()
	c := a.client()
	for _, scan := range m.Scans {
		sc.mu.Lock()
		due := force || time.Since(sc.lastRun[scan.ID]) >= scan.Every()
		if due {
			if sc.lastRun == nil {
				sc.lastRun = map[string]time.Time{}
			}
			sc.lastRun[scan.ID] = time.Now()
		}
		sc.mu.Unlock()
		if !due {
			continue
		}
		for _, in := range installs {
			if s.Disabled[in.Flavor] || !scan.AppliesTo(in.Flavor) {
				continue
			}
			a.writeAddonSettings(in)
			if err := a.scanInstall(c, m.Version, scan, in); errors.Is(err, api.ErrUnauthorized) {
				a.unlinked()
				return
			}
		}
	}
}

func (a *App) scanInstall(c *api.Client, version int, scan scans.Scan, in wow.Install) error {
	seen := map[string]bool{}
	for _, pattern := range scan.Paths {
		matches, err := scans.Find(in.Dir, pattern)
		if err != nil {
			continue // el manifiesto ya viene validado: no debería pasar
		}
		for _, m := range matches {
			key := in.Flavor + "|" + scan.ID + "|" + m.Rel
			if seen[key] {
				continue
			}
			seen[key] = true
			f := ScanFile{ScanID: scan.ID, Flavor: in.Flavor, Install: in.Label, Path: m.Rel, Size: m.Size}
			data, err := scans.Read(m, scan.MaxBytes)
			var big scans.ErrTooBig
			switch {
			case errors.As(err, &big):
				f.State, f.Detail = "grande", fmt.Sprintf("pesa %s; el tope es %s", human(big.Size), human(big.Max))
				a.setScanFile(key, f)
				continue
			case err != nil:
				f.State, f.Detail = "error", err.Error()
				a.setScanFile(key, f)
				continue
			}
			noBalance := a.Store.Read().NoBalance
			if noBalance {
				data = StripBalance(data)
			}
			sum := sha256.Sum256(data)
			sha := hex.EncodeToString(sum[:])
			if a.Store.Read().ScansSent[key] == sha {
				f.State = "sin cambios"
				a.keepUploadedAt(key, &f)
				a.setScanFile(key, f)
				continue
			}
			res, code, err := c.SendFile(api.UploadFile{Endpoint: scan.Upload, ScanID: scan.ID, Flavor: in.Flavor, Path: m.Rel, SHA256: sha, ModTime: m.ModTime, ManifestVersion: version, Data: data, Balance: !noBalance})
			if errors.Is(err, api.ErrUnauthorized) {
				return err
			}
			if err != nil {
				a.log("error", "No pude subir %s (%s): %v", m.Rel, in.Label, err)
				f.State, f.Detail = "error", err.Error()
				if definitive(code) {
					// Rechazo definitivo del sitio: no se reintenta hasta que el archivo cambie.
					f.State = "rechazado"
					a.markScanSent(key, sha)
				}
				a.setScanFile(key, f)
				continue
			}
			now := time.Now()
			f.State, f.UploadedAt = "subido", &now
			a.markScanSent(key, sha)
			a.setScanFile(key, f)
			if !res.Duplicate {
				a.mu.Lock()
				a.uploaded += int64(res.Sent)
				a.mu.Unlock()
				a.log("upload", "Subí %s (%s, %s).", m.Rel, in.Label, human(int64(res.Sent)))
			}
			for _, msg := range res.Messages {
				a.log("info", "%s", msg)
			}
		}
	}
	// Lo que ya no aparece (borrado, o el relevamiento cambió de rutas) deja de mostrarse.
	a.scans.mu.Lock()
	for k, f := range a.scans.files {
		if f.ScanID == scan.ID && f.Flavor == in.Flavor && !seen[k] {
			delete(a.scans.files, k)
		}
	}
	a.scans.mu.Unlock()
	return nil
}

// definitive: el sitio rechazó el archivo en sí (no lo pide, es muy grande…). Un 404 es un sitio
// que todavía no tiene el endpoint (se reintenta), igual que 408/429.
func definitive(code int) bool {
	return code >= 400 && code < 500 && code != 404 && code != 408 && code != 429
}

func (a *App) markScanSent(key, sha string) {
	a.Store.Update(func(st *state.State) {
		if st.ScansSent == nil {
			st.ScansSent = map[string]string{}
		}
		st.ScansSent[key] = sha
	})
}

func (a *App) setScanFile(key string, f ScanFile) {
	a.scans.mu.Lock()
	defer a.scans.mu.Unlock()
	if a.scans.files == nil {
		a.scans.files = map[string]ScanFile{}
	}
	a.scans.files[key] = f
}

func (a *App) keepUploadedAt(key string, f *ScanFile) {
	a.scans.mu.Lock()
	defer a.scans.mu.Unlock()
	if prev, ok := a.scans.files[key]; ok {
		f.UploadedAt = prev.UploadedAt
	}
}

// ScansStatus es lo que muestra el panel: el manifiesto vigente y cada archivo relevado.
func (a *App) ScansStatus() ScansStatus {
	sc := &a.scans
	sc.mu.Lock()
	defer sc.mu.Unlock()
	st := ScansStatus{Version: sc.manifest.Version, Updated: sc.manifest.Updated, Source: sc.source, Error: sc.lastErr, Scans: []ScanStatus{}}
	if !sc.checkedAt.IsZero() {
		t, n := sc.checkedAt, sc.nextCheck
		st.CheckedAt, st.NextCheck = &t, &n
	}
	for _, s := range sc.manifest.Scans {
		ss := ScanStatus{Scan: s, Files: []ScanFile{}}
		if t, ok := sc.lastRun[s.ID]; ok {
			ss.LastRun = &t
		}
		for _, f := range sc.files {
			if f.ScanID == s.ID {
				ss.Files = append(ss.Files, f)
			}
		}
		sort.Slice(ss.Files, func(i, j int) bool {
			if ss.Files[i].Flavor != ss.Files[j].Flavor {
				return ss.Files[i].Flavor < ss.Files[j].Flavor
			}
			return ss.Files[i].Path < ss.Files[j].Path
		})
		st.Scans = append(st.Scans, ss)
	}
	return st
}

// CheckScansNow: botón «Buscar cambios» del panel.
func (a *App) CheckScansNow() {
	a.SyncScans()
	go a.RunScans(true)
}

// Balance de cada personaje que anota el addon (líneas «mo1;…» de RebirthTrack.lua, ver Collect.lua):
// con «Enviar mi balance» desactivada no sale de la PC aunque el juego todavía no haya recargado el
// addon con la opción nueva. El balance del banco de la guild («gbm1») no se toca.
var balanceLine = regexp.MustCompile(`(?m)^[^\n]*"(?:@[^;"\n]*;)?mo1;[^\n]*(?:\n|$)`)

func StripBalance(data []byte) []byte { return balanceLine.ReplaceAll(data, nil) }

// AppSettings.lua, en la carpeta del addon: las opciones del panel que el addon tiene que saber (lo
// lee al cargar). Sin este archivo (sin la app) el addon no anota el balance de los personajes.
const addonSettingsFile = "AppSettings.lua"

func addonSettings(balance bool) []byte {
	return []byte(fmt.Sprintf("-- Lo escribe Rebirth Track Uploader según su panel (no lo edites: la app lo vuelve a escribir).\nRebirthTrackApp = { balance = %t }\n", balance))
}

// writeAddonSettings lo deja al día en una versión de WoW, si el addon está instalado.
func (a *App) writeAddonSettings(in wow.Install) {
	dir := filepath.Join(in.AddOnsDir(), "RebirthTrack")
	if st, err := os.Stat(dir); err != nil || !st.IsDir() {
		return
	}
	want := addonSettings(!a.Store.Read().NoBalance)
	p := filepath.Join(dir, addonSettingsFile)
	if cur, err := os.ReadFile(p); err == nil && string(cur) == string(want) {
		return
	}
	if err := os.WriteFile(p, want, 0o644); err != nil {
		a.log("error", "No pude guardar las opciones del addon en %s: %v", in.Label, err)
	}
}

// SetSendBalance: casilla «Enviar mi balance» del panel.
func (a *App) SetSendBalance(v bool) {
	a.Store.Update(func(s *state.State) { s.NoBalance = !v })
	_, installs := a.Installs()
	for _, in := range installs {
		a.writeAddonSettings(in)
	}
	if v {
		a.log("info", "Enviar mi balance: activado. El addon lo anota desde la próxima vez que entres al juego (o con /reload).")
	} else {
		a.log("info", "Enviar mi balance: desactivado. La app deja de mandarlo, el sitio borra el que tenía y el addon deja de anotarlo al recargar.")
	}
	go a.RunScans(true)
}
