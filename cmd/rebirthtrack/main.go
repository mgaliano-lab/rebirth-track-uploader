// Rebirth Track Uploader: instala el addon Rebirth Track y sube a rebirthknights.fun los tramos
// de bandas de los registros de combate de WoW. Corre en la bandeja del sistema; el panel se
// abre en el navegador (http://127.0.0.1:47812, solo local).
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
	"runtime"
	"syscall"

	"fyne.io/systray"

	"github.com/mgaliano-lab/rebirth-track-uploader/internal/app"
	"github.com/mgaliano-lab/rebirth-track-uploader/internal/state"
	"github.com/mgaliano-lab/rebirth-track-uploader/internal/ui"
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
	flag.Parse()

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
	srv, err := ui.Listen(a, stateDir)
	if errors.Is(err, ui.ErrRunning) {
		openURL(ui.RunningURL(stateDir)) // ya estaba abierta: mostrar su panel
		return
	}
	if err != nil {
		fatal(err)
	}
	go func() {
		if err := srv.Serve(); err != nil {
			log.Println("panel:", err)
		}
	}()

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { a.Run(ctx); close(done) }()
	if !(*minimized || app.StartedAtLogin()) || st.Read().ConsentAt == nil {
		openURL(srv.URL())
	}
	quit := func() { cancel(); <-done }

	if *headless {
		sig := make(chan os.Signal, 1)
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
		mPause := systray.AddMenuItem("Pausar subida", "Dejar de subir hasta que la reanudes")
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
		systray.SetOnTapped(func() { openURL(srv.URL()) })
		go func() {
			for {
				select {
				case <-mOpen.ClickedCh:
					openURL(srv.URL())
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
