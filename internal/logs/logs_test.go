package logs

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

var raid = Instances{Difficulties: map[int]bool{3: true, 4: true, 14: true, 15: true, 16: true, 1: true}, Maps: map[int]bool{389: true}}

const sample = `10/24/2026 20:55:00.000-3  COMBAT_LOG_VERSION,20,ADVANCED_LOG_ENABLED,1,BUILD_VERSION,2.5.5,PROJECT_ID,5
10/24/2026 20:55:01.000-3  ZONE_CHANGE,530,"Península del Fuego Infernal",0
10/24/2026 20:55:02.000-3  SPELL_DAMAGE,Player-1-A,"Yo",0x511,0x0,Creature-0-1,"Jabalí",0x10a48,0x0,1,"Golpe",1
10/24/2026 21:00:00.000-3  ZONE_CHANGE,532,"Karazhan",3
10/24/2026 21:00:01.000-3  ENCOUNTER_START,652,"Attumen el Montero",3,10,532
10/24/2026 21:00:02.000-3  SPELL_DAMAGE,Player-1-A,"Yo",0x511,0x0,Creature-0-2,"Attumen",0x10a48,0x0,1,"Golpe",1
10/24/2026 22:00:00.000-3  ZONE_CHANGE,0,"Reinos del Este",0
10/24/2026 22:00:01.000-3  SPELL_DAMAGE,Player-1-A,"Yo",0x511,0x0,Creature-0-3,"Lobo",0x10a48,0x0,1,"Golpe",1
`

func TestFilterKeepsOnlyRaid(t *testing.T) {
	ch := Filter([]byte(sample), false, "", raid)
	got := string(ch.Data)
	if !strings.Contains(string(ch.Header), "ADVANCED_LOG_ENABLED,1") {
		t.Errorf("encabezado: %q", ch.Header)
	}
	for _, want := range []string{`ZONE_CHANGE,532,"Karazhan",3`, "ENCOUNTER_START", `"Attumen"`} {
		if !strings.Contains(got, want) {
			t.Errorf("falta %q", want)
		}
	}
	for _, bad := range []string{"Jabalí", "Lobo", "Reinos del Este", "Península"} {
		if strings.Contains(got, bad) {
			t.Errorf("no debería subir %q", bad)
		}
	}
	if ch.Lines != 3 || ch.Skipped != 4 {
		t.Errorf("líneas %d/%d", ch.Lines, ch.Skipped)
	}
	if ch.InRaid || len(ch.Zones) != 1 || ch.Zones[0] != "Karazhan" {
		t.Errorf("estado final %v %v", ch.InRaid, ch.Zones)
	}
}

func TestReadContinuesStateAndPartialLine(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "WoWCombatLog-102426_205500.txt")
	lines := strings.SplitAfter(sample, "\n")
	// Hasta la mitad del primer evento de Karazhan, con la última línea cortada.
	part := strings.Join(lines[:5], "") + lines[5][:20]
	if err := os.WriteFile(p, []byte(part), 0o600); err != nil {
		t.Fatal(err)
	}
	ch, err := Read(p, 0, false, "", raid)
	if err != nil {
		t.Fatal(err)
	}
	if !ch.InRaid || ch.Zone != "Karazhan" {
		t.Fatalf("debería quedar en Karazhan: %+v", ch)
	}
	if ch.OffsetEnd != int64(len(strings.Join(lines[:5], ""))) {
		t.Fatalf("no debe leer la línea a medias: %d", ch.OffsetEnd)
	}
	// El juego termina de escribir: se sigue desde el offset, todavía dentro de la banda.
	if err := os.WriteFile(p, []byte(sample), 0o600); err != nil {
		t.Fatal(err)
	}
	ch2, err := Read(p, ch.OffsetEnd, ch.InRaid, ch.Zone, raid)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(ch2.Data), `"Attumen"`) || strings.Contains(string(ch2.Data), "Lobo") {
		t.Fatalf("tramo inesperado: %s", ch2.Data)
	}
	if ts := FirstTimestamp(p); ts == nil || ts.Hour() != 20 {
		t.Fatalf("fecha de la primera línea: %v", ts)
	}
}

func TestList(t *testing.T) {
	dir := t.TempDir()
	for _, n := range []string{"WoWCombatLog-1.txt", "otra.txt", "WoWCombatLog.txt"} {
		os.WriteFile(filepath.Join(dir, n), []byte("x\n"), 0o600)
	}
	fs, _ := List(dir)
	if len(fs) != 2 {
		t.Fatalf("esperaba 2 logs, hay %d", len(fs))
	}
}

func TestScanSummarizesRaids(t *testing.T) {
	path := filepath.Join(t.TempDir(), "WoWCombatLog.txt")
	if err := os.WriteFile(path, []byte(sample), 0o600); err != nil {
		t.Fatal(err)
	}
	sum, err := Scan(path, int64(len(sample)), raid)
	if err != nil {
		t.Fatal(err)
	}
	if len(sum.Zones) != 1 || sum.Zones[0] != "Karazhan" {
		t.Errorf("bandas: %v", sum.Zones)
	}
	want := int64(len(string(Filter([]byte(sample), false, "", raid).Data)))
	if sum.RaidBytes != want {
		t.Errorf("bytes de banda %d, quería %d", sum.RaidBytes, want)
	}
	if sum, _ := Scan(path, 100, raid); len(sum.Zones) != 0 {
		t.Errorf("con end corto no debería ver bandas: %v", sum.Zones)
	}
}

func TestFilterDungeons(t *testing.T) {
	// Sima Ígnea (TBC Anniversary): la primera vez con dificultad 1, la segunda con 0.
	log := `10/20/2026 17:32:16.307-3  ZONE_CHANGE,1,"Orgrimmar",0
10/20/2026 17:32:17.000-3  SPELL_DAMAGE,Player-1-A,"Yo",0x511,0x0,Creature-0-3,"Peón",0x10a48,0x0,1,"Golpe",1
10/20/2026 17:33:00.728-3  ZONE_CHANGE,389,"Ragefire Chasm",1
10/20/2026 17:33:01.000-3  SPELL_DAMAGE,Player-1-A,"Yo",0x511,0x0,Creature-0-4,"Taragaman",0x10a48,0x0,1,"Golpe",1
10/20/2026 17:40:06.158-3  ZONE_CHANGE,1,"Orgrimmar",0
10/20/2026 18:20:40.819-3  ZONE_CHANGE,389,"Ragefire Chasm",0
10/20/2026 18:20:41.000-3  SPELL_DAMAGE,Player-1-A,"Yo",0x511,0x0,Creature-0-5,"Jergosh",0x10a48,0x0,1,"Golpe",1
10/20/2026 18:25:56.299-3  ZONE_CHANGE,1,"Orgrimmar",0
10/20/2026 18:26:00.000-3  ZONE_CHANGE,489,"Warsong Gulch",0
10/20/2026 18:26:01.000-3  SPELL_DAMAGE,Player-1-A,"Yo",0x511,0x0,Player-2-B,"Alianza",0x548,0x0,1,"Golpe",1
`
	ch := Filter([]byte(log), false, "", raid)
	got := string(ch.Data)
	for _, want := range []string{"Taragaman", "Jergosh"} {
		if !strings.Contains(got, want) {
			t.Errorf("falta %q (mazmorra)", want)
		}
	}
	for _, bad := range []string{"Peón", "Alianza"} {
		if strings.Contains(got, bad) {
			t.Errorf("no debería subir %q", bad)
		}
	}
	if len(ch.Zones) != 1 || ch.Zones[0] != "Ragefire Chasm" {
		t.Errorf("zonas: %v", ch.Zones)
	}
}
