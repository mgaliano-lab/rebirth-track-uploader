// Package api habla con Rebirth Track: vinculación de la PC, configuración y subida de lotes.
package api

import (
	"bufio"
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

var ErrUnauthorized = errors.New("la PC no está vinculada (o la desvincularon desde la web)")
var ErrPending = errors.New("pendiente")

// ErrNoEvents: el sitio todavía no tiene el stream de avisos (versión vieja).
var ErrNoEvents = errors.New("el sitio no tiene el stream de avisos")

// ErrSessionGone: la sesión ya no existe en el sitio (la borraron desde la web).
var ErrSessionGone = errors.New("la sesión ya no existe en el sitio")

type Client struct {
	BaseURL    string
	Token      string
	AppVersion string
	HTTP       *http.Client
}

func New(base, token, version string) *Client {
	return &Client{BaseURL: base, Token: token, AppVersion: version, HTTP: &http.Client{Timeout: 60 * time.Second}}
}

type apiError struct {
	Message string `json:"message"`
}

func (c *Client) do(method, path string, body io.Reader, headers map[string]string, out any) (int, error) {
	req, err := http.NewRequest(method, c.BaseURL+path, body)
	if err != nil {
		return 0, err
	}
	req.Header.Set("User-Agent", "RebirthTrackUploader/"+c.AppVersion)
	req.Header.Set("X-App-Version", c.AppVersion)
	if c.Token != "" {
		req.Header.Set("Authorization", "Bearer "+c.Token)
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	res, err := c.HTTP.Do(req)
	if err != nil {
		return 0, err
	}
	defer res.Body.Close()
	b, _ := io.ReadAll(io.LimitReader(res.Body, 8<<20)) // el contenido de una subida llega a 1 MB de texto (más el JSON)
	if res.StatusCode == http.StatusUnauthorized {
		return res.StatusCode, ErrUnauthorized
	}
	if res.StatusCode >= 400 {
		var e apiError
		_ = json.Unmarshal(b, &e)
		if e.Message == "" {
			e.Message = res.Status
		}
		return res.StatusCode, fmt.Errorf("%s", e.Message)
	}
	if out != nil && len(b) > 0 {
		if err := json.Unmarshal(b, out); err != nil {
			return res.StatusCode, err
		}
	}
	return res.StatusCode, nil
}

func jsonBody(v any) (io.Reader, map[string]string) {
	b, _ := json.Marshal(v)
	return bytes.NewReader(b), map[string]string{"Content-Type": "application/json"}
}

type LinkStart struct {
	DeviceCode string `json:"deviceCode"`
	UserCode   string `json:"userCode"`
	VerifyURL  string `json:"verifyUrl"`
	Interval   int    `json:"interval"`
	ExpiresIn  int    `json:"expiresIn"`
}

func (c *Client) StartLink(deviceName string) (LinkStart, error) {
	var out LinkStart
	b, h := jsonBody(map[string]string{"deviceName": deviceName, "appVersion": c.AppVersion})
	_, err := c.do("POST", "/api/uploader/link/start", b, h, &out)
	return out, err
}

type LinkResult struct {
	Status   string `json:"status"`
	Token    string `json:"token"`
	UserName string `json:"userName"`
}

func (c *Client) PollLink(deviceCode string) (LinkResult, error) {
	var out LinkResult
	b, h := jsonBody(map[string]string{"deviceCode": deviceCode})
	code, err := c.do("POST", "/api/uploader/link/poll", b, h, &out)
	if err == nil && code == http.StatusAccepted {
		return out, ErrPending
	}
	return out, err
}

// Events abre el stream de avisos del sitio (SSE) y llama a onEvent con el nombre de cada uno
// ("revoked": la desvincularon desde la web) hasta que se corte, se cancele ctx o pase `idle` sin
// recibir nada (el sitio manda un ping cada 25 s: si no llega, la conexión quedó colgada).
func (c *Client) Events(ctx context.Context, idle time.Duration, onEvent func(name string)) error {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, "GET", c.BaseURL+"/api/uploader/events", nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", "RebirthTrackUploader/"+c.AppVersion)
	req.Header.Set("X-App-Version", c.AppVersion)
	req.Header.Set("Authorization", "Bearer "+c.Token)
	req.Header.Set("Accept", "text/event-stream")
	res, err := (&http.Client{}).Do(req) // sin Timeout: el stream queda abierto
	if err != nil {
		return err
	}
	defer res.Body.Close()
	switch {
	case res.StatusCode == http.StatusUnauthorized:
		return ErrUnauthorized
	case res.StatusCode == http.StatusNotFound:
		return ErrNoEvents
	case res.StatusCode >= 400:
		return fmt.Errorf("%s", res.Status)
	}
	watchdog := time.AfterFunc(idle, cancel)
	defer watchdog.Stop()
	sc := bufio.NewScanner(res.Body)
	for sc.Scan() {
		watchdog.Reset(idle)
		if name, ok := strings.CutPrefix(sc.Text(), "event:"); ok {
			onEvent(strings.TrimSpace(name))
		}
	}
	if err := sc.Err(); err != nil {
		return err
	}
	return io.EOF
}

// Uploads: lo que subió esta PC según el sitio (registros de combate con sus lotes y archivos del
// juego con cada versión). Se pasa tal cual al panel.
func (c *Client) Uploads() (json.RawMessage, error) {
	var out json.RawMessage
	_, err := c.do("GET", "/api/uploader/uploads", nil, nil, &out)
	return out, err
}

// UploadData: el contenido de un lote (kind "batch") o de una versión de un archivo ("file").
func (c *Client) UploadData(kind string, id int) (json.RawMessage, error) {
	var out json.RawMessage
	_, err := c.do("GET", "/api/uploader/uploads/"+url.PathEscape(kind)+"/"+strconv.Itoa(id), nil, nil, &out)
	return out, err
}

type Config struct {
	UserName            string `json:"userName"`
	DeviceName          string `json:"deviceName"`
	BatchSeconds        int    `json:"batchSeconds"`
	RaidDifficulties    []int  `json:"raidDifficulties"`
	DungeonDifficulties []int  `json:"dungeonDifficulties"`
	InstanceMaps        []int  `json:"instanceMaps"`
	OpenWorldMaps       []int  `json:"openWorldMaps"`
	AddonLatestURL      string `json:"addonLatestUrl"`
	SiteURL             string `json:"siteUrl"`
}

// Plans: Plans.lua con los planes de los personajes vinculados de la cuenta (planificador del sitio).
type Plans struct {
	File      string `json:"file"`
	Count     int    `json:"count"`
	UpdatedAt string `json:"updatedAt"`
	SHA256    string `json:"sha256"`
	Lua       string `json:"lua"`
}

func (c *Client) Plans() (Plans, int, error) {
	var out Plans
	code, err := c.do("GET", "/api/uploader/plans", nil, nil, &out)
	return out, code, err
}

func (c *Client) Config() (Config, error) {
	var out Config
	_, err := c.do("GET", "/api/uploader/config", nil, nil, &out)
	return out, err
}

func (c *Client) OpenSession(flavor, fileName string, firstLineAt *time.Time) (int, error) {
	payload := map[string]any{"flavor": flavor, "fileName": fileName}
	if firstLineAt != nil {
		payload["firstLineAt"] = firstLineAt.UTC().Format(time.RFC3339)
	}
	var out struct {
		ID int `json:"id"`
	}
	b, h := jsonBody(payload)
	_, err := c.do("POST", "/api/uploader/sessions", b, h, &out)
	return out.ID, err
}

// BatchResult: si el sitio ya tenía esa secuencia (reintento), Duplicate=true y OffsetEnd dice
// hasta dónde quedó guardada: la app sigue desde ahí para no duplicar ni perder líneas.
type BatchResult struct {
	Sent      int
	Duplicate bool  `json:"duplicate"`
	OffsetEnd int64 `json:"offsetEnd"`
}

// RestartSession borra en el sitio lo subido de un archivo para volver a subirlo entero (por
// ejemplo, porque se leyó con un filtro anterior que no incluía mazmorras).
func (c *Client) RestartSession(sessionID int) error {
	code, err := c.do("POST", fmt.Sprintf("/api/uploader/sessions/%d/restart", sessionID), nil, nil, nil)
	if code == http.StatusNotFound {
		return nil // ya no existía
	}
	return err
}

// SendBatch comprime y sube un lote.
func (c *Client) SendBatch(sessionID, seq int, offsetStart, offsetEnd int64, data []byte) (BatchResult, error) {
	var gz bytes.Buffer
	w, _ := gzip.NewWriterLevel(&gz, gzip.BestCompression)
	w.Write(data)
	w.Close()
	var res BatchResult
	code, err := c.do("POST", fmt.Sprintf("/api/uploader/sessions/%d/batches", sessionID), bytes.NewReader(gz.Bytes()), map[string]string{
		"Content-Type":   "application/gzip",
		"X-Seq":          strconv.Itoa(seq),
		"X-Offset-Start": strconv.FormatInt(offsetStart, 10),
		"X-Offset-End":   strconv.FormatInt(offsetEnd, 10),
	}, &res)
	if code == http.StatusNotFound {
		return res, ErrSessionGone
	}
	res.Sent = gz.Len()
	if !res.Duplicate {
		res.OffsetEnd = offsetEnd
	}
	return res, err
}

// Scans baja el manifiesto de relevamientos (firmado). Con el ETag de la última vez, si no cambió
// el sitio contesta 304 y notModified=true.
func (c *Client) Scans(etag string) (env []byte, newEtag string, notModified bool, err error) {
	req, err := http.NewRequest("GET", c.BaseURL+"/api/uploader/scans", nil)
	if err != nil {
		return nil, "", false, err
	}
	req.Header.Set("User-Agent", "RebirthTrackUploader/"+c.AppVersion)
	req.Header.Set("X-App-Version", c.AppVersion)
	if etag != "" {
		req.Header.Set("If-None-Match", etag)
	}
	res, err := c.HTTP.Do(req)
	if err != nil {
		return nil, "", false, err
	}
	defer res.Body.Close()
	b, _ := io.ReadAll(io.LimitReader(res.Body, 8<<20)) // el contenido de una subida llega a 1 MB de texto (más el JSON)
	switch {
	case res.StatusCode == http.StatusNotModified:
		return nil, etag, true, nil
	case res.StatusCode >= 400:
		var e apiError
		_ = json.Unmarshal(b, &e)
		if e.Message == "" {
			e.Message = res.Status
		}
		return nil, "", false, fmt.Errorf("%s", e.Message)
	}
	return b, res.Header.Get("ETag"), false, nil
}

// FileResult es lo que devuelve el sitio al recibir un archivo de un relevamiento: si ya lo tenía
// y lo que pasó al procesarlo (para la actividad del panel).
type FileResult struct {
	Sent      int
	Duplicate bool     `json:"duplicate"`
	Messages  []string `json:"messages"`
}

// UploadFile es un archivo que pide el manifiesto, tal cual está en disco.
type UploadFile struct {
	Endpoint        string // del manifiesto (validado: /api/uploader/…)
	ScanID          string
	Flavor          string
	Path            string // relativa a la carpeta de la versión, con «/»
	SHA256          string
	ModTime         time.Time
	ManifestVersion int
	Data            []byte
}

// SendFile comprime y sube un archivo de un relevamiento. Devuelve el código HTTP para distinguir
// rechazos definitivos (4xx) de errores que conviene reintentar.
func (c *Client) SendFile(f UploadFile) (FileResult, int, error) {
	var gz bytes.Buffer
	w, _ := gzip.NewWriterLevel(&gz, gzip.BestCompression)
	w.Write(f.Data)
	w.Close()
	var res FileResult
	code, err := c.do("POST", f.Endpoint, bytes.NewReader(gz.Bytes()), map[string]string{
		"Content-Type":       "application/gzip",
		"X-Scan-Id":          f.ScanID,
		"X-Flavor":           f.Flavor,
		"X-Path":             url.PathEscape(f.Path),
		"X-Sha256":           f.SHA256,
		"X-Modified-At":      strconv.FormatInt(f.ModTime.Unix(), 10),
		"X-Manifest-Version": strconv.Itoa(f.ManifestVersion),
	}, &res)
	res.Sent = gz.Len()
	return res, code, err
}
