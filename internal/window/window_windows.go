//go:build windows

// Package window muestra el panel en una ventana propia, con WebView2 (el motor de Edge que trae
// Windows), en vez de abrirlo en el navegador: sin barra de direcciones, con el título y el ícono
// de la app. Cerrarla solo la oculta de la vista: la app sigue en la bandeja subiendo registros.
// Si la PC no tiene WebView2, el panel se abre en el navegador como antes.
//
// La ventana no muestra la barra de título de Windows: el panel dibuja la suya (con minimizar,
// maximizar y cerrar al estilo de la app) y le pide a la ventana lo que corresponde con rtWin.
// Por debajo sigue siendo una ventana normal (WS_OVERLAPPEDWINDOW): se acopla a los costados, anima
// al minimizar y se redimensiona desde los bordes; solo se le saca el alto del título (frameless).
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
	pSetWindowLongW  = user32.NewProc("SetWindowLongPtrW")
	pCallWindowProcW = user32.NewProc("CallWindowProcW")
	pIsZoomed        = user32.NewProc("IsZoomed")
	pPostMessageW    = user32.NewProc("PostMessageW")
	pReleaseCapture  = user32.NewProc("ReleaseCapture")
	pGetSystemMetric = user32.NewProc("GetSystemMetrics")
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
	frameless(hwnd)
	// Los botones y la barra de título del panel. Se ejecutan con Dispatch, fuera del aviso de
	// WebView2 que trae el pedido (maximizar o arrastrar reentra en el bucle de mensajes).
	if err := wv.Bind("rtWin", func(action string) { wv.Dispatch(func() { winAction(hwnd, action) }) }); err != nil {
		log.Println("ventana:", err)
	}
	if err := wv.Bind("rtWinMaximized", func() bool { z, _, _ := pIsZoomed.Call(hwnd); return z != 0 }); err != nil {
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

// ── Sin la barra de título de Windows ────────────────────────────────────────────────────

const (
	wmClose         = 0x0010
	wmNCCalcSize    = 0x0083
	wmNCLButtonDown = 0x00A1
	htCaption       = 2
	htTop           = 12
	htTopLeft       = 13
	htTopRight      = 14
)

var (
	origProc  uintptr // wndproc de go-webview2 (una sola ventana a la vez)
	framedCB  = windows.NewCallback(framedProc)
	edgeHitTo = map[string]uintptr{"resize-top": htTop, "resize-topleft": htTopLeft, "resize-topright": htTopRight}
)

// frameless reemplaza el wndproc de la ventana para quitar el área del título y vuelve a calcular
// el marco. Los bordes de los costados y de abajo siguen siendo de Windows (invisibles, para
// redimensionar); el de arriba lo pone el panel (rtWin("resize-top")).
func frameless(hwnd uintptr) {
	gwlpWndProc := -4
	origProc, _, _ = pSetWindowLongW.Call(hwnd, uintptr(gwlpWndProc), framedCB)
	pSetWindowPos.Call(hwnd, 0, 0, 0, 0, 0, 0x20|0x1|0x2|0x4|0x10) // SWP_FRAMECHANGED|NOSIZE|NOMOVE|NOZORDER|NOACTIVATE
}

func framedProc(hwnd, msg, wp, lp uintptr) uintptr {
	if msg == wmNCCalcSize && wp != 0 {
		// El primer rect de NCCALCSIZE_PARAMS es la ventana: Windows le resta el marco y el título;
		// se le devuelve el borde de arriba para que el contenido llegue hasta el tope.
		r := *(**rect)(unsafe.Pointer(&lp)) // lp apunta a memoria de Windows, no de Go
		top := r.Top
		pCallWindowProcW.Call(origProc, hwnd, msg, wp, lp)
		r.Top = top
		if z, _, _ := pIsZoomed.Call(hwnd); z != 0 {
			// Maximizada, Windows la agranda un marco más allá de la pantalla: que no se corte arriba.
			frame, _, _ := pGetSystemMetric.Call(33) // SM_CYSIZEFRAME
			pad, _, _ := pGetSystemMetric.Call(92)   // SM_CXPADDEDBORDER
			r.Top += int32(frame + pad)
		}
		return 0
	}
	ret, _, _ := pCallWindowProcW.Call(origProc, hwnd, msg, wp, lp)
	return ret
}

// winAction: lo que pide la barra de título del panel.
func winAction(hwnd uintptr, action string) {
	switch action {
	case "min":
		pShowWindow.Call(hwnd, 6) // SW_MINIMIZE
	case "max":
		if z, _, _ := pIsZoomed.Call(hwnd); z != 0 {
			pShowWindow.Call(hwnd, 9) // SW_RESTORE
		} else {
			pShowWindow.Call(hwnd, 3) // SW_MAXIMIZE
		}
	case "close":
		pPostMessageW.Call(hwnd, wmClose, 0, 0) // como la × de Windows: se cierra la ventana, la app sigue en la bandeja
	case "drag":
		// Como si se apretara la barra de título nativa: mover, acoplar a los costados y, maximizada,
		// volver al tamaño normal al arrastrar.
		pReleaseCapture.Call()
		pPostMessageW.Call(hwnd, wmNCLButtonDown, htCaption, 0)
	default:
		if ht, ok := edgeHitTo[action]; ok {
			pReleaseCapture.Call()
			pPostMessageW.Call(hwnd, wmNCLButtonDown, ht, 0)
		}
	}
}
