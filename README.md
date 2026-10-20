# Rebirth Track Uploader

App de Windows de la guild **Rebirth Knights**. Hace dos cosas:

- instala y actualiza el addon **Rebirth Track**;
- sube a [rebirthknights.fun](https://rebirthknights.fun) los **tramos de bandas** de los
  registros de combate que escribe World of Warcraft.

Corre en la bandeja del sistema. El panel se abre en el navegador, en
`http://127.0.0.1:47812`, y solo es accesible desde la misma PC.

## Qué hace y qué sube

| En la PC | Se sube | No se sube nunca |
|---|---|---|
| Instala el addon en `Interface\AddOns` de las versiones elegidas. | Solo las líneas del log entre un `ZONE_CHANGE` a una instancia con dificultad de banda y la salida (filtrado en la PC: [`internal/logs`](internal/logs/logs.go)). | Mundo abierto, mazmorras, JcJ. |
| Lee `Logs\WoWCombatLog*.txt`. | Lotes gzip cada 3 min por HTTPS, a nombre de la cuenta vinculada. | Chat, credenciales de Battle.net, capturas, teclas, otros archivos. |
| Guarda su configuración en `%APPDATA%\RebirthTrack`. | | |

Hasta que el usuario acepta en el panel y vincula la PC (código que confirma en la web, logueado
con Discord), no se lee ni se sube nada.

De los logs anteriores a la aceptación solo se toma lo que se escriba después. Desde la web se
puede:

- ver todo lo que subió cada PC;
- borrarlo;
- desvincular la PC.

El texto que muestra el instalador está en [installer/que-hace.txt](installer/que-hace.txt).

## Estructura

| Carpeta | Qué hay |
|---|---|
| `cmd/rebirthtrack` | `main`: bandeja (`fyne.io/systray`), instancia única y panel. |
| `internal/app` | Ciclo de subida, vinculación, ajustes, inicio con Windows. |
| `internal/logs` | Lectura incremental y filtro de bandas (con tests). |
| `internal/addon` | Descarga de `latest.json` del CDN, verificación SHA-256 e instalación. |
| `internal/wow` | Detecta la carpeta de WoW (registro de Windows y rutas comunes) y sus versiones. |
| `internal/api` | Cliente de `/api/uploader/*` del sitio. |
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

[`.github/workflows/build.yml`](.github/workflows/build.yml) corre en los runners del cluster Talos
(`talos-lab-mgaliano-lab`).

- **push / PR**: compila, corre los tests y arma el instalador **sin firmar** como artefacto.
- **tag `vX.Y.Z`**, en el environment `release` (con revisor requerido):
  1. build **firmado**;
  2. GitHub Release;
  3. publicación en `ast.rebirthknights.fun/uploader/`, de donde lo ofrece la web.

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
