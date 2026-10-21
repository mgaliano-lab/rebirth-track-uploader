package wow

import (
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
)

// GroupSnapshot son los grupos de banda que anotó el addon al entrar a una instancia o empezar un
// jefe (el registro de combate no los trae): "g1;hora;GUID=grupo,GUID=grupo…" en RebirthTrackDB.groups.
type GroupSnapshot struct {
	Raw     string        `json:"-"`
	At      int64         `json:"at"` // segundos epoch (reloj de la PC)
	Members []GroupMember `json:"members"`
}

type GroupMember struct {
	GUID  string `json:"guid"`
	Group int    `json:"group"`
}

var groupRe = regexp.MustCompile(`"(g1;\d+;[^"\n]*)"`)

// GroupSnapshots lee los grupos anotados en todas las cuentas de esta versión de WoW.
func (i Install) GroupSnapshots() []GroupSnapshot {
	files, _ := filepath.Glob(filepath.Join(i.Dir, "WTF", "Account", "*", "SavedVariables", "RebirthTrack.lua"))
	var out []GroupSnapshot
	for _, f := range files {
		b, err := os.ReadFile(f)
		if err != nil {
			continue
		}
		for _, m := range groupRe.FindAllStringSubmatch(string(b), -1) {
			if s, ok := parseGroups(m[1]); ok {
				out = append(out, s)
			}
		}
	}
	return out
}

func parseGroups(raw string) (GroupSnapshot, bool) {
	p := strings.SplitN(raw, ";", 3)
	if len(p) != 3 {
		return GroupSnapshot{}, false
	}
	at, err := strconv.ParseInt(p[1], 10, 64)
	if err != nil {
		return GroupSnapshot{}, false
	}
	s := GroupSnapshot{Raw: raw, At: at}
	for _, m := range strings.Split(p[2], ",") {
		guid, g, ok := strings.Cut(m, "=")
		n, err := strconv.Atoi(g)
		if ok && err == nil && n >= 1 && n <= 8 && strings.HasPrefix(guid, "Player-") {
			s.Members = append(s.Members, GroupMember{GUID: guid, Group: n})
		}
	}
	return s, len(s.Members) > 0
}
