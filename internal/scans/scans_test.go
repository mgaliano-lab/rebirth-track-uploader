package scans

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func valid() Manifest {
	m := Default
	m.Version, m.Updated = 1, "2026-10-22"
	return m
}

func TestDefaultIsValid(t *testing.T) {
	if err := valid().Validate(); err != nil {
		t.Fatal(err)
	}
}

func TestPathProblem(t *testing.T) {
	for _, ok := range []string{"WTF/Account/*/SavedVariables/RebirthTrack.lua", "WTF/Account/*/*/*/SavedVariables/X.lua", "Logs/Notas?.txt"} {
		if err := PathProblem(ok); err != nil {
			t.Errorf("%s: %v", ok, err)
		}
	}
	for _, bad := range []string{
		"../WTF/x.lua", "WTF/../../x.lua", "./x.lua", "/etc/x.lua", "C:/x.lua", `WTF\Account\x.lua`, "WTF//x.lua",
		"WTF/**/x.lua", "WTF/Config.wtf", "Interface/AddOns/X/X.dll", "WTF/x.exe", "WTF/*", "WTF/x.l?a", "Logs/WoWCombatLog*.txt",
		"WTF/[ab].lua", "", "a/b/c/d/e/f/g/h/i.lua",
	} {
		if PathProblem(bad) == nil {
			t.Errorf("%q debería ser inválida", bad)
		}
	}
}

func TestValidate(t *testing.T) {
	m := valid()
	m.Scans = append([]Scan{}, m.Scans...)
	m.Scans[0].Upload = "https://otro.sitio/x"
	m.Scans[0].MaxBytes = 1 << 30
	m.Scans = append(m.Scans, m.Scans[0]) // id repetido
	if err := m.Validate(); err == nil || !strings.Contains(err.Error(), "upload") || !strings.Contains(err.Error(), "maxBytes") || !strings.Contains(err.Error(), "repetido") {
		t.Fatalf("%v", err)
	}
	m = valid()
	m.Schema = 2
	if m.Validate() == nil {
		t.Fatal("aceptó un formato que no conoce")
	}
}

func sign(t *testing.T, m Manifest, priv ed25519.PrivateKey) Envelope {
	b, _ := json.Marshal(m)
	return Envelope{KeyID: "x", Payload: base64.StdEncoding.EncodeToString(b), Signature: base64.StdEncoding.EncodeToString(ed25519.Sign(priv, b))}
}

func TestOpen(t *testing.T) {
	pub, priv, _ := ed25519.GenerateKey(rand.Reader)
	_, other, _ := ed25519.GenerateKey(rand.Reader)
	keys := []string{base64.StdEncoding.EncodeToString(pub)}
	env := sign(t, valid(), priv)
	m, err := Open(env, keys)
	if err != nil || m.Version != 1 || m.Scans[0].ID != "rebirthtrack-sv" {
		t.Fatalf("%+v %v", m, err)
	}
	if _, err := Open(sign(t, valid(), other), keys); !errors.Is(err, ErrSignature) {
		t.Fatalf("firmado con otra clave: %v", err)
	}
	// Payload cambiado (misma firma).
	b, _ := base64.StdEncoding.DecodeString(env.Payload)
	env2 := env
	env2.Payload = base64.StdEncoding.EncodeToString([]byte(strings.Replace(string(b), "RebirthTrack.lua", "RebirthTrack.lux", 1)))
	if _, err := Open(env2, keys); !errors.Is(err, ErrSignature) {
		t.Fatalf("payload alterado: %v", err)
	}
	// Bien firmado pero con una ruta peligrosa: se descarta igual.
	bad := valid()
	bad.Scans = []Scan{bad.Scans[0]}
	bad.Scans[0].Paths = []string{"../../Windows/win.ini.txt"}
	if _, err := Open(sign(t, bad, priv), keys); err == nil || errors.Is(err, ErrSignature) {
		t.Fatalf("ruta peligrosa: %v", err)
	}
	if _, err := Open(env, nil); !errors.Is(err, ErrSignature) {
		t.Fatal("sin claves de confianza no se acepta nada")
	}
}

func write(t *testing.T, p, s string) {
	t.Helper()
	os.MkdirAll(filepath.Dir(p), 0o755)
	if err := os.WriteFile(p, []byte(s), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestFind(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "_anniversary_")
	write(t, filepath.Join(dir, "WTF", "Account", "MATIAS#1", "SavedVariables", "RebirthTrack.lua"), "a")
	write(t, filepath.Join(dir, "WTF", "Account", "OTRA#2", "SavedVariables", "rebirthtrack.lua"), "b") // mayúsculas como en Windows
	write(t, filepath.Join(dir, "WTF", "Account", "MATIAS#1", "SavedVariables", "Otro.lua"), "c")
	os.MkdirAll(filepath.Join(dir, "WTF", "Account", "DIR#3", "SavedVariables", "RebirthTrack.lua"), 0o755) // carpeta, no archivo
	// Afuera de la carpeta de la versión: un enlace no debe llevar ahí.
	write(t, filepath.Join(root, "secreto", "SavedVariables", "RebirthTrack.lua"), "secreto")
	if runtime.GOOS != "windows" {
		os.Symlink(filepath.Join(root, "secreto"), filepath.Join(dir, "WTF", "Account", "LINK#4"))
		os.Symlink(filepath.Join(root, "secreto", "SavedVariables", "RebirthTrack.lua"), filepath.Join(dir, "WTF", "Account", "OTRA#2", "SavedVariables", "Enlace.lua"))
	}
	got, err := Find(dir, "WTF/Account/*/SavedVariables/RebirthTrack.lua")
	if err != nil {
		t.Fatal(err)
	}
	var rels []string
	for _, m := range got {
		rels = append(rels, m.Rel)
	}
	if strings.Join(rels, ",") != "WTF/Account/MATIAS#1/SavedVariables/RebirthTrack.lua,WTF/Account/OTRA#2/SavedVariables/rebirthtrack.lua" {
		t.Fatalf("%v", rels)
	}
	if got, _ := Find(dir, "WTF/Account/*/SavedVariables/*.lua"); len(got) != 3 {
		t.Fatalf("*.lua: %+v (no tiene que seguir el enlace)", got)
	}
	if _, err := Find(dir, "../secreto/SavedVariables/RebirthTrack.lua"); err == nil {
		t.Fatal("aceptó «..»")
	}
	if got, err := Find(filepath.Join(root, "no-existe"), "WTF/x.lua"); err != nil || got != nil {
		t.Fatalf("carpeta inexistente: %v %v", got, err)
	}
	// Tope de tamaño.
	if _, err := Read(Match{Path: got0(t, dir), Size: 1}, 0); err == nil {
		t.Fatal("tope 0")
	}
	if b, err := Read(Match{Path: got0(t, dir), Size: 1}, 10); err != nil || string(b) != "a" {
		t.Fatalf("%q %v", b, err)
	}
	var big ErrTooBig
	if _, err := Read(Match{Path: got0(t, dir), Size: 100}, 10); !errors.As(err, &big) {
		t.Fatalf("%v", err)
	}
}

func got0(t *testing.T, dir string) string {
	return filepath.Join(dir, "WTF", "Account", "MATIAS#1", "SavedVariables", "RebirthTrack.lua")
}
