package wow

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
)

// GuildSnapshot es la guild que anotó el addon en RebirthTrackDB.guild: la lista de miembros que
// muestra el juego y los personajes que otros addons anunciaron por el canal de la guild. Se sube
// para que el sitio tenga a todos los miembros (sin dueño: cada uno pide vincularse en la web).
type GuildSnapshot struct {
	Hash    string        `json:"-"` // del contenido, para no mandar dos veces lo mismo
	Name    string        `json:"name"`
	Realm   string        `json:"realm"`
	Region  string        `json:"region"`
	Project int           `json:"project"`
	At      int64         `json:"at"`
	Members []GuildMember `json:"members"`
	Known   []KnownChar   `json:"known"`
}

// GuildMember es una fila de la lista de miembros: "m1;nombre;reino;clase;nivel;rango;nombre del
// rango;guid;segundos desde la última conexión" (0 = conectado).
type GuildMember struct {
	Name       string `json:"name"`
	Realm      string `json:"realm"`
	Class      string `json:"class"`
	Level      int    `json:"level"`
	Rank       int    `json:"rank"`
	RankName   string `json:"rankName"`
	GUID       string `json:"guid,omitempty"`
	LastOnline int64  `json:"lastOnline"`
}

// KnownChar es un personaje anunciado por el canal: "k1;hora;S|L;región;reino;nombre;clase;nivel;guid;proyecto".
type KnownChar struct {
	At      int64  `json:"at"`
	Linking bool   `json:"linking"` // pidió vincular desde el addon (L)
	Region  string `json:"region"`
	Realm   string `json:"realm"`
	Name    string `json:"name"`
	Class   string `json:"class"`
	Level   int    `json:"level"`
	GUID    string `json:"guid,omitempty"`
	Project int    `json:"project"`
}

var (
	guildInfoRe   = regexp.MustCompile(`"(gi1;[^"\n]*)"`)
	guildMemberRe = regexp.MustCompile(`"(m1;[^"\n]*)"`)
	guildKnownRe  = regexp.MustCompile(`"(k1;[^"\n]*)"`)
)

// Guilds lee la guild anotada en cada cuenta de esta versión de WoW (una por cuenta).
func (i Install) Guilds() []GuildSnapshot {
	files, _ := filepath.Glob(filepath.Join(i.Dir, "WTF", "Account", "*", "SavedVariables", "RebirthTrack.lua"))
	var out []GuildSnapshot
	for _, f := range files {
		b, err := os.ReadFile(f)
		if err != nil {
			continue
		}
		if g, ok := parseGuild(string(b)); ok {
			out = append(out, g)
		}
	}
	return out
}

func unescLua(s string) string {
	return strings.ReplaceAll(strings.ReplaceAll(s, `\\`, `\`), `\'`, `'`)
}

func parseGuild(src string) (GuildSnapshot, bool) {
	m := guildInfoRe.FindStringSubmatch(src)
	if m == nil {
		return GuildSnapshot{}, false
	}
	p := strings.Split(unescLua(m[1]), ";")
	if len(p) < 6 || p[1] == "" {
		return GuildSnapshot{}, false
	}
	proj, _ := strconv.Atoi(p[4])
	at, _ := strconv.ParseInt(p[5], 10, 64)
	g := GuildSnapshot{Name: p[1], Realm: p[2], Region: p[3], Project: proj, At: at}
	h := sha256.New()
	h.Write([]byte(p[1] + ";" + p[2]))
	for _, mm := range guildMemberRe.FindAllStringSubmatch(src, -1) {
		f := strings.Split(unescLua(mm[1]), ";")
		if len(f) < 9 || f[1] == "" || f[2] == "" {
			continue
		}
		lvl, _ := strconv.Atoi(f[4])
		rank, _ := strconv.Atoi(f[5])
		last, _ := strconv.ParseInt(f[8], 10, 64)
		g.Members = append(g.Members, GuildMember{Name: f[1], Realm: f[2], Class: f[3], Level: lvl, Rank: rank, RankName: f[6], GUID: f[7], LastOnline: last})
		// La última conexión cambia a cada rato: se redondea a días para no resubir por eso.
		h.Write([]byte(strings.Join(f[1:8], ";") + ";" + strconv.FormatInt(last/86400, 10) + "\n"))
	}
	for _, km := range guildKnownRe.FindAllStringSubmatch(src, -1) {
		f := strings.Split(unescLua(km[1]), ";")
		if len(f) < 10 || (f[2] != "S" && f[2] != "L") || f[4] == "" || f[5] == "" {
			continue
		}
		at, _ := strconv.ParseInt(f[1], 10, 64)
		lvl, _ := strconv.Atoi(f[7])
		proj, _ := strconv.Atoi(f[9])
		g.Known = append(g.Known, KnownChar{At: at, Linking: f[2] == "L", Region: f[3], Realm: f[4], Name: f[5], Class: f[6], Level: lvl, GUID: f[8], Project: proj})
		h.Write([]byte(km[1] + "\n"))
	}
	g.Hash = hex.EncodeToString(h.Sum(nil))
	return g, len(g.Members) > 0 || len(g.Known) > 0
}
