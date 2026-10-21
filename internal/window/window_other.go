//go:build !windows

package window

// Fuera de Windows (desarrollo en macOS/Linux) el panel se sigue abriendo en el navegador.
type Window struct{ external func(string) }

func New(dataDir string, icon []byte, external func(string)) *Window {
	return &Window{external: external}
}

func (w *Window) Show(url string) { w.external(url) }
func (w *Window) IsOpen() bool    { return false }
