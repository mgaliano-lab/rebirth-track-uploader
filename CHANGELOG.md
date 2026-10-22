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
- **Tu privacidad primero**: no lee ni sube nada hasta que aceptás y vinculás la PC con tu Discord; en la web ves y borrás todo lo que subió.

### Técnico

- Go para Windows (`fyne.io/systray`), panel local en `127.0.0.1:47812` con clave por ejecución y chequeo de `Host`, en ventana WebView2 o el navegador. Instalador NSIS por usuario (se autoactualiza desde GitHub Releases) y MSIX para la Microsoft Store; build en Docker y firma Authenticode enchufable.
- Lectura incremental de `Logs\WoWCombatLog*.txt` con filtro de bandas y mazmorras en la PC (listas de dificultades y mapas desde `/api/uploader/config`); lotes gzip idempotentes por secuencia y reanudación exacta por offset.
- Vinculación tipo device code con token por PC cifrado con DPAPI; estado en `%APPDATA%\RebirthTrack`.
- Relevamientos remotos (`internal/scans`): manifiesto firmado con Ed25519 desde `/api/uploader/scans` con ETag y caché local; rutas relativas validadas (sin `..`, solo `.lua`/`.txt`, sin seguir enlaces), topes de tamaño y cantidad; sube cada archivo cuando cambia su SHA-256. Nunca ejecuta nada de lo que baja.
- CI: build y tests en cada push; con el tag `vX.Y.Z` arma el GitHub Release con las notas de este changelog, lo anuncia en Discord (token OIDC de GitHub contra `/api/internal/releases`) y manda el MSIX a certificación con `msstore`. Un workflow programado anuncia la versión de la Store cuando Microsoft la publica.
