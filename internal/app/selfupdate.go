package app

import (
	"os"
	"path/filepath"

	"github.com/mgaliano-lab/rebirth-track-uploader/internal/state"
	"github.com/mgaliano-lab/rebirth-track-uploader/internal/update"
)

// Cómo se instaló la app, que define cómo se actualiza:
//   - store: la actualiza la Microsoft Store; la app solo avisa que hay una versión nueva y abre
//     su página en la Store (una app de la Store no puede reemplazarse a sí misma).
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
	StoreURL  string `json:"storeUrl,omitempty"` // solo instalada desde la Store
	Available bool   `json:"available"`
	Updating  bool   `json:"updating"`
}

// CheckApp busca una versión nueva de la app y, si está activada la actualización automática
// (o la pidió el usuario), la instala y reinicia la app.
func (a *App) CheckApp(force bool) {
	mode := InstallMode()
	rel, err := update.Latest()
	if err != nil {
		if force {
			a.log("error", "No pude buscar actualizaciones de la app: %v", err)
		}
		return
	}
	a.mu.Lock()
	prev := a.appLatest
	a.appLatest = &rel
	busy := a.updating
	a.mu.Unlock()
	if mode == "store" {
		// Se avisa una vez por versión; actualizar es abrir la Store (OpenStore).
		if update.Newer(rel.Version, a.Version) && (force || prev == nil || prev.Version != rel.Version) {
			a.log("info", "Hay una versión nueva de la app (v%s): actualizala desde la Microsoft Store.", rel.Version)
		} else if force {
			a.log("info", "La app está al día (v%s).", a.Version)
		}
		return
	}
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
		if a.BeforeUpdate != nil {
			a.BeforeUpdate()
		}
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

// StoreURL abre la página de la app en la Microsoft Store (ahí está el botón Actualizar).
const StoreURL = "ms-windows-store://pdp/?productid=9NHNP7JCN1BG"

// OpenStore abre la Microsoft Store en la página de la app para actualizarla.
func (a *App) OpenStore() {
	if a.OpenURL != nil {
		a.OpenURL(StoreURL)
	}
}

func (a *App) SetAppAuto(v bool) { a.Store.Update(func(s *state.State) { s.AppAuto = v }) }

func (a *App) appUpdateStatus(s state.State) AppUpdate {
	u := AppUpdate{Mode: InstallMode(), Auto: s.AppAuto}
	if u.Mode == "store" {
		u.StoreURL = StoreURL
	}
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
