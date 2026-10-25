//go:build windows

// Package window muestra el panel en una ventana propia, con WebView2 (el motor de Edge que trae
// Windows), en vez de abrirlo en el navegador: sin barra de direcciones, con el título y el ícono
// de la app. Cerrarla solo la oculta de la vista: la app sigue en la bandeja subiendo registros.
// Si la PC no tiene WebView2, el panel se abre en el navegador como antes.
package window

import (
	"encoding/json"
	"log"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"time"
	"unsafe"

	webview2 "github.com/jchv/go-webview2"
	"golang.org/x/sys/windows"
)

const title = "Rebirth Track Uploader"

// Los links a otros sitios (Discord, la web, GitHub) se abren en el navegador normal: así el
// login de Discord usa la sesión de siempre y la ventana nunca sale del panel.
const linksJS = `(() => {
  const outside = (u) => { try { return new URL(u, location.href).origin !== location.origin } catch { return false } }
  const ext = (u) => window.rtOpenExternal(new URL(u, location.href).href)
  document.addEventListener('click', (e) => {
    const a = e.target.closest && e.target.closest('a[href]')
    if (!a || !(outside(a.href) || a.target === '_blank')) return
    e.preventDefault()
    if (outside(a.href)) ext(a.href); else location.href = a.href
  }, true)
  const open = window.open
  window.open = (u, ...rest) => {
    if (u && outside(u)) { ext(u); return null }
    if (u) { location.href = u; return null }
    return open.call(window, u, ...rest)
  }
})()`

var (
	user32           = windows.NewLazySystemDLL("user32.dll")
	pShowWindow      = user32.NewProc("ShowWindow")
	pSetForeground   = user32.NewProc("SetForegroundWindow")
	pLoadImageW      = user32.NewProc("LoadImageW")
	pSendMessageW    = user32.NewProc("SendMessageW")
	pGetWindowRect   = user32.NewProc("GetWindowRect")
	pSetWindowPos    = user32.NewProc("SetWindowPos")
	pIsIconic        = user32.NewProc("IsIconic")
	pMonitorFromRect = user32.NewProc("MonitorFromRect")
	pMonitorFromWin  = user32.NewProc("MonitorFromWindow")
	pGetMonitorInfoW = user32.NewProc("GetMonitorInfoW")
)

type rect struct{ Left, Top, Right, Bottom int32 }

type monitorInfo struct {
	Size          uint32
	Monitor, Work rect
	Flags         uint32
}

// Tamaño de la primera apertura: el ancho de siempre y el alto de la vista más larga (la de la
// primera vez: qué hace, qué se releva y la actividad), achicados a lo que entre en la pantalla.
// La app no declara DPI, así que son unidades lógicas (Windows escala la ventana entera).
const (
	firstWidth, firstHeight = 1120, 1280
	minWidth, minHeight     = 900, 620
	screenMargin            = 24
)

type Window struct {
	dataDir  string
	icon     []byte
	external func(string)

	mu      sync.Mutex
	hwnd    uintptr
	opening bool
	broken  bool // sin WebView2: todo va al navegador
}

func New(dataDir string, icon []byte, external func(string)) *Window {
	return &Window{dataDir: dataDir, icon: icon, external: external}
}

func (w *Window) IsOpen() bool {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.hwnd != 0 || w.opening
}

// Show abre la ventana del panel o, si ya está abierta, la trae al frente.
func (w *Window) Show(url string) {
	w.mu.Lock()
	switch {
	case w.broken:
		w.mu.Unlock()
		w.external(url)
		return
	case w.hwnd != 0:
		h := w.hwnd
		w.mu.Unlock()
		pShowWindow.Call(h, 9) // SW_RESTORE
		pSetForeground.Call(h)
		return
	case w.opening:
		w.mu.Unlock()
		return
	}
	w.opening = true
	w.mu.Unlock()
	go w.run(url)
}

// La ventana vive en su propio hilo del sistema, con su propio bucle de mensajes (el de la
// bandeja corre en otro); cuando el usuario la cierra, el bucle termina y el hilo se libera.
func (w *Window) run(url string) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	wv := webview2.NewWithOptions(webview2.WebViewOptions{
		DataPath:      filepath.Join(w.dataDir, "webview2"),
		AutoFocus:     true,
		WindowOptions: webview2.WindowOptions{Title: title, Width: 1120, Height: 780}, // se acomoda en placeWindow
	})
	if wv == nil {
		log.Println("ventana: WebView2 no disponible; el panel se abre en el navegador")
		w.mu.Lock()
		w.broken, w.opening = true, false
		w.mu.Unlock()
		w.external(url)
		return
	}
	hwnd := uintptr(wv.Window())
	w.setIcon(hwnd)
	w.placeWindow(wv, hwnd)
	if err := wv.Bind("rtOpenExternal", func(u string) { w.external(u) }); err != nil {
		log.Println("ventana:", err)
	}
	wv.Init(linksJS)
	wv.Navigate(url)
	pSetForeground.Call(hwnd)

	w.mu.Lock()
	w.hwnd, w.opening = hwnd, false
	w.mu.Unlock()
	stop := make(chan struct{})
	go w.trackBounds(hwnd, stop)
	wv.Run()
	close(stop)
	w.mu.Lock()
	w.hwnd = 0
	w.mu.Unlock()
}

// Ícono de la app en la barra de título y en la de tareas (desde el .ico embebido).
func (w *Window) setIcon(hwnd uintptr) {
	path := filepath.Join(w.dataDir, "icon.ico")
	if err := os.WriteFile(path, w.icon, 0o600); err != nil {
		return
	}
	p, _ := windows.UTF16PtrFromString(path)
	for _, size := range []struct{ kind, px uintptr }{{1, 32}, {0, 16}} { // ICON_BIG, ICON_SMALL
		h, _, _ := pLoadImageW.Call(0, uintptr(unsafe.Pointer(p)), 1, size.px, size.px, 0x10) // IMAGE_ICON, LR_LOADFROMFILE
		if h != 0 {
			pSendMessageW.Call(hwnd, 0x80, size.kind, h) // WM_SETICON
		}
	}
}

// Tamaño y posición: se recuerdan entre aperturas (si todavía caen en algún monitor).
func (w *Window) boundsFile() string { return filepath.Join(w.dataDir, "window.json") }

// placeWindow: la posición guardada si todavía cae en algún monitor (achicada a su área útil si no
// entra); si no hay, la primera vez: centrada en el área útil (sin la barra de tareas) con el
// tamaño que pide la vista, o el que entre en pantallas chicas o con escala alta (ej. 1080p al
// 150 %: unos 1280×680 útiles).
func (w *Window) placeWindow(wv webview2.WebView, hwnd uintptr) {
	r, saved := w.savedBounds()
	var work rect
	if saved {
		m, _, _ := pMonitorFromRect.Call(uintptr(unsafe.Pointer(&r)), 0) // MONITOR_DEFAULTTONULL
		saved = m != 0 && monitorWork(m, &work)
	}
	if !saved {
		m, _, _ := pMonitorFromWin.Call(hwnd, 2) // MONITOR_DEFAULTTONEAREST
		if !monitorWork(m, &work) {
			wv.SetSize(minWidth, minHeight, webview2.HintMin)
			return
		}
		width := min(firstWidth, work.Right-work.Left-2*screenMargin)
		height := min(firstHeight, work.Bottom-work.Top-2*screenMargin)
		r.Left = work.Left + (work.Right-work.Left-width)/2
		r.Top = work.Top + (work.Bottom-work.Top-height)/2
		r.Right, r.Bottom = r.Left+width, r.Top+height
	}
	// Ni la guardada ni el mínimo pueden pasarse del área útil.
	r.Right = min(r.Right, r.Left+work.Right-work.Left)
	r.Bottom = min(r.Bottom, r.Top+work.Bottom-work.Top)
	if r.Right > work.Right {
		r.Left, r.Right = work.Right-(r.Right-r.Left), work.Right
	}
	if r.Bottom > work.Bottom {
		r.Top, r.Bottom = work.Bottom-(r.Bottom-r.Top), work.Bottom
	}
	r.Left, r.Top = max(r.Left, work.Left), max(r.Top, work.Top)
	wv.SetSize(int(min(minWidth, work.Right-work.Left)), int(min(minHeight, work.Bottom-work.Top)), webview2.HintMin)
	pSetWindowPos.Call(hwnd, 0, uintptr(r.Left), uintptr(r.Top), uintptr(r.Right-r.Left), uintptr(r.Bottom-r.Top), 0x4|0x10) // SWP_NOZORDER|SWP_NOACTIVATE
}

func (w *Window) savedBounds() (rect, bool) {
	var r rect
	b, err := os.ReadFile(w.boundsFile())
	if err != nil || json.Unmarshal(b, &r) != nil || r.Right-r.Left < 600 || r.Bottom-r.Top < 400 {
		return rect{}, false
	}
	return r, true
}

// monitorWork: el área útil del monitor (sin la barra de tareas).
func monitorWork(m uintptr, work *rect) bool {
	if m == 0 {
		return false
	}
	mi := monitorInfo{Size: uint32(unsafe.Sizeof(monitorInfo{}))}
	if ok, _, _ := pGetMonitorInfoW.Call(m, uintptr(unsafe.Pointer(&mi))); ok == 0 {
		return false
	}
	*work = mi.Work
	return true
}

func (w *Window) trackBounds(hwnd uintptr, stop chan struct{}) {
	var last rect
	t := time.NewTicker(time.Second)
	defer t.Stop()
	for {
		select {
		case <-stop:
			return
		case <-t.C:
		}
		if min, _, _ := pIsIconic.Call(hwnd); min != 0 {
			continue
		}
		var r rect
		if ok, _, _ := pGetWindowRect.Call(hwnd, uintptr(unsafe.Pointer(&r))); ok == 0 || r == last {
			continue
		}
		last = r
		if b, err := json.Marshal(r); err == nil {
			_ = os.WriteFile(w.boundsFile(), b, 0o600)
		}
	}
}
