package app

import (
	"fmt"
	"time"

	"github.com/mgaliano-lab/rebirth-track-uploader/internal/logs"
	"github.com/mgaliano-lab/rebirth-track-uploader/internal/state"
)

// Logs anteriores al consentimiento: por defecto no se suben. Desde el panel se listan con lo
// que tienen de bandas y el usuario elige cuáles importar; esos se releen desde el principio con
// el mismo filtro de siempre (solo tramos de banda).

type OldLog struct {
	Key       string     `json:"key"`
	Install   string     `json:"install"`
	Name      string     `json:"name"`
	Size      int64      `json:"size"`
	First     *time.Time `json:"firstLineAt,omitempty"`
	Modified  time.Time  `json:"modifiedAt"`
	Zones     []string   `json:"zones"`
	RaidBytes int64      `json:"raidBytes"`
	// available: se puede importar · empty: no tiene bandas · partial: ya se empezó a subir lo nuevo
	// de este archivo, así que lo anterior no se puede intercalar · queued/importing/imported.
	Status   string  `json:"status"`
	Progress float64 `json:"progress"`
}

type scanCache struct {
	size  int64
	mod   time.Time
	first *time.Time
	sum   logs.Summary
}

// importable dice si un archivo es un log viejo salteado (o que se va a saltear) y hasta dónde.
func importable(p *state.FileProgress, f logs.File, consentAt time.Time) (end int64, ok bool) {
	if p == nil { // todavía no lo vio ningún ciclo
		return f.Size, f.ModTime.Before(consentAt)
	}
	if p.Imported {
		return f.Size, true
	}
	// Los anteriores a la marca Skipped: sin sesión, leídos hasta algún lado y sin tocar desde antes de aceptar.
	if p.Skipped || (p.SessionID == 0 && p.Seq == 0 && p.Offset > 0 && f.ModTime.Before(consentAt)) {
		return min(p.Offset, f.Size), true
	}
	return 0, false
}

func (a *App) OldLogs() ([]OldLog, error) {
	s := a.Store.Read()
	if s.ConsentAt == nil {
		return nil, fmt.Errorf("primero aceptá lo que hace la app")
	}
	raid := a.instanceSet()
	_, installs := a.Installs()
	out := []OldLog{}
	for _, in := range installs {
		if s.Disabled[in.Flavor] {
			continue
		}
		files, err := logs.List(in.LogsDir())
		if err != nil {
			continue
		}
		for i := len(files) - 1; i >= 0; i-- { // más nuevo primero
			f := files[i]
			key := in.Flavor + "/" + f.Name
			var p *state.FileProgress
			if pp, ok := s.Files[key]; ok {
				cp := *pp
				p = &cp
			}
			end, ok := importable(p, f, *s.ConsentAt)
			if !ok {
				continue
			}
			sc, err := a.scan(key, f, end, raid)
			if err != nil {
				continue
			}
			o := OldLog{Key: key, Install: in.Label, Name: f.Name, Size: f.Size, First: sc.first, Modified: f.ModTime, Zones: sc.sum.Zones, RaidBytes: sc.sum.RaidBytes}
			a.mu.Lock()
			queued := a.imports[key]
			a.mu.Unlock()
			switch {
			case queued:
				o.Status = "queued"
			case p != nil && p.Imported:
				o.Status, o.Progress = "imported", 1
				if f.Size > 0 && p.Offset < f.Size {
					o.Status, o.Progress = "importing", float64(p.Offset)/float64(f.Size)
				}
			case p != nil && p.SessionID != 0:
				o.Status = "partial"
			case len(sc.sum.Zones) == 0:
				o.Status = "empty"
			default:
				o.Status = "available"
			}
			if o.Zones == nil {
				o.Zones = []string{}
			}
			out = append(out, o)
		}
	}
	return out, nil
}

// scan cachea el resumen por archivo mientras no cambie (los logs viejos pueden pesar cientos de MB).
func (a *App) scan(key string, f logs.File, end int64, raid logs.Instances) (scanCache, error) {
	a.mu.Lock()
	sc, ok := a.scans[key]
	a.mu.Unlock()
	if ok && sc.size == end && sc.mod.Equal(f.ModTime) {
		return sc, nil
	}
	sum, err := logs.Scan(f.Path, end, raid)
	if err != nil {
		return scanCache{}, err
	}
	sc = scanCache{size: end, mod: f.ModTime, first: logs.FirstTimestamp(f.Path), sum: sum}
	a.mu.Lock()
	if a.scans == nil {
		a.scans = map[string]scanCache{}
	}
	a.scans[key] = sc
	a.mu.Unlock()
	return sc, nil
}

// ImportLogs encola los archivos elegidos; el próximo ciclo (que se dispara ya) los relee.
func (a *App) ImportLogs(keys []string) (int, error) {
	list, err := a.OldLogs()
	if err != nil {
		return 0, err
	}
	ok := map[string]OldLog{}
	for _, o := range list {
		if o.Status == "available" {
			ok[o.Key] = o
		}
	}
	n := 0
	a.mu.Lock()
	if a.imports == nil {
		a.imports = map[string]bool{}
	}
	for _, k := range keys {
		if _, valid := ok[k]; valid {
			a.imports[k] = true
			n++
		}
	}
	a.mu.Unlock()
	if n == 0 {
		return 0, fmt.Errorf("ninguno de los archivos elegidos se puede importar")
	}
	a.log("info", "Importando %d registro(s) anteriores: se sube solo lo de bandas.", n)
	a.UploadNow()
	return n, nil
}

// applyImports rebobina los archivos encolados (corre dentro del ciclo, sin otro ciclo en paralelo).
func (a *App) applyImports() {
	a.mu.Lock()
	keys := a.imports
	a.imports = nil
	a.mu.Unlock()
	if len(keys) == 0 {
		return
	}
	a.Store.Update(func(s *state.State) {
		for k := range keys {
			p := s.Files[k]
			if p != nil && p.SessionID != 0 {
				continue // empezó a subirse lo nuevo mientras tanto
			}
			s.Files[k] = &state.FileProgress{Imported: true}
		}
	})
}
