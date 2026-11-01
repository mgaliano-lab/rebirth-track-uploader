package addon

import (
	"archive/zip"
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func zipOf(files map[string]string) []byte {
	var b bytes.Buffer
	w := zip.NewWriter(&b)
	for n, c := range files {
		f, _ := w.Create(n)
		f.Write([]byte(c))
	}
	w.Close()
	return b.Bytes()
}

func TestInstallOnlyAddonFolder(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, "RebirthTrack"), 0o755)
	os.WriteFile(filepath.Join(dir, "RebirthTrack", "viejo.lua"), []byte("x"), 0o600)
	z := zipOf(map[string]string{
		"RebirthTrack/RebirthTrack.toc": "## Version: 0.0.2\n",
		"RebirthTrack/RebirthTrack.lua": "-- lua",
		"../../malo.txt":                "no",
		"OtroAddon/x.lua":               "no",
	})
	if err := Install(z, dir); err != nil {
		t.Fatal(err)
	}
	if v := InstalledVersion(dir); v != "0.0.2" {
		t.Fatalf("versión %q", v)
	}
	for _, p := range []string{"RebirthTrack/viejo.lua", "OtroAddon", "../malo.txt"} {
		if _, err := os.Stat(filepath.Join(dir, p)); err == nil {
			t.Errorf("no debería existir %s", p)
		}
	}
}

func TestWritePlans(t *testing.T) {
	dir := t.TempDir()
	if wrote, err := WritePlans(dir, []byte("x")); wrote || err != nil {
		t.Fatalf("sin el addon no escribe: %v %v", wrote, err)
	}
	os.MkdirAll(filepath.Join(dir, "RebirthTrack"), 0o755)
	os.WriteFile(filepath.Join(dir, "RebirthTrack", "RebirthTrack.toc"), []byte("## Version: 1\n"), 0o600)
	lua := []byte("RebirthTrackPlans = { at = 1, plans = {} }\n")
	if wrote, err := WritePlans(dir, lua); !wrote || err != nil {
		t.Fatalf("primera vez escribe: %v %v", wrote, err)
	}
	if wrote, _ := WritePlans(dir, lua); wrote {
		t.Fatal("igual: no reescribe")
	}
	got, _ := os.ReadFile(filepath.Join(dir, "RebirthTrack", PlansFile))
	if !bytes.Equal(got, lua) {
		t.Fatalf("contenido %q", got)
	}
}

func TestWritePlansReadOnlyAndBlocked(t *testing.T) {
	replaceWait = time.Millisecond
	dir := t.TempDir()
	addon := filepath.Join(dir, "RebirthTrack")
	os.MkdirAll(addon, 0o755)
	os.WriteFile(filepath.Join(addon, "RebirthTrack.toc"), []byte("## Version: 1\n"), 0o600)
	dst := filepath.Join(addon, PlansFile)

	// Plans.lua de solo lectura: se reemplaza igual.
	os.WriteFile(dst, []byte("viejo"), 0o444)
	if wrote, err := WritePlans(dir, []byte("nuevo")); !wrote || err != nil {
		t.Fatalf("%v %v", wrote, err)
	}
	if got, _ := os.ReadFile(dst); string(got) != "nuevo" {
		t.Fatalf("%q", got)
	}

	// Si no se puede reemplazar, después de reintentar devuelve el error y no deja temporales.
	os.Remove(dst)
	os.Mkdir(dst, 0o755) // un directorio en lugar del archivo: el reemplazo falla siempre
	os.WriteFile(filepath.Join(dst, "x"), []byte("x"), 0o600)
	if _, err := WritePlans(dir, []byte("otro")); err == nil {
		t.Fatal("tenía que fallar")
	}
	entries, _ := os.ReadDir(addon)
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), ".plans-") {
			t.Fatalf("quedó el temporal %s", e.Name())
		}
	}
}
