package wow

import (
	"os"
	"path/filepath"
	"testing"
)

func TestGroupSnapshots(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "_anniversary_")
	sv := filepath.Join(dir, "WTF", "Account", "MATIAS#1", "SavedVariables")
	os.MkdirAll(sv, 0o755)
	os.WriteFile(filepath.Join(sv, "RebirthTrack.lua"), []byte(`
RebirthTrackDB = {
	["groups"] = {
		"g1;1792530000;Player-6065-044C2423=1,Player-6065-0411=2,raro=3,Player-1-X=9", -- [1]
		"g1;roto;Player-1-A=1",
	},
}`), 0o644)
	got := Install{Flavor: "_anniversary_", Dir: dir}.GroupSnapshots()
	if len(got) != 1 || got[0].At != 1792530000 || len(got[0].Members) != 2 || got[0].Members[1] != (GroupMember{GUID: "Player-6065-0411", Group: 2}) {
		t.Fatalf("%+v", got)
	}
}
