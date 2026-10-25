// Rebirth Track Uploader: instala el addon Rebirth Track y sube a rebirthknights.fun los tramos
// de bandas de los registros de combate de WoW. Corre en la bandeja del sistema; el panel
// (http://127.0.0.1:47812, solo local) se abre en una ventana propia (WebView2) o, si la PC no
// lo tiene, en el navegador.
package main

import (
	"context"
	_ "embed"
	"errors"
	"flag"
	"fmt"
	"log"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"runtime"
	"syscall"
	"time"

	"fyne.io/systray"

	"github.com/mgaliano-lab/rebirth-track-uploader/internal/app"
	"github.com/mgaliano-lab/rebirth-track-uploader/internal/state"
	"github.com/mgaliano-lab/rebirth-track-uploader/internal/ui"
	"github.com/mgaliano-lab/rebirth-track-uploader/internal/window"
)

var version = "dev"

//go:embed icon.ico
var iconICO []byte

//go:embed icon.png
var iconPNG []byte

func main() {
	minimized := flag.Bool("minimized", false, "arrancar sin abrir el panel (inicio con Windows)")
	headless := flag.Bool("headless", false, "sin ícono en la bandeja (pruebas)")
	site := flag.String("site", envOr("REBIRTH_TRACK_SITE", "https://rebirthknights.fun"), "URL de Rebirth Track")
	dir := flag.String("state-dir", "", "carpeta de datos (por defecto, la de configuración del usuario)")
	scansKey := flag.String("scans-pubkey", os.Getenv("REBIRTH_TRACK_SCANS_PUBKEY"), "clave pública extra (Ed25519, base64) para el manifiesto de relevamientos: pruebas contra un sitio local")
	panelPort := flag.Int("panel-port", ui.Port, "puerto del panel local (solo pruebas)")
	afterUpdate := flag.Bool("after-update", false, "la lanzó la versión anterior al actualizarse: esperar a que libere el panel")
	flag.Parse()
	ui.Port = *panelPort
	app.CleanupUpdate()

	stateDir := *dir
	if stateDir == "" {
		d, err := state.Dir()
		if err != nil {
			fatal(err)
		}
		stateDir = d
	}
	st, err := state.Open(stateDir)
	if err != nil {
		fatal(err)
	}
	if f, err := os.OpenFile(stateDir+string(os.PathSeparator)+"app.log", os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600); err == nil {
		log.SetOutput(f)
	}

	a := app.New(version, *site, st, openURL)
	if *scansKey != "" {
		a.ExtraScanKeys = []string{*scansKey}
	}
	srv, err := ui.Listen(a, stateDir)
	for i := 0; *afterUpdate && errors.Is(err, ui.ErrRunning) && i < 40; i++ {
		time.Sleep(500 * time.Millisecond) // la versión vieja todavía está cerrando
		srv, err = ui.Listen(a, stateDir)
	}
	if errors.Is(err, ui.ErrRunning) {
		// Ya estaba abierta: que muestre su ventana (o, si es una versión vieja, su panel en el navegador).
		if !ui.ShowRunning(stateDir) {
			openURL(ui.RunningURL(stateDir))
		}
		return
	}
	if err != nil {
		fatal(err)
	}
	win := window.New(stateDir, iconICO, openURL)
	showPanel := func() { win.Show(srv.URL()) }
	if *headless {
		showPanel = func() { openURL(srv.URL()) }
	}
	srv.OnShow = showPanel
	a.OnUnlinked = showPanel // que se vea en el acto que la desvincularon desde la web
	// Si el panel estaba abierto al actualizarse, la versión nueva lo vuelve a abrir.
	reopen := filepath.Join(stateDir, "reopen-panel")
	a.BeforeUpdate = func() {
		if win.IsOpen() {
			_ = os.WriteFile(reopen, nil, 0o600)
		}
	}
	reopenAfterUpdate := os.Remove(reopen) == nil
	go func() {
		if err := srv.Serve(); err != nil {
			log.Println("panel:", err)
		}
	}()

	sig := make(chan os.Signal, 1)
	if *headless {
		a.Quit = func() { sig <- os.Interrupt }
	} else {
		a.Quit = systray.Quit
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { a.Run(ctx); close(done) }()
	if !(*minimized || app.StartedAtLogin()) || st.Read().ConsentAt == nil || reopenAfterUpdate {
		showPanel()
	}
	quit := func() { cancel(); <-done }

	if *headless {
		signal.Notify(sig, os.Interrupt, syscall.SIGTERM)
		fmt.Println("Panel:", srv.URL())
		<-sig
		quit()
		return
	}
	systray.Run(func() {
		if runtime.GOOS == "windows" {
			systray.SetIcon(iconICO)
		} else {
			systray.SetIcon(iconPNG)
		}
		systray.SetTitle("")
		systray.SetTooltip("Rebirth Track Uploader")
		mOpen := systray.AddMenuItem("Abrir panel", "Ver qué hace y qué sube la app")
		mNow := systray.AddMenuItem("Subir ahora", "Subir ya lo pendiente sin esperar al próximo ciclo")
		mPause := systray.AddMenuItem("Pausar subida", "Dejar de subir hasta que la reanudes")
		mSite := systray.AddMenuItem("Abrir rebirthknights.fun", "Ver las raids y los registros en la web")
		systray.AddSeparator()
		mQuit := systray.AddMenuItem("Salir", "Cerrar la app (sube lo pendiente antes de cerrar)")
		syncPause := func() {
			if st.Read().Paused {
				mPause.SetTitle("Reanudar subida")
			} else {
				mPause.SetTitle("Pausar subida")
			}
		}
		syncPause()
		systray.SetOnTapped(showPanel)
		go func() {
			for {
				select {
				case <-mOpen.ClickedCh:
					showPanel()
				case <-mNow.ClickedCh:
					a.UploadNow()
				case <-mSite.ClickedCh:
					openURL(a.BaseURL)
				case <-mPause.ClickedCh:
					a.SetPaused(!st.Read().Paused)
					syncPause()
				case <-mQuit.ClickedCh:
					systray.Quit()
					return
				}
			}
		}()
	}, quit)
}

func envOr(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}

func openURL(u string) {
	if os.Getenv("REBIRTH_TRACK_NO_BROWSER") == "1" { // pruebas automáticas
		log.Println("abrir:", u)
		return
	}
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", u)
	case "darwin":
		cmd = exec.Command("open", u)
	default:
		cmd = exec.Command("xdg-open", u)
	}
	_ = cmd.Start()
}

func fatal(err error) {
	log.Println(err)
	fmt.Fprintln(os.Stderr, "Rebirth Track Uploader:", err)
	os.Exit(1)
}
