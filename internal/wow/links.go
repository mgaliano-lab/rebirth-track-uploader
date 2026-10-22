package wow

import (
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
)

// LinkRequest es un personaje que el jugador pidió vincular desde el addon (botón «Vincular a
// Rebirth Track» o /rt vincular). El addon lo deja en RebirthTrackDB.links, que WoW guarda en
// WTF\Account\<cuenta>\SavedVariables\RebirthTrack.lua al recargar o salir.
type LinkRequest struct {
	Raw     string `json:"-"` // el registro tal cual (para no mandarlo dos veces)
	Region  string `json:"region"`
	Realm   string `json:"realm"`
	Name    string `json:"name"`
	Class   string `json:"class"` // token en inglés: WARRIOR, DRUID…
	Level   int    `json:"level"`
	GUID    string `json:"guid"`
	Project int    `json:"project"` // WOW_PROJECT_ID
	Build   string `json:"build"`
	Account string `json:"account"` // carpeta WTF\Account\<cuenta> (ej. 12345678#1: el #N es la licencia WoW1, WoW2…)
}

// Al guardar las SavedVariables, WoW escapa cada «|» como «||» (es el carácter de escape de
// sus textos): en disco el pedido queda "v1||us||Reino||…". Se aceptan las dos formas.
var linkRe = regexp.MustCompile(`"(v1\|\|?[^"\n]*)"`)

// LinkRequests lee los pedidos de todas las cuentas de esta versión de WoW.
func (i Install) LinkRequests() []LinkRequest {
	files, _ := filepath.Glob(filepath.Join(i.Dir, "WTF", "Account", "*", "SavedVariables", "RebirthTrack.lua"))
	var out []LinkRequest
	for _, f := range files {
		b, err := os.ReadFile(f)
		if err != nil {
			continue
		}
		account := filepath.Base(filepath.Dir(filepath.Dir(f)))
		for _, m := range linkRe.FindAllStringSubmatch(string(b), -1) {
			if r, ok := parseLink(strings.ReplaceAll(m[1], "||", "|")); ok {
				r.Account = account
				out = append(out, r)
			}
		}
	}
	return out
}

func parseLink(raw string) (LinkRequest, bool) {
	p := strings.Split(raw, "|")
	if len(p) < 10 || p[0] != "v1" || p[3] == "" || p[2] == "" {
		return LinkRequest{}, false
	}
	lvl, _ := strconv.Atoi(p[5])
	proj, _ := strconv.Atoi(p[7])
	unesc := func(s string) string { return strings.ReplaceAll(strings.ReplaceAll(s, `\\`, `\`), `\'`, `'`) }
	return LinkRequest{Raw: raw, Region: p[1], Realm: unesc(p[2]), Name: unesc(p[3]), Class: p[4], Level: lvl, GUID: p[6], Project: proj, Build: p[8]}, true
}
