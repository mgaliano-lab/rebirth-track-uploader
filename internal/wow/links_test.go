package wow

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLinkRequests(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "_anniversary_")
	sv := filepath.Join(dir, "WTF", "Account", "MATIAS#1", "SavedVariables")
	os.MkdirAll(sv, 0o755)
	os.WriteFile(filepath.Join(sv, "RebirthTrack.lua"), []byte(`
RebirthTrackDB = {
	["links"] = {
		"v1|us|Nightslayer|Roguebash|ROGUE|60|Player-6065-044C2423|5|2.5.6|1792530000", -- [1]
		"v1||us||Nightslayer||Mindbugger||PRIEST||58||Player-6065-0411||5||2.5.6||1792530100", -- [2] así lo guarda WoW
		"basura",
	},
}`), 0o644)
	got := Install{Flavor: "_anniversary_", Dir: dir}.LinkRequests()
	if len(got) != 2 || got[0].Name != "Roguebash" || got[0].Realm != "Nightslayer" || got[0].Class != "ROGUE" || got[0].Region != "us" || got[0].Level != 60 {
		t.Fatalf("%+v", got)
	}
	if got[0].Account != "MATIAS#1" {
		t.Fatalf("cuenta: %q", got[0].Account)
	}
	if g := got[1]; g.Name != "Mindbugger" || g.Class != "PRIEST" || g.Level != 58 || g.Raw != "v1|us|Nightslayer|Mindbugger|PRIEST|58|Player-6065-0411|5|2.5.6|1792530100" {
		t.Fatalf("escapado por WoW: %+v", g)
	}
}
