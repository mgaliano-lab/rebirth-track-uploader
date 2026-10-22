// Package api habla con Rebirth Track: vinculación de la PC, configuración y subida de lotes.
package api

import (
	"bytes"
	"compress/gzip"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"time"
)

var ErrUnauthorized = errors.New("la PC no está vinculada (o la desvincularon desde la web)")
var ErrPending = errors.New("pendiente")

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
	b, _ := io.ReadAll(io.LimitReader(res.Body, 1<<20))
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

// CharacterLinked es lo que devuelve el sitio al vincular un personaje desde el addon.
type CharacterLinked struct {
	Name  string `json:"name"`
	Realm string `json:"realm"`
	Game  string `json:"game"`
}

// LinkCharacter vincula al usuario de esta PC un personaje pedido desde el addon. Devuelve el
// código HTTP para distinguir errores definitivos (4xx) de los que conviene reintentar.
func (c *Client) LinkCharacter(flavor string, r any) (CharacterLinked, int, error) {
	var out CharacterLinked
	b, h := jsonBody(map[string]any{"flavor": flavor, "character": r})
	code, err := c.do("POST", "/api/uploader/characters", b, h, &out)
	return out, code, err
}

// SendGroups sube los grupos de banda que anotó el addon.
func (c *Client) SendGroups(snapshots any) (int, error) {
	b, h := jsonBody(map[string]any{"snapshots": snapshots})
	return c.do("POST", "/api/uploader/groups", b, h, nil)
}

// GuildSynced es lo que devuelve el sitio al recibir la guild del addon.
type GuildSynced struct {
	Members int `json:"members"`
	Known   int `json:"known"`
}

// SendGuild sube la lista de miembros de la guild y los personajes anunciados por el canal del addon.
func (c *Client) SendGuild(flavor string, guild any) (GuildSynced, int, error) {
	var out GuildSynced
	b, h := jsonBody(map[string]any{"flavor": flavor, "guild": guild})
	code, err := c.do("POST", "/api/uploader/guild-sync", b, h, &out)
	return out, code, err
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
