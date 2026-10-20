package wow

import (
	"os"
	"path/filepath"
	"testing"
)

func TestNormalize(t *testing.T) {
	root := filepath.Join(t.TempDir(), "World of Warcraft")
	for _, in := range []string{
		root,
		filepath.Join(root, "_anniversary_"),
		filepath.Join(root, "_anniversary_", "Logs"),
		filepath.Join(root, "_retail_", "Interface", "AddOns"),
	} {
		if got := Normalize(in); got != root {
			t.Errorf("Normalize(%q) = %q", in, got)
		}
	}
	os.MkdirAll(filepath.Join(root, "_anniversary_", "Logs"), 0o755)
	if _, ins := Detect(root); len(ins) != 1 || ins[0].Flavor != "_anniversary_" {
		t.Errorf("Detect: %+v", ins)
	}
}
