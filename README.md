# Rebirth Track Uploader

App de Windows de la guild **Rebirth Knights**. Hace dos cosas:

- instala y actualiza el addon **Rebirth Track**;
- sube a [rebirthknights.fun](https://rebirthknights.fun) los **tramos de bandas y mazmorras** de los
  registros de combate que escribe World of Warcraft.

Corre en la bandeja del sistema. El panel se abre en el navegador, en
`http://127.0.0.1:47812`, y solo es accesible desde la misma PC.

## Qué hace y qué sube

| En la PC | Se sube | No se sube nunca |
|---|---|---|
| Instala el addon en `Interface\AddOns` de las versiones elegidas. | Solo las líneas del log entre un `ZONE_CHANGE` a una banda o mazmorra (por dificultad o, si el juego manda 0, por id de mapa) y la salida (filtrado en la PC: [`internal/logs`](internal/logs/logs.go)). | Mundo abierto, JcJ. |
| Lee `Logs\WoWCombatLog*.txt`. | Lotes gzip cada 3 min por HTTPS, a nombre de la cuenta vinculada. | Chat, credenciales de Battle.net, capturas, teclas, otros archivos. |
| Lee los archivos que pide el **manifiesto de relevamientos** (hoy `WTF\Account\*\SavedVariables\RebirthTrack.lua`). | Cada archivo entero (gzip) cuando cambia. | Nada fuera de la carpeta de WoW ni que no sea `.lua`/`.txt`. |
| Guarda su configuración en `%APPDATA%\RebirthTrack`. | | |

Hasta que el usuario acepta en el panel y vincula la PC (código que confirma en la web, logueado
con Discord), no se lee ni se sube nada.

De los logs anteriores a la aceptación solo se toma lo que se escriba después. Desde la web se
puede:

- ver todo lo que subió cada PC;
- borrarlo;
- desvincular la PC.

El texto que muestra el instalador está en [installer/que-hace.txt](installer/que-hace.txt).

## Relevamientos (manifiesto remoto)

Qué archivos de la carpeta de WoW se suben (además de los registros de combate) no está fijo en la
app: lo dice el manifiesto que publica el sitio en `GET /api/uploader/scans` (fuente:
`server/data/scans.ts` de raidtrack). Sumar un archivo es un deploy del sitio, no un release de la
app. La app ([`internal/scans`](internal/scans/scans.go), [`internal/app/scans.go`](internal/app/scans.go)):

- lo consulta al arrancar y cada `pollSeconds` (con ETag), lo guarda en `scans.json` por si no hay red
  y, si nunca bajó uno, usa el incluido (`scans.Default`: `RebirthTrack.lua`);
- solo acepta manifiestos **firmados** (Ed25519) por una clave de [`internal/scans/keys.go`](internal/scans/keys.go),
  de un formato (`schema`) que conoce, con `version` mayor o igual a la que tiene y `minAppVersion` ≤ la suya;
- valida cada ruta: relativa a la carpeta de la versión, sin `..`, sin absolutas, comodines `*`/`?`
  solo dentro de un tramo, extensión `.lua` o `.txt`, nada de `WoWCombatLog*`; recorre tramo a tramo
  sin seguir enlaces ni junctions; tope de 8 MB por archivo (o el del manifiesto) y 50 archivos por
  relevamiento;
- sube cada archivo entero cuando cambia su SHA-256, al endpoint del manifiesto (siempre del mismo
  sitio, bajo `/api/uploader/`). **Nunca ejecuta nada** de lo que baja: es configuración (política
  10.2.2 de la Microsoft Store);
- muestra en el panel (pestaña «Qué hace» → «Qué se releva») el manifiesto vigente, de dónde salió y el
  estado de cada archivo.

Lo que antes la app leía y mandaba por su cuenta (guild `gi1`/`m1`/`k1`, pedidos de vincular `v1`,
grupos `g1`) ahora lo interpreta el sitio a partir de `RebirthTrack.lua`. Los endpoints viejos
(`characters`, `groups`, `guild-sync`) siguen para las apps ≤ 0.2.x.

Para probar contra un sitio local con su propia clave:

```bash
REBIRTH_TRACK_NO_BROWSER=1 go run ./cmd/rebirthtrack --headless --site http://localhost:3101 \
  --state-dir /tmp/rt --scans-pubkey <pública de desarrollo> --panel-port 47813
```

## Estructura

| Carpeta | Qué hay |
|---|---|
| `cmd/rebirthtrack` | `main`: bandeja (`fyne.io/systray`), instancia única y panel. |
| `internal/app` | Ciclo de subida, vinculación, ajustes, inicio con Windows. |
| `internal/logs` | Lectura incremental y filtro de bandas y mazmorras (con tests). |
| `internal/addon` | Descarga de `latest.json` del CDN, verificación SHA-256 e instalación. |
| `internal/wow` | Detecta la carpeta de WoW (registro de Windows y rutas comunes) y sus versiones. |
| `internal/api` | Cliente de `/api/uploader/*` del sitio. |
| `internal/scans` | Manifiesto de relevamientos: firma, validación y búsqueda segura de archivos. |
| `internal/ui` | Panel local, protegido con una clave por ejecución y chequeo de `Host`. |
| `installer/` | Instalador NSIS por usuario, sin admin, con página de consentimiento. |
| `build/sign.sh` | Firma Authenticode enchufable: jsign o comando propio; sin certificado avisa. |

## Build

Todo dentro de Docker, igual en local y en CI:

```bash
docker build --platform linux/amd64 --build-arg VERSION=0.1.0 --target dist -o out .
```

Deja en `out/`:
- el instalador `RebirthTrackUploader-Setup-<v>.exe`;
- el `.exe` suelto;
- `SHA256SUMS.txt`.

Para probar en Mac o Linux contra el sitio local:

```bash
REBIRTH_TRACK_NO_BROWSER=1 go run ./cmd/rebirthtrack --headless --site http://localhost:3100 --state-dir /tmp/rt
```

## CI y releases

[`.github/workflows/build.yml`](.github/workflows/build.yml) corre en runners de GitHub
(`ubuntu-latest`), gratis por ser un repo público.

- **push / PR**: compila, corre los tests y arma el instalador **sin firmar** como artefacto.
- **tag `vX.Y.Z`** (environment `release`, solo tags `v*`):
  1. build **firmado**, con [`CHANGELOG.md`](CHANGELOG.md) validado y esa versión arriba de todo;
  2. GitHub Release con las notas del changelog y anuncio en el canal de novedades de Discord
     (`scripts/announce-release.sh`, autenticado con el token OIDC de GitHub);
  3. el MSIX a certificación en la Microsoft Store (`msstore`, con la app de Entra
     `rebirth-track-uploader-ci` en los secrets `PARTNER_CENTER_*`). Se publica sola al aprobarse.
- **cada 30 min** ([`store-announce.yml`](.github/workflows/store-announce.yml)): si la Store ya
  publicó una versión nueva, la anuncia en Discord con el link de la Store.

### Changelog

Formato obligatorio (lo valida `scripts/changelog-check.sh` en cada build): por versión,
`## [X.Y.Z] - AAAA-MM-DD` con `### Para todos` (lo que se anuncia en Discord, sin tecnicismos) y
`### Técnico`.

### Firma (Authenticode)

`build/sign.sh` firma tres cosas: la app, el instalador y el desinstalador. Se configura con
secrets del environment `release`:

| Proveedor | Secrets |
|---|---|
| SSL.com eSigner, Azure Artifact Signing, DigiCert… (vía [jsign](https://ebourg.github.io/jsign/)) | `SIGN_PROVIDER=jsign`, `JSIGN_STORETYPE`, `JSIGN_KEYSTORE`, `JSIGN_STOREPASS`, `JSIGN_ALIAS` |
| Certum SimplySign u otra CLI | `SIGN_PROVIDER=command`, `SIGN_COMMAND` con `{file}` |

Con la variable `REQUIRE_SIGNING=1` el release falla si no hay firma. Hasta tener el certificado
queda en `0`: el build se arma sin firmar y avisa.

### CDN

`scripts/publish-cdn.sh` sube el instalador y `uploader/latest.json`. En CI necesita los secrets
`CDN_AWS_ACCESS_KEY_ID` / `CDN_AWS_SECRET_ACCESS_KEY` del usuario IAM
`rebirthknights-uploader-ci` (Terraform en `aws/rebirthknights/ci.tf`).
