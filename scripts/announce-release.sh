#!/usr/bin/env bash
# Anuncia una versión en el canal de novedades de Discord a través del sitio, con lo de «Para todos»
# de CHANGELOG.md. Se autentica con el token OIDC de GitHub Actions (el job necesita
# `permissions: id-token: write`): el sitio verifica que viene de este repo y de su tag (o de main,
# para la Store). Si esa versión ya se anunció, el sitio contesta «already» y no repite.
#   scripts/announce-release.sh app 0.4.0
set -euo pipefail
cd "$(dirname "$0")/.."
product="${1:?producto: app|store|addon}"; version="${2:?versión}"
SITE="${REBIRTH_TRACK_SITE:-https://rebirthknights.fun}"
: "${ACTIONS_ID_TOKEN_REQUEST_URL:?falta el token OIDC: el job necesita permissions: id-token: write}"
token=$(curl -sS --fail-with-body -H "Authorization: Bearer $ACTIONS_ID_TOKEN_REQUEST_TOKEN" \
  "$ACTIONS_ID_TOKEN_REQUEST_URL&audience=rebirthknights.fun/releases" | jq -r .value)
notes=$(scripts/changelog-section.sh "$version" public)
body=$(jq -n --arg p "$product" --arg v "$version" --arg n "$notes" '{product: $p, version: $v, notes: $n}')
curl -sS --fail-with-body -X POST -H "Authorization: Bearer $token" -H 'Content-Type: application/json' \
  --data "$body" "$SITE/api/internal/releases"
echo
