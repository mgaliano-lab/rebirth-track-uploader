package app

import (
	"archive/zip"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/mgaliano-lab/rebirth-track-uploader/internal/api"
	"github.com/mgaliano-lab/rebirth-track-uploader/internal/state"
)

// CDN de mentira con latest.json y el zip del addon en la versión que diga el test.
func addonCDN(t *testing.T, version *string) *httptest.Server {
	t.Helper()
	zipOf := func(v string) []byte {
		var b bytes.Buffer
		w := zip.NewWriter(&b)
		f, _ := w.Create("RebirthTrack/RebirthTrack.toc")
		fmt.Fprintf(f, "## Version: %s\n", v)
		w.Close()
		return b.Bytes()
	}
	var srv *httptest.Server
	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		z := zipOf(*version)
		switch r.URL.Path {
		case "/latest.json":
			sum := sha256.Sum256(z)
			fmt.Fprintf(w, `{"version":%q,"url":%q,"size":%d,"sha256":%q}`, *version, srv.URL+"/addon.zip", len(z), hex.EncodeToString(sum[:]))
		case "/addon.zip":
			w.Write(z)
		default:
			http.NotFound(w, r)
		}
	}))
	return srv
}

func newAddonApp(t *testing.T, cdn string, auto bool) (*App, string) {
	t.Helper()
	root := filepath.Join(t.TempDir(), "World of Warcraft")
	os.MkdirAll(filepath.Join(root, "_anniversary_", "Interface", "AddOns"), 0o755)
	st, err := state.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	st.Update(func(s *state.State) { s.ConsentAt = &now; s.WowRoot = root; s.AddonAuto = auto })
	a := New("0.6.0", "http://127.0.0.1:1", st, nil)
	a.cfg = &api.Config{AddonLatestURL: cdn + "/latest.json"}
	return a, filepath.Join(root, "_anniversary_", "Interface", "AddOns")
}

// waitAddon espera a que termine la búsqueda o instalación en segundo plano.
func waitAddon(t *testing.T, a *App) AddonStatus {
	t.Helper()
	for i := 0; i < 200; i++ {
		if st := a.Status().Addon; st.Busy == "" {
			return st
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("la operación del addon no terminó")
	return AddonStatus{}
}

func TestAddonCheckThenInstall(t *testing.T) {
	v := "0.4.1"
	cdn := addonCDN(t, &v)
	defer cdn.Close()
	a, addons := newAddonApp(t, cdn.URL, false)

	// Buscar solo consulta: avisa que hay una y no instala nada.
	if !a.StartAddonCheck() {
		t.Fatal("no arrancó la búsqueda")
	}
	if a.StartAddonCheck() || a.StartAddonInstall() {
		t.Fatal("con una búsqueda en curso no arranca otra")
	}
	if st := waitAddon(t, a); st.Result != "available" || !st.Pending || st.At == nil {
		t.Fatalf("%+v", st)
	}
	if _, err := os.Stat(filepath.Join(addons, "RebirthTrack")); !os.IsNotExist(err) {
		t.Fatal("buscar no tiene que instalar")
	}

	// Instalar deja la versión encontrada.
	a.StartAddonInstall()
	if st := waitAddon(t, a); st.Result != "installed" || st.Pending {
		t.Fatalf("%+v", st)
	}
	if got := a.Status().Installs[0].AddonVersion; got != "0.4.1" {
		t.Fatalf("instalado %q", got)
	}

	// Al día.
	a.StartAddonCheck()
	if st := waitAddon(t, a); st.Result != "uptodate" {
		t.Fatalf("%+v", st)
	}

	// Sale otra: con «instalar solo» apagado la revisión automática solo avisa.
	v = "0.4.2"
	a.CheckAddon()
	if st := a.Status().Addon; st.Result != "available" || !st.Pending {
		t.Fatalf("%+v", st)
	}
}

func TestAddonAutoInstall(t *testing.T) {
	v := "0.4.1"
	cdn := addonCDN(t, &v)
	defer cdn.Close()
	a, _ := newAddonApp(t, cdn.URL, false)

	// Activar «instalar y actualizar solo» busca e instala enseguida.
	a.SetAddonAuto(true)
	if !a.Store.Read().AddonAuto {
		t.Fatal("no se guardó")
	}
	for i := 0; i < 200 && a.Status().Installs[0].AddonVersion != "0.4.1"; i++ {
		time.Sleep(10 * time.Millisecond)
	}
	if st := waitAddon(t, a); st.Result != "installed" {
		t.Fatalf("%+v", st)
	}

	v = "0.4.2"
	a.CheckAddon()
	if got := a.Status().Installs[0].AddonVersion; got != "0.4.2" {
		t.Fatalf("la revisión automática tenía que actualizar: %q", got)
	}
}

func TestAddonCheckError(t *testing.T) {
	cdn := httptest.NewServer(http.NotFoundHandler())
	defer cdn.Close()
	a, _ := newAddonApp(t, cdn.URL, false)
	a.StartAddonCheck()
	if st := waitAddon(t, a); st.Result != "error" || st.Error == "" {
		t.Fatalf("%+v", st)
	}
}

func TestPlansErrorLoggedOnce(t *testing.T) {
	v := "0.4.1"
	cdn := addonCDN(t, &v)
	defer cdn.Close()
	a, _ := newAddonApp(t, cdn.URL, false)
	_, installs := a.Installs()
	in := installs[0]
	denied := &os.PathError{Op: "rename", Path: "Plans.lua", Err: os.ErrPermission}
	count := func() int {
		n := 0
		for _, x := range a.Status().Activity {
			if strings.Contains(x.Text, "Plans.lua") {
				n++
			}
		}
		return n
	}
	a.plansError(in, denied)
	a.plansError(in, denied)
	if n := count(); n != 1 {
		t.Fatalf("el mismo error se anota una vez: %d", n)
	}
	if txt := a.Status().Activity[0].Text; !strings.Contains(txt, "Se reintenta solo") || strings.Contains(txt, "rename") {
		t.Fatalf("mensaje: %s", txt)
	}
	a.plansError(in, nil) // se pudo escribir
	a.plansError(in, denied)
	if n := count(); n != 2 {
		t.Fatalf("si vuelve a fallar después de andar, se avisa de nuevo: %d", n)
	}
}
