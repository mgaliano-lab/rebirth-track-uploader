package update

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
)

func TestNewer(t *testing.T) {
	for _, c := range []struct {
		l, c string
		want bool
	}{
		{"0.1.2", "0.1.1", true}, {"0.2.0", "0.1.9", true}, {"1.0.0", "0.9.9", true},
		{"0.1.1", "0.1.1", false}, {"0.1.0", "0.1.1", false},
		{"0.1.2", "dev", false}, {"0.1.2", "0.1.1-dev.abc", false}, {"basura", "0.1.1", false},
	} {
		if got := Newer(c.l, c.c); got != c.want {
			t.Errorf("Newer(%q, %q) = %v", c.l, c.c, got)
		}
	}
}

func TestDownloadVerifies(t *testing.T) {
	good := []byte("instalador")
	sum := sha256.Sum256(good)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/sums":
			fmt.Fprintf(w, "%s  App-1.exe\n%s  Mala-1.exe\n", hex.EncodeToString(sum[:]), hex.EncodeToString(sum[:]))
		case "/App-1.exe":
			w.Write(good)
		case "/Mala-1.exe":
			w.Write([]byte("manipulado"))
		}
	}))
	defer srv.Close()
	r := Release{Version: "1", sums: &Asset{Name: "SHA256SUMS.txt", URL: srv.URL + "/sums"}}
	dir := t.TempDir()
	p, err := Download(r, &Asset{Name: "App-1.exe", URL: srv.URL + "/App-1.exe"}, dir)
	if err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(p); string(b) != "instalador" {
		t.Errorf("contenido %q", b)
	}
	if _, err := Download(r, &Asset{Name: "Mala-1.exe", URL: srv.URL + "/Mala-1.exe"}, dir); err == nil {
		t.Error("debería rechazar un archivo que no coincide con su SHA-256")
	}
	if _, err := os.Stat(dir + "/Mala-1.exe"); err == nil {
		t.Error("no debería dejar el archivo malo")
	}
}
