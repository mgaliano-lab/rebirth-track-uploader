package addon

import (
	"bytes"
	"os"
	"path/filepath"
	"time"
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
	return true, replace(tmp.Name(), dst)
}

// replaceWait: lo que espera replace entre intentos (las pruebas lo achican).
var replaceWait = 200 * time.Millisecond

// replace pone src en lugar de dst. En Windows reemplazar falla con «Acceso denegado» si otro
// programa tiene abierto dst en ese momento (el antivirus o el indexador revisándolo, el juego
// cargando el addon) o si quedó de solo lectura: se le saca el solo lectura y se reintenta unos
// segundos antes de rendirse (la app lo vuelve a intentar en el próximo latido).
func replace(src, dst string) error {
	var err error
	for i := range 6 {
		if err = os.Rename(src, dst); err == nil {
			return nil
		}
		if fi, e := os.Stat(dst); i == 0 && e == nil && fi.Mode().IsRegular() {
			os.Chmod(dst, fi.Mode().Perm()|0o200) // saca el solo lectura (en Windows, el atributo)
		}
		time.Sleep(time.Duration(i+1) * replaceWait)
	}
	return err
}
