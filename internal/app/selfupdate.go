package app

import (
	"os"
	"path/filepath"

	"github.com/mgaliano-lab/rebirth-track-uploader/internal/state"
	"github.com/mgaliano-lab/rebirth-track-uploader/internal/update"
)

// Cómo se instaló la app, que define cómo se actualiza:
//   - store: la actualiza la Microsoft Store (acá no se hace nada).
//   - installer: se bajó el instalador; se corre la versión nueva en modo silencioso.
//   - portable: el .exe suelto; se reemplaza a sí mismo.
func InstallMode() string {
	if Packaged() {
		return "store"
	}
	exe, err := os.Executable()
	if err != nil {
		return "portable"
	}
	if _, err := os.Stat(filepath.Join(filepath.Dir(exe), "Desinstalar.exe")); err == nil {
		return "installer"
	}
	return "portable"
}

type AppUpdate struct {
	Mode      string `json:"mode"`
	Auto      bool   `json:"auto"`
	Latest    string `json:"latest,omitempty"`
	PageURL   string `json:"pageUrl,omitempty"`
	Available bool   `json:"available"`
	Updating  bool   `json:"updating"`
}

// CheckApp busca una versión nueva de la app y, si está activada la actualización automática
// (o la pidió el usuario), la instala y reinicia la app.
func (a *App) CheckApp(force bool) {
	mode := InstallMode()
	if mode == "store" {
		return
	}
	rel, err := update.Latest()
	if err != nil {
		if force {
			a.log("error", "No pude buscar actualizaciones de la app: %v", err)
		}
		return
	}
	a.mu.Lock()
	a.appLatest = &rel
	busy := a.updating
	a.mu.Unlock()
	if busy || !update.Newer(rel.Version, a.Version) {
		if force {
			a.log("info", "La app está al día (v%s).", a.Version)
		}
		return
	}
	if !force && !a.Store.Read().AppAuto {
		return
	}
	a.mu.Lock()
	a.updating = true
	a.mu.Unlock()
	a.log("info", "Actualizando la app a v%s…", rel.Version)
	dir, _ := state.Dir()
	asset := rel.Portable
	if mode == "installer" {
		asset = rel.Setup
	}
	path, err := update.Download(rel, asset, filepath.Join(dir, "updates"))
	if err == nil {
		err = applyUpdate(mode, path)
	}
	if err != nil {
		a.mu.Lock()
		a.updating = false
		a.mu.Unlock()
		a.log("error", "No se pudo actualizar la app: %v", err)
		return
	}
	a.log("info", "Versión v%s lista: la app se reinicia.", rel.Version)
	if a.Quit != nil {
		a.Quit()
	}
}

func (a *App) SetAppAuto(v bool) { a.Store.Update(func(s *state.State) { s.AppAuto = v }) }

func (a *App) appUpdateStatus(s state.State) AppUpdate {
	u := AppUpdate{Mode: InstallMode(), Auto: s.AppAuto}
	a.mu.Lock()
	defer a.mu.Unlock()
	u.Updating = a.updating
	if a.appLatest != nil {
		u.Latest, u.PageURL = a.appLatest.Version, a.appLatest.PageURL
		u.Available = update.Newer(a.appLatest.Version, a.Version)
	}
	return u
}

// CleanupUpdate borra lo que quedó de una actualización anterior (el .exe viejo y las descargas).
func CleanupUpdate() {
	if exe, err := os.Executable(); err == nil {
		os.Remove(exe + ".old")
	}
	if dir, err := state.Dir(); err == nil {
		os.RemoveAll(filepath.Join(dir, "updates"))
	}
}
