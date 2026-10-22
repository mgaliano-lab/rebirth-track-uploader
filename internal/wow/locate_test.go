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

func TestDetectForever(t *testing.T) {
	root := filepath.Join(t.TempDir(), "World of Warcraft")
	for _, f := range []string{"_classic_era_", "_classic_beta_", "_forever_nuevo_", "_ptr_", "_classic_ptr_", "_beta_", "Data"} {
		os.MkdirAll(filepath.Join(root, f, "WTF"), 0o755)
	}
	_, ins := Detect(root)
	got := map[string]string{}
	for _, i := range ins {
		got[i.Flavor] = i.Label
	}
	want := map[string]string{"_classic_era_": "Classic Era", "_classic_beta_": "WoW Forever (beta)", "_forever_nuevo_": "_forever_nuevo_"}
	if len(got) != len(want) {
		t.Fatalf("Detect: %+v", got)
	}
	for k, v := range want {
		if got[k] != v {
			t.Errorf("%s: %q, quería %q", k, got[k], v)
		}
	}
}
