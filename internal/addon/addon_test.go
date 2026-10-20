package addon

import (
	"archive/zip"
	"bytes"
	"os"
	"path/filepath"
	"testing"
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
