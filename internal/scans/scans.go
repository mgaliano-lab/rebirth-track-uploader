// Package scans es el manifiesto de relevamientos: qué archivos de la carpeta de World of Warcraft
// lee y sube la app, cada cuánto y con qué tope. Lo publica el sitio firmado (GET
// /api/uploader/scans) y la app lo aplica sin release. Es solo configuración: la app lee esos
// archivos y los sube, nunca ejecuta nada de lo que baja.
//
// Las reglas son las mismas que valida el sitio (shared/scans.ts de raidtrack): si se cambian,
// cambiar las dos.
package scans

import (
	"crypto/ed25519"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"
)

// Schema es el formato que entiende esta versión de la app. Un manifiesto con otro formato se
// ignora (la app sigue con el que tenía): cambia solo con un release.
const Schema = 1

const (
	MaxBytes   = 8 << 20 // tope duro por archivo, diga lo que diga el manifiesto
	MaxMatches = 50      // archivos por relevamiento y versión de WoW
	MaxScans   = 20
	MinEvery   = 60
	MaxEvery   = 86400
	MinPoll    = 60
	MaxPoll    = 86400
)

// Extensions: solo archivos de datos.
var Extensions = []string{".lua", ".txt"}

type Scan struct {
	ID           string   `json:"id"`
	Label        string   `json:"label"`
	Flavors      []string `json:"flavors"` // carpetas _x_ o "*" (todas las activas)
	Paths        []string `json:"paths"`   // relativas a la carpeta de la versión, con «/» y comodines * ?
	EverySeconds int      `json:"everySeconds"`
	MaxBytes     int64    `json:"maxBytes"`
	Upload       string   `json:"upload"` // endpoint del mismo sitio, bajo /api/uploader/
}

type Manifest struct {
	Schema        int    `json:"schema"`
	Version       int    `json:"version"`
	Updated       string `json:"updated"`
	MinAppVersion string `json:"minAppVersion,omitempty"`
	PollSeconds   int    `json:"pollSeconds"`
	Scans         []Scan `json:"scans"`
}

// Envelope es lo que manda el sitio: el JSON del manifiesto (en base64) y su firma Ed25519.
type Envelope struct {
	KeyID     string `json:"keyId"`
	Payload   string `json:"payload"`
	Signature string `json:"signature"`
}

// Default es lo que la app relevaba antes del manifiesto (y lo que usa si nunca pudo bajar uno):
// lo que anota el addon Rebirth Track.
var Default = Manifest{
	Schema: Schema, Version: 0, Updated: "", PollSeconds: 900,
	Scans: []Scan{{
		ID:           "rebirthtrack-sv",
		Label:        "Addon Rebirth Track: guild, pedidos de vincular personajes y grupos de banda",
		Flavors:      []string{"*"},
		Paths:        []string{"WTF/Account/*/SavedVariables/RebirthTrack.lua"},
		EverySeconds: 180,
		MaxBytes:     4 << 20,
		Upload:       "/api/uploader/files",
	}},
}

func (s Scan) Every() time.Duration { return time.Duration(s.EverySeconds) * time.Second }

// AppliesTo dice si el relevamiento corre en esa versión de WoW.
func (s Scan) AppliesTo(flavor string) bool {
	for _, f := range s.Flavors {
		if f == "*" || f == flavor {
			return true
		}
	}
	return false
}

func (m Manifest) Poll() time.Duration { return time.Duration(m.PollSeconds) * time.Second }

var (
	idRe      = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{1,40}$`)
	flavorRe  = regexp.MustCompile(`^_[a-z0-9_]+_$`)
	segmentRe = regexp.MustCompile(`^[A-Za-z0-9 _.#*?()+-]+$`)
	uploadRe  = regexp.MustCompile(`^/api/uploader/[a-z0-9][a-z0-9/-]*$`)
	dateRe    = regexp.MustCompile(`^\d{4}-\d{2}-\d{2}$`)
	semverRe  = regexp.MustCompile(`^\d+\.\d+\.\d+$`)
	// Los registros de combate tienen su propio camino (filtrado en la PC: solo bandas y mazmorras).
	denyRe = regexp.MustCompile(`(?i)^wowcombatlog`)
)

// PathProblem dice qué tiene de malo un patrón de ruta (nil = válido): tiene que ser relativo, sin
// «..», con «/», con comodines solo dentro de un tramo y terminar en una extensión de datos.
func PathProblem(p string) error {
	if p == "" || len(p) > 200 {
		return errors.New("vacía o demasiado larga")
	}
	if strings.ContainsAny(p, `\:`) || strings.HasPrefix(p, "/") {
		return errors.New("tiene que ser relativa y con «/»")
	}
	if strings.Contains(p, "**") {
		return errors.New("«**» no está soportado")
	}
	segs := strings.Split(p, "/")
	if len(segs) > 8 {
		return errors.New("demasiados niveles")
	}
	for _, s := range segs {
		if s == "" || s == "." || s == ".." {
			return errors.New("tramo vacío, «.» o «..»")
		}
		if !segmentRe.MatchString(s) {
			return fmt.Errorf("caracteres no permitidos en «%s»", s)
		}
		if denyRe.MatchString(s) {
			return errors.New("los registros de combate no se relevan por acá")
		}
	}
	if !HasDataExt(segs[len(segs)-1]) {
		return fmt.Errorf("extensión no permitida (solo %s)", strings.Join(Extensions, ", "))
	}
	return nil
}

// HasDataExt: el nombre termina en una extensión permitida (la literal, no un comodín).
func HasDataExt(name string) bool {
	n := strings.ToLower(name)
	for _, e := range Extensions {
		if strings.HasSuffix(n, e) {
			return true
		}
	}
	return false
}

// Validate revisa todo el manifiesto. Uno inválido se descarta entero (no se aplica a medias).
func (m Manifest) Validate() error {
	var out []string
	add := func(f string, a ...any) { out = append(out, fmt.Sprintf(f, a...)) }
	if m.Schema != Schema {
		add("formato %d (esta app entiende el %d)", m.Schema, Schema)
	}
	if m.Version < 1 {
		add("versión %d", m.Version)
	}
	if !dateRe.MatchString(m.Updated) {
		add("fecha inválida")
	}
	if m.MinAppVersion != "" && !semverRe.MatchString(m.MinAppVersion) {
		add("minAppVersion inválida")
	}
	if m.PollSeconds < MinPoll || m.PollSeconds > MaxPoll {
		add("pollSeconds fuera de rango")
	}
	if len(m.Scans) > MaxScans {
		add("demasiados relevamientos")
	}
	ids := map[string]bool{}
	for _, s := range m.Scans {
		if !idRe.MatchString(s.ID) || ids[s.ID] {
			add("id inválido o repetido: %q", s.ID)
		}
		ids[s.ID] = true
		if strings.TrimSpace(s.Label) == "" || len(s.Label) > 120 {
			add("%s: label vacío o largo", s.ID)
		}
		if len(s.Flavors) == 0 {
			add("%s: sin versiones de WoW", s.ID)
		}
		for _, f := range s.Flavors {
			if f != "*" && !flavorRe.MatchString(f) {
				add("%s: versión inválida %q", s.ID, f)
			}
		}
		if len(s.Paths) == 0 || len(s.Paths) > 10 {
			add("%s: entre 1 y 10 rutas", s.ID)
		}
		for _, p := range s.Paths {
			if err := PathProblem(p); err != nil {
				add("%s: ruta %q: %v", s.ID, p, err)
			}
		}
		if s.EverySeconds < MinEvery || s.EverySeconds > MaxEvery {
			add("%s: everySeconds fuera de rango", s.ID)
		}
		if s.MaxBytes <= 0 || s.MaxBytes > MaxBytes {
			add("%s: maxBytes fuera de rango", s.ID)
		}
		if !uploadRe.MatchString(s.Upload) {
			add("%s: upload tiene que ser /api/uploader/…", s.ID)
		}
	}
	if len(out) > 0 {
		return errors.New(strings.Join(out, "; "))
	}
	return nil
}

var ErrSignature = errors.New("la firma del manifiesto no es válida")

// Open verifica la firma con alguna de las claves de confianza (Ed25519, 32 bytes en base64) y
// valida el manifiesto. No mira la versión de la app ni la del manifiesto: eso lo decide quien lo usa.
func Open(env Envelope, trusted []string) (Manifest, error) {
	payload, err1 := base64.StdEncoding.DecodeString(env.Payload)
	sig, err2 := base64.StdEncoding.DecodeString(env.Signature)
	if err1 != nil || err2 != nil || len(sig) != ed25519.SignatureSize {
		return Manifest{}, ErrSignature
	}
	ok := false
	for _, k := range trusted {
		pub, err := base64.StdEncoding.DecodeString(strings.TrimSpace(k))
		if err == nil && len(pub) == ed25519.PublicKeySize && ed25519.Verify(ed25519.PublicKey(pub), payload, sig) {
			ok = true
			break
		}
	}
	if !ok {
		return Manifest{}, ErrSignature
	}
	var m Manifest
	if err := json.Unmarshal(payload, &m); err != nil {
		return Manifest{}, fmt.Errorf("manifiesto ilegible: %w", err)
	}
	if err := m.Validate(); err != nil {
		return Manifest{}, fmt.Errorf("manifiesto inválido: %w", err)
	}
	return m, nil
}
