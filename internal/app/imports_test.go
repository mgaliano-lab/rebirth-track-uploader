package app

import (
	"testing"
	"time"

	"github.com/mgaliano-lab/rebirth-track-uploader/internal/logs"
	"github.com/mgaliano-lab/rebirth-track-uploader/internal/state"
)

func TestImportable(t *testing.T) {
	consent := time.Date(2026, 10, 20, 12, 0, 0, 0, time.UTC)
	old := logs.File{Size: 1000, ModTime: consent.Add(-24 * time.Hour)}
	recent := logs.File{Size: 1000, ModTime: consent.Add(time.Hour)}
	cases := []struct {
		name string
		p    *state.FileProgress
		f    logs.File
		end  int64
		ok   bool
	}{
		{"viejo sin ver todavía", nil, old, 1000, true},
		{"nuevo sin ver todavía", nil, recent, 0, false},
		{"salteado con marca", &state.FileProgress{Offset: 800, Skipped: true}, recent, 800, true},
		{"salteado de antes de la marca", &state.FileProgress{Offset: 1000}, old, 1000, true},
		{"leído con el filtro viejo (solo bandas), sin sesión", &state.FileProgress{Offset: 1000}, recent, 1000, true},
		{"leído con el filtro viejo, con sesión", &state.FileProgress{Offset: 600, SessionID: 3, Seq: 2}, recent, 1000, true},
		{"leído con el filtro actual, sin nada", &state.FileProgress{Offset: 1000, Filter: 2}, recent, 0, false},
		{"leído con el filtro actual, con sesión", &state.FileProgress{Offset: 1000, SessionID: 3, Seq: 2, Filter: 2}, old, 0, false},
		{"ya importado", &state.FileProgress{Offset: 400, Imported: true, SessionID: 3}, old, 1000, true},
	}
	for _, c := range cases {
		end, _, ok := importable(c.p, c.f, consent)
		if ok != c.ok || (ok && end != c.end) {
			t.Errorf("%s: got (%d, %v), quería (%d, %v)", c.name, end, ok, c.end, c.ok)
		}
	}
}
