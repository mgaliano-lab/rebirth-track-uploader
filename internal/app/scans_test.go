package app

import (
	"compress/gzip"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/mgaliano-lab/rebirth-track-uploader/internal/scans"
	"github.com/mgaliano-lab/rebirth-track-uploader/internal/state"
)

// Sitio de mentira: publica un manifiesto firmado (que el test cambia) y recibe archivos.
type fakeSite struct {
	mu       sync.Mutex
	priv     ed25519.PrivateKey
	manifest scans.Manifest
	gets     int
	notMod   int
	uploads  []string // "flavor|scan|ruta|contenido"
	reject   bool
}

func (f *fakeSite) envelope() ([]byte, string) {
	b, _ := json.Marshal(f.manifest)
	env, _ := json.Marshal(scans.Envelope{KeyID: "t", Payload: base64.StdEncoding.EncodeToString(b), Signature: base64.StdEncoding.EncodeToString(ed25519.Sign(f.priv, b))})
	sum := sha256.Sum256(b)
	return env, `"` + hex.EncodeToString(sum[:8]) + `"`
}

func (f *fakeSite) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	switch r.URL.Path {
	case "/api/uploader/config":
		w.Write([]byte(`{"userName":"Tester","deviceName":"PC"}`))
	case "/api/uploader/scans":
		f.gets++
		env, etag := f.envelope()
		if r.Header.Get("If-None-Match") == etag {
			f.notMod++
			w.WriteHeader(http.StatusNotModified)
			return
		}
		w.Header().Set("ETag", etag)
		w.Write(env)
	case "/api/uploader/files":
		if r.Header.Get("Authorization") != "Bearer tok" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		if f.reject {
			w.WriteHeader(http.StatusBadRequest)
			w.Write([]byte(`{"message":"El sitio no pide ese archivo"}`))
			return
		}
		zr, _ := gzip.NewReader(r.Body)
		b, _ := io.ReadAll(zr)
		sum := sha256.Sum256(b)
		p, _ := url.PathUnescape(r.Header.Get("X-Path"))
		if r.Header.Get("X-Sha256") != hex.EncodeToString(sum[:]) {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		f.uploads = append(f.uploads, fmt.Sprintf("%s|%s|%s|%s", r.Header.Get("X-Flavor"), r.Header.Get("X-Scan-Id"), p, b))
		w.WriteHeader(http.StatusCreated)
		w.Write([]byte(`{"ok":true,"messages":["Personaje Roguebash-Nightslayer vinculado a tu cuenta."]}`))
	default:
		http.NotFound(w, r)
	}
}

func (f *fakeSite) took() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := f.uploads
	f.uploads = nil
	return out
}

func newScanApp(t *testing.T) (*App, *fakeSite, string, func()) {
	t.Helper()
	pub, priv, _ := ed25519.GenerateKey(rand.Reader)
	site := &fakeSite{priv: priv, manifest: scans.Default}
	site.manifest.Version, site.manifest.Updated = 1, "2026-10-22"
	srv := httptest.NewServer(site)
	root := filepath.Join(t.TempDir(), "World of Warcraft")
	sv := filepath.Join(root, "_anniversary_", "WTF", "Account", "MATIAS#1", "SavedVariables")
	os.MkdirAll(sv, 0o755)
	os.WriteFile(filepath.Join(sv, "RebirthTrack.lua"), []byte("RebirthTrackDB = {}"), 0o644)
	os.WriteFile(filepath.Join(sv, "Otro.lua"), []byte("OtroDB = {}"), 0o644)
	st, err := state.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	st.Update(func(s *state.State) { s.ConsentAt = &now; s.Token = "tok"; s.WowRoot = root })
	a := New("0.3.0", srv.URL, st, nil)
	a.ExtraScanKeys = []string{base64.StdEncoding.EncodeToString(pub)}
	return a, site, sv, srv.Close
}

func TestScansSyncAndUpload(t *testing.T) {
	a, site, sv, done := newScanApp(t)
	defer done()
	a.loadScans()
	if st := a.ScansStatus(); st.Source != "incluido" || st.Version != 0 {
		t.Fatalf("sin caché arranca con el incluido: %+v", st)
	}
	a.SyncScans()
	if st := a.ScansStatus(); st.Source != "sitio" || st.Version != 1 || st.Error != "" {
		t.Fatalf("%+v", st)
	}

	// Sube RebirthTrack.lua (no Otro.lua) y la segunda vez, sin cambios, no sube nada.
	a.RunScans(true)
	if got := site.took(); len(got) != 1 || got[0] != "_anniversary_|rebirthtrack-sv|WTF/Account/MATIAS#1/SavedVariables/RebirthTrack.lua|RebirthTrackDB = {}" {
		t.Fatalf("%v", got)
	}
	a.RunScans(true)
	if got := site.took(); len(got) != 0 {
		t.Fatalf("subió sin cambios: %v", got)
	}
	if !strings.Contains(fmt.Sprint(a.Status().Activity), "Roguebash") {
		t.Fatal("no mostró lo que contestó el sitio")
	}

	// El sitio cambia el manifiesto (suma Otro.lua): la app lo toma sin reiniciar y lo sube.
	a.SyncScans()
	if site.notMod != 1 {
		t.Fatalf("sin cambios tenía que ser 304 (ETag): %d", site.notMod)
	}
	site.mu.Lock()
	site.manifest.Version = 2
	site.manifest.Scans = append(site.manifest.Scans, scans.Scan{ID: "otro-addon", Label: "Otro addon", Flavors: []string{"_anniversary_"}, Paths: []string{"WTF/Account/*/SavedVariables/Otro.lua"}, EverySeconds: 600, MaxBytes: 1024, Upload: "/api/uploader/files"})
	site.mu.Unlock()
	a.SyncScans()
	if st := a.ScansStatus(); st.Version != 2 || len(st.Scans) != 2 {
		t.Fatalf("%+v", st)
	}
	if !a.anyScanDue() {
		t.Fatal("con un manifiesto nuevo tienen que correr todos")
	}
	a.RunScans(false)
	if got := site.took(); len(got) != 1 || !strings.Contains(got[0], "otro-addon|WTF/Account/MATIAS#1/SavedVariables/Otro.lua|OtroDB") {
		t.Fatalf("%v", got)
	}

	// Cambia el archivo: se vuelve a subir.
	os.WriteFile(filepath.Join(sv, "RebirthTrack.lua"), []byte("RebirthTrackDB = {1}"), 0o644)
	a.RunScans(true)
	if got := site.took(); len(got) != 1 || !strings.HasSuffix(got[0], "{1}") {
		t.Fatalf("%v", got)
	}

	// Más grande que el tope: no se sube y el panel lo dice.
	os.WriteFile(filepath.Join(sv, "Otro.lua"), []byte(strings.Repeat("x", 2048)), 0o644)
	a.RunScans(true)
	if got := site.took(); len(got) != 0 {
		t.Fatalf("subió uno grande: %v", got)
	}
	var big bool
	for _, s := range a.ScansStatus().Scans {
		for _, f := range s.Files {
			big = big || f.State == "grande"
		}
	}
	if !big {
		t.Fatalf("%+v", a.ScansStatus())
	}

	// El sitio vuelve a una versión anterior: se ignora.
	site.mu.Lock()
	site.manifest.Version = 1
	site.mu.Unlock()
	a.SyncScans()
	if st := a.ScansStatus(); st.Version != 2 || !strings.Contains(st.Error, "anterior") {
		t.Fatalf("%+v", st)
	}

	// Al reiniciar arranca con la caché (versión 2), aunque el sitio no conteste.
	b := New("0.3.0", "http://127.0.0.1:1", a.Store, nil)
	b.ExtraScanKeys = a.ExtraScanKeys
	b.loadScans()
	if st := b.ScansStatus(); st.Source != "caché" || st.Version != 2 {
		t.Fatalf("%+v", st)
	}
	// Sin la clave en la que confiaba, la caché no vale.
	c := New("0.3.0", "http://127.0.0.1:1", a.Store, nil)
	c.loadScans()
	if st := c.ScansStatus(); st.Source != "incluido" {
		t.Fatalf("%+v", st)
	}
}

func TestScansRules(t *testing.T) {
	a, site, _, done := newScanApp(t)
	defer done()
	a.loadScans()

	// Necesita una app más nueva: se queda con el que tenía.
	site.mu.Lock()
	site.manifest.MinAppVersion = "0.9.0"
	site.mu.Unlock()
	a.SyncScans()
	if st := a.ScansStatus(); st.Source != "incluido" || !strings.Contains(st.Error, "v0.9.0") {
		t.Fatalf("%+v", st)
	}

	// Mal firmado: igual.
	site.mu.Lock()
	site.manifest.MinAppVersion = ""
	_, site.priv, _ = ed25519.GenerateKey(rand.Reader)
	site.mu.Unlock()
	a.SyncScans()
	if st := a.ScansStatus(); st.Source != "incluido" || !strings.Contains(st.Error, "firma") {
		t.Fatalf("%+v", st)
	}

	// Sin consentimiento, en pausa o con la versión de WoW desactivada: no lee nada.
	a.Store.Update(func(s *state.State) { s.Paused = true })
	a.RunScans(true)
	a.Store.Update(func(s *state.State) { s.Paused = false; s.Disabled["_anniversary_"] = true })
	a.RunScans(true)
	if got := site.took(); len(got) != 0 {
		t.Fatalf("%v", got)
	}

	// Rechazo definitivo (4xx): no se reintenta hasta que el archivo cambie.
	a.Store.Update(func(s *state.State) { s.Disabled["_anniversary_"] = false })
	site.mu.Lock()
	site.reject = true
	site.mu.Unlock()
	a.RunScans(true)
	if v := a.Store.Read().ScansSent; len(v) != 1 {
		t.Fatalf("%v", v)
	}
	if f := a.ScansStatus().Scans[0].Files; len(f) != 1 || f[0].State != "rechazado" {
		t.Fatalf("%+v", f)
	}
}

func TestDefinitive(t *testing.T) {
	for code, want := range map[int]bool{400: true, 413: true, 404: false, 429: false, 408: false, 500: false, 502: false, 0: false} {
		if definitive(code) != want {
			t.Errorf("%d: %v", code, !want)
		}
	}
}
