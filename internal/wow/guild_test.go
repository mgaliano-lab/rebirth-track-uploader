package wow

import (
	"os"
	"path/filepath"
	"testing"
)

func TestGuilds(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "_classic_era_")
	sv := filepath.Join(dir, "WTF", "Account", "MATIAS#1", "SavedVariables")
	os.MkdirAll(sv, 0o755)
	write := func(last string) {
		os.WriteFile(filepath.Join(sv, "RebirthTrack.lua"), []byte(`
RebirthTrackDB = {
	["guild"] = {
		["info"] = "gi1;Rebirth Knights;Nightslayer;us;2;1792530000",
		["members"] = {
			"m1;Roguebash;Nightslayer;ROGUE;60;1;Oficial;Player-6065-044C2423;0", -- [1]
			"m1;Healz;Nightslayer;PRIEST;58;3;Miembro;Player-6065-0411;`+last+`", -- [2]
			"m1;roto",
		},
		["known"] = {
			"k1;1792530100;L;us;Nightslayer;Roguebash;ROGUE;60;Player-6065-044C2423;2", -- [1]
			"k1;1792530100;X;us;Nightslayer;Malo;MAGE;1;;2",
		},
	},
}`), 0o644)
	}
	write("183600")
	in := Install{Flavor: "_classic_era_", Dir: dir}
	got := in.Guilds()
	if len(got) != 1 {
		t.Fatalf("%+v", got)
	}
	g := got[0]
	if g.Name != "Rebirth Knights" || g.Realm != "Nightslayer" || g.Region != "us" || g.Project != 2 || len(g.Members) != 2 || len(g.Known) != 1 {
		t.Fatalf("%+v", g)
	}
	if m := g.Members[1]; m.Name != "Healz" || m.Rank != 3 || m.RankName != "Miembro" || m.LastOnline != 183600 || m.Class != "PRIEST" {
		t.Fatalf("%+v", m)
	}
	if k := g.Known[0]; !k.Linking || k.Name != "Roguebash" || k.Level != 60 || k.GUID != "Player-6065-044C2423" {
		t.Fatalf("%+v", k)
	}
	// Unas horas más de desconexión no cambian el contenido; un día más sí.
	write("190800")
	if in.Guilds()[0].Hash != g.Hash {
		t.Fatal("cambió el hash por unas horas")
	}
	write("270000")
	if in.Guilds()[0].Hash == g.Hash {
		t.Fatal("no cambió el hash con un día más")
	}
}
