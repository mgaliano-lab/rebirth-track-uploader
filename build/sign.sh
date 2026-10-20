#!/usr/bin/env bash
# Firma Authenticode de un .exe (la app, el instalador y su desinstalador).
# Configuración por variables (en CI vienen de un secret de BuildKit montado en /run/secrets/sign):
#   SIGN_PROVIDER=jsign    → JSIGN_STORETYPE (ESIGNER, TRUSTEDSIGNING, DIGICERTONE, PKCS12…),
#                            JSIGN_KEYSTORE, JSIGN_STOREPASS, JSIGN_ALIAS, [JSIGN_KEYPASS]
#   SIGN_PROVIDER=command  → SIGN_COMMAND con {file} (ej. la CLI de Certum SimplySign)
#   sin SIGN_PROVIDER      → no firma (avisa); con REQUIRE_SIGNING=1 falla.
set -euo pipefail
file="$1"
[[ -f /run/secrets/sign ]] && set -a && . /run/secrets/sign && set +a
TSA="${SIGN_TSA:-http://timestamp.sectigo.com}"

case "${SIGN_PROVIDER:-}" in
  jsign)
    java -jar /opt/jsign.jar \
      --storetype "$JSIGN_STORETYPE" --keystore "$JSIGN_KEYSTORE" --storepass "$JSIGN_STOREPASS" \
      ${JSIGN_ALIAS:+--alias "$JSIGN_ALIAS"} ${JSIGN_KEYPASS:+--keypass "$JSIGN_KEYPASS"} \
      --tsaurl "$TSA" --tsmode RFC3161 --alg SHA-256 \
      --name "Rebirth Track Uploader" --url "https://rebirthknights.fun" \
      "$file"
    ;;
  command)
    eval "${SIGN_COMMAND//\{file\}/\"$file\"}"
    ;;
  "")
    if [[ "${REQUIRE_SIGNING:-0}" == "1" ]]; then
      echo "ERROR: se pidió firma (REQUIRE_SIGNING=1) pero no hay SIGN_PROVIDER configurado" >&2
      exit 1
    fi
    echo "AVISO: $(basename "$file") queda SIN FIRMAR (no hay certificado configurado)" >&2
    ;;
  *)
    echo "SIGN_PROVIDER desconocido: $SIGN_PROVIDER" >&2; exit 1 ;;
esac
