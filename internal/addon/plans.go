package addon

import (
	"bytes"
	"os"
	"path/filepath"
)

// PlansFile: los planes de los personajes de la cuenta (lo arma el sitio), que el addon lee al
// entrar al juego o con /reload. No viene en el zip: Install lo borra con la carpeta y la app lo
// vuelve a escribir.
const PlansFile = "Plans.lua"

// WritePlans deja Plans.lua en AddOns/RebirthTrack si el addon está instalado y el contenido
// cambió. Devuelve si lo escribió.
func WritePlans(addonsDir string, lua []byte) (bool, error) {
	dir := filepath.Join(addonsDir, Folder)
	if _, err := os.Stat(filepath.Join(dir, Folder+".toc")); err != nil {
		return false, nil // sin el addon en esta versión de WoW
	}
	dst := filepath.Join(dir, PlansFile)
	if old, err := os.ReadFile(dst); err == nil && bytes.Equal(old, lua) {
		return false, nil
	}
	tmp, err := os.CreateTemp(dir, ".plans-")
	if err != nil {
		return false, err
	}
	defer os.Remove(tmp.Name())
	if _, err := tmp.Write(lua); err != nil {
		tmp.Close()
		return false, err
	}
	if err := tmp.Close(); err != nil {
		return false, err
	}
	return true, os.Rename(tmp.Name(), dst)
}
