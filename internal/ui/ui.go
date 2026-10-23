// Package ui sirve el panel de la app en http://127.0.0.1:47812, solo para esta PC.
// Cada pedido lleva una clave aleatoria (guardada en la carpeta de la app) para que ninguna
// página web pueda manejar la app a escondidas, y se rechaza cualquier Host que no sea local.
package ui

import (
	"crypto/rand"
	"embed"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/mgaliano-lab/rebirth-track-uploader/internal/app"
)

// Port del panel. Se cambia solo para pruebas (--panel-port), p. ej. con otra instancia corriendo.
var Port = 47812

//go:embed index.html icon.png
var files embed.FS

type Server struct {
	App *app.App
	Key string
	ln  net.Listener
	// OnShow muestra el panel de esta instancia (lo pide una segunda instancia, ej. al abrir
	// el acceso directo con la app ya corriendo).
	OnShow func()
}

// URL del panel con la clave.
func (s *Server) URL() string { return fmt.Sprintf("http://127.0.0.1:%d/?k=%s", Port, s.Key) }

// KeyFile guarda la clave para que una segunda instancia abra el panel de la primera.
func KeyFile(dir string) string { return filepath.Join(dir, "panel.key") }

// Listen toma el puerto; si ya hay otra instancia de la app, devuelve ErrRunning.
var ErrRunning = errors.New("la app ya está abierta")

func Listen(a *app.App, stateDir string) (*Server, error) {
	ln, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", Port))
	if err != nil {
		c := http.Client{Timeout: 2 * time.Second}
		if res, err2 := c.Get(fmt.Sprintf("http://127.0.0.1:%d/ping", Port)); err2 == nil {
			res.Body.Close()
			if res.Header.Get("X-Rebirth-Track") == "1" {
				return nil, ErrRunning
			}
		}
		return nil, fmt.Errorf("el puerto %d está ocupado por otro programa: %w", Port, err)
	}
	// La clave se conserva entre reinicios (ej. al actualizarse sola): así el panel que estaba
	// abierto se reconecta solo cuando vuelve la app. Es un archivo 0600 en la carpeta del usuario.
	key := ""
	if b, err := os.ReadFile(KeyFile(stateDir)); err == nil {
		if k := strings.TrimSpace(string(b)); len(k) == 32 {
			if _, err := hex.DecodeString(k); err == nil {
				key = k
			}
		}
	}
	if key == "" {
		b := make([]byte, 16)
		rand.Read(b)
		key = hex.EncodeToString(b)
		if err := os.WriteFile(KeyFile(stateDir), []byte(key), 0o600); err != nil {
			return nil, err
		}
	}
	return &Server{App: a, Key: key, ln: ln}, nil
}

// ShowRunning le pide a la instancia que ya está corriendo que muestre su panel.
func ShowRunning(stateDir string) bool {
	k, _ := os.ReadFile(KeyFile(stateDir))
	req, _ := http.NewRequest("POST", fmt.Sprintf("http://127.0.0.1:%d/show", Port), nil)
	req.Header.Set("X-Key", strings.TrimSpace(string(k)))
	res, err := (&http.Client{Timeout: 3 * time.Second}).Do(req)
	if err != nil {
		return false
	}
	res.Body.Close()
	return res.StatusCode == http.StatusOK
}

// RunningURL arma la URL del panel de la instancia que ya está corriendo.
func RunningURL(stateDir string) string {
	k, _ := os.ReadFile(KeyFile(stateDir))
	return fmt.Sprintf("http://127.0.0.1:%d/?k=%s", Port, strings.TrimSpace(string(k)))
}

func (s *Server) Serve() error {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /ping", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Rebirth-Track", "1")
		w.Write([]byte("ok"))
	})
	mux.HandleFunc("POST /show", func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Key") != s.Key || s.OnShow == nil {
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}
		go s.OnShow()
		w.Write([]byte("ok"))
	})
	mux.HandleFunc("GET /icon.png", func(w http.ResponseWriter, r *http.Request) {
		b, _ := files.ReadFile("icon.png")
		w.Header().Set("Content-Type", "image/png")
		w.Write(b)
	})
	mux.HandleFunc("GET /", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" || r.URL.Query().Get("k") != s.Key {
			http.Error(w, "Abrí el panel desde el ícono de Rebirth Track en la bandeja del sistema.", http.StatusForbidden)
			return
		}
		b, _ := files.ReadFile("index.html")
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("Content-Security-Policy", "default-src 'self'; style-src 'self' 'unsafe-inline'; script-src 'self' 'unsafe-inline'; img-src 'self'")
		w.Write(b)
	})
	api := func(path string, fn func(r *http.Request) (any, error)) {
		mux.HandleFunc(path, func(w http.ResponseWriter, r *http.Request) {
			if r.Header.Get("X-Key") != s.Key {
				http.Error(w, "forbidden", http.StatusForbidden)
				return
			}
			out, err := fn(r)
			w.Header().Set("Content-Type", "application/json")
			if err != nil {
				w.WriteHeader(http.StatusBadRequest)
				json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
				return
			}
			json.NewEncoder(w).Encode(out)
		})
	}
	body := func(r *http.Request) map[string]any {
		m := map[string]any{}
		json.NewDecoder(http.MaxBytesReader(nil, r.Body, 1<<16)).Decode(&m)
		return m
	}
	a := s.App
	api("GET /api/status", func(r *http.Request) (any, error) { return a.Status(), nil })
	api("POST /api/consent", func(r *http.Request) (any, error) { a.Consent(); go a.CheckAddon(false); return a.Status(), nil })
	api("POST /api/link", func(r *http.Request) (any, error) { return a.StartLink() })
	api("POST /api/unlink", func(r *http.Request) (any, error) { a.Unlink(); return a.Status(), nil })
	api("POST /api/pause", func(r *http.Request) (any, error) {
		p, _ := body(r)["paused"].(bool)
		a.SetPaused(p)
		return a.Status(), nil
	})
	api("POST /api/upload-now", func(r *http.Request) (any, error) { a.UploadNow(); return a.Status(), nil })
	api("GET /api/old-logs", func(r *http.Request) (any, error) { return a.OldLogs() })
	api("POST /api/old-logs/import", func(r *http.Request) (any, error) {
		var keys []string
		if list, ok := body(r)["keys"].([]any); ok {
			for _, k := range list {
				if s, ok := k.(string); ok {
					keys = append(keys, s)
				}
			}
		}
		n, err := a.ImportLogs(keys)
		if err != nil {
			return nil, err
		}
		return map[string]int{"queued": n}, nil
	})
	api("POST /api/app/update", func(r *http.Request) (any, error) { go a.CheckApp(true); return a.Status(), nil })
	api("POST /api/app/store", func(r *http.Request) (any, error) { a.OpenStore(); return a.Status(), nil })
	api("POST /api/wow/browse", func(r *http.Request) (any, error) {
		if _, err := a.BrowseWowRoot(); err != nil {
			return nil, err
		}
		return a.Status(), nil
	})
	api("POST /api/wow/locate", func(r *http.Request) (any, error) {
		if _, err := a.LocateWow(); err != nil {
			return nil, err
		}
		return a.Status(), nil
	})
	api("POST /api/scans/check", func(r *http.Request) (any, error) { a.CheckScansNow(); return a.Status(), nil })
	api("POST /api/addon/install", func(r *http.Request) (any, error) { a.CheckAddon(true); return a.Status(), nil })
	api("POST /api/settings", func(r *http.Request) (any, error) {
		b := body(r)
		if v, ok := b["flavor"].(string); ok {
			en, _ := b["enabled"].(bool)
			a.SetFlavor(v, en)
		}
		if v, ok := b["wowRoot"].(string); ok {
			if err := a.SetWowRoot(strings.TrimSpace(v)); err != nil {
				return nil, err
			}
		}
		if v, ok := b["addonAutoUpdate"].(bool); ok {
			a.SetAddonAuto(v)
		}
		if v, ok := b["sendBalance"].(bool); ok {
			a.SetSendBalance(v)
		}
		if v, ok := b["uploadEverySeconds"].(float64); ok {
			if err := a.SetEvery(int(v)); err != nil {
				return nil, err
			}
		}
		if v, ok := b["appAutoUpdate"].(bool); ok {
			a.SetAppAuto(v)
		}
		if v, ok := b["autostart"].(bool); ok {
			if err := app.SetAutostart(v); err != nil {
				return nil, err
			}
		}
		return a.Status(), nil
	})
	// Solo Host local (evita DNS rebinding).
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		host := r.Host
		if host != fmt.Sprintf("127.0.0.1:%d", Port) && host != fmt.Sprintf("localhost:%d", Port) {
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}
		mux.ServeHTTP(w, r)
	})
	return (&http.Server{Handler: h, ReadHeaderTimeout: 10 * time.Second}).Serve(s.ln)
}
