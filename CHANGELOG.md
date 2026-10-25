# Changelog

Formato obligatorio (lo valida `scripts/changelog-check.sh` en CI): una sección por versión, de la
más nueva a la más vieja, titulada `## [X.Y.Z] - AAAA-MM-DD`, con dos partes y al menos un ítem en
cada una:

- `### Para todos`: lo que se anuncia en el canal de novedades de Discord (el release con el link
  del instalador y, cuando Microsoft la publica, la Store). Para jugadores, sin tecnicismos.
- `### Técnico`: el detalle real del cambio, para quien mantiene el código. Va, junto con lo de
  arriba, en las notas del GitHub Release.

Al taggear `vX.Y.Z` la versión tiene que estar arriba de todo. La 0.4.0 es el inicio de este
changelog: resume todo lo que hace la app hasta ahí.

## [0.5.4] - 2026-10-25

### Para todos

- **Lo subido, a la vista**: en Actividad, el botón «Lo subido» muestra todo lo que esta PC mandó a la web, separado por tipo (registros de combate con cada lote, y cada tipo de archivo del juego con sus versiones). Con «Ver» se abre exactamente lo que recibió el sitio: las líneas del registro o el archivo, y lo que el sitio tomó de él.

### Técnico

- Panel: selector «Registro / Lo subido» en `#activitySec` y visor modal (Esc o clic afuera para cerrar). Datos del sitio (≥ 0.8.10) vía `GET /api/uploads` y `GET /api/uploads/data?kind=batch|file&id=` del panel → `api.Uploads` / `api.UploadData` (`/api/uploader/uploads…`), pasados tal cual. Se consulta al abrir la vista o con «Actualizar», no en el refresco de 3 s. El límite de lectura de respuestas del sitio sube de 1 a 8 MB (el contenido llega a 1 MB de texto).

## [0.5.3] - 2026-10-25

### Para todos

- **La actividad se ve siempre**: el registro de lo que va haciendo la app queda abajo de todo en cualquier pestaña, no solo en Inicio.
- **Configuraciones en un solo lugar**: la pestaña «World of Warcraft» pasa a llamarse «Configuraciones» y junta las opciones de la app (inicio con Windows y actualización) con las de World of Warcraft (carpeta, versiones y addon). La pestaña «Cuenta y app» queda como «Cuenta».
- **La ventana abre del tamaño justo**: la primera vez ocupa el alto disponible de la pantalla para que entre todo lo que hay que leer y aceptar, y en pantallas chicas o con zoom de Windows ya no queda cortada ni con la barra de título afuera.

### Técnico

- Panel (`internal/ui/index.html`): `#activitySec` sale de la pestaña `home` y va al final de `main` (con alto máximo y scroll); `#appSec` pasa de `cfg` a `wow`, arriba de `#wow`. Los ids de pestaña y los atajos 1–5 no cambian.
- Ventana (`internal/window/window_windows.go`, `placeWindow`): sin tamaño guardado abre de 1120 × hasta 1280 (lo que mide la vista de la primera vez, ~1220) achicado al área útil del monitor (`GetMonitorInfoW`, sin la barra de tareas) y centrada ahí. Antes era fija de 1120×780 y centrada con `SM_CYSCREEN` en aritmética sin signo: en pantallas de menos de 780 de alto lógico (ej. 1080p al 150 %) quedaba con la barra de título fuera de la pantalla. La posición guardada y el mínimo (900×620) también se ajustan al área útil. La app no declara DPI, así que todo va en unidades lógicas.

## [0.5.2] - 2026-10-25

### Para todos

- **Te enterás al toque si desvinculás la PC desde la web**: la app se abre sola y te avisa que la desvincularon (y cuándo). Si la PC estaba apagada, te avisa apenas arranca.

### Técnico

- La app mantiene abierto el stream de avisos del sitio (`GET /api/uploader/events`, SSE, sitio ≥ 0.8.9) mientras está vinculada: con `revoked` (o un 401 al reconectar) borra el token, guarda `UnlinkedAt` y llama a `OnUnlinked` (abre el panel). Antes solo se enteraba cuando intentaba subir algo y el sitio le contestaba 401. Reconecta sola: 5 s después de un corte, con espera creciente hasta 2 min si falla, 10 min si el sitio no tiene el endpoint; si pasan 90 s sin ping, da la conexión por colgada.
- Un 401 solo desvincula si sigue siendo el token con el que se preguntó (no el de una vinculación posterior). El panel muestra «Desvinculada» y la fecha hasta que se vuelve a vincular; desvincular a mano no deja el aviso.

## [0.5.1] - 2026-10-24

### Para todos

- **Sin casilla «Enviar mi balance»**: el addon ya no anota el balance, las bolsas ni el banco de nadie (de oro e inventario, solo los de la guild), así que la casilla no hace falta.

### Técnico

- Fuera `state.NoBalance`, `StripBalance`, `X-Balance` y la escritura de `AppSettings.lua`; la pasada de relevamientos borra el `AppSettings.lua` que dejó la v0.5.0 en la carpeta del addon (el addon 0.3.2 ya no lo carga).

## [0.5.0] - 2026-10-23

### Para todos

- **Enviar mi balance**: casilla nueva en «Qué se releva». Activada, el addon anota el balance de tus personajes y la app lo sube; si la desactivás, el addon deja de anotarlo, la app no lo manda y el sitio borra el que tenía. El balance del banco de la guild se manda igual.

### Técnico

- `state.NoBalance` (casilla desactivada). La app escribe `Interface/AddOns/RebirthTrack/AppSettings.lua` (`RebirthTrackApp = { balance = … }`) en cada versión con el addon instalado: al cambiar la casilla, al instalar el addon y en cada pasada de relevamientos si cambió. El addon (v0.3.1) solo anota `mo1` con `balance = true`.
- Con la casilla desactivada, `StripBalance` saca las líneas `mo1` del archivo antes de subirlo (por si el juego todavía no recargó el addon), y cada subida manda `X-Balance: on|off`; el sitio (v0.5.1) solo acepta el balance con `on`.

## [0.4.0] - 2026-10-22

### Para todos

- **Tus bandas y mazmorras, solas en la web**: la app lee los registros de combate que escribe WoW y sube solo lo que pasa adentro de bandas y mazmorras. Nada de mundo abierto, JcJ ni chat.
- **El addon siempre al día**: instala y actualiza el addon Rebirth Track en las versiones de WoW que elijas.
- **Guild y vínculos sin esfuerzo**: lo que anota el addon (la lista de la guild, los personajes que pediste vincular y los grupos de banda) llega solo al sitio.
- **Mejoras sin actualizar la app**: qué datos del juego se recolectan lo decide el sitio con una lista firmada, así que sumar información nueva ya no necesita una versión de la app. Siempre ves en el panel qué se releva.
- **Todas las versiones de WoW**: Retail, WoW Forever, Classic Anniversary, Classic y Classic Era; encuentra la carpeta sola.
- **Registros anteriores**: podés importar las bandas que jugaste antes de instalarla.
- **Cómoda y a tu ritmo**: vive en la bandeja, tiene su propio panel con pasos para empezar, elegís cada cuánto sube y la podés pausar cuando quieras.
- **Se actualiza sola** con el instalador; desde la Microsoft Store, se actualiza desde la Store.
- **Al día y más segura**: todos sus componentes actualizados a las últimas versiones.
- **Tu privacidad primero**: no lee ni sube nada hasta que aceptás y vinculás la PC con tu Discord; en la web ves y borrás todo lo que subió.

### Técnico

- Go para Windows (`fyne.io/systray`), panel local en `127.0.0.1:47812` con clave por ejecución y chequeo de `Host`, en ventana WebView2 o el navegador. Instalador NSIS por usuario (se autoactualiza desde GitHub Releases) y MSIX para la Microsoft Store; build en Docker y firma Authenticode enchufable.
- Lectura incremental de `Logs\WoWCombatLog*.txt` con filtro de bandas y mazmorras en la PC (listas de dificultades y mapas desde `/api/uploader/config`); lotes gzip idempotentes por secuencia y reanudación exacta por offset.
- Vinculación tipo device code con token por PC cifrado con DPAPI; estado en `%APPDATA%\RebirthTrack`.
- Relevamientos remotos (`internal/scans`): manifiesto firmado con Ed25519 desde `/api/uploader/scans` con ETag y caché local; rutas relativas validadas (sin `..`, solo `.lua`/`.txt`, sin seguir enlaces), topes de tamaño y cantidad; sube cada archivo cuando cambia su SHA-256. Nunca ejecuta nada de lo que baja.
- Dependencias al día: Go 1.27.3, `godbus/dbus` 5.2.4, imágenes base `golang:1.27-trixie` y `debian:trixie-slim` (OpenJDK 21 para jsign 7.6, el 17 ya no está en trixie).
- CI: build y tests en cada push; con el tag `vX.Y.Z` arma el GitHub Release con las notas de este changelog, lo anuncia en Discord (token OIDC de GitHub contra `/api/internal/releases`) y manda el MSIX a certificación con `msstore`. Un workflow programado anuncia la versión de la Store cuando Microsoft la publica.
