#!/usr/bin/env bash
# Sube el instalador de out/ al CDN y actualiza uploader/latest.json (lo que ofrece la web).
# Uso: scripts/publish-cdn.sh <versión>   (en local: AWS_PROFILE=mgaliano)
set -euo pipefail
VERSION="$1"
BUCKET=rebirthknights-assets
DIST_ID=E360BSSXQTTMVZ
CDN=https://ast.rebirthknights.fun
SETUP="RebirthTrackUploader-Setup-$VERSION.exe"
[[ -f "out/$SETUP" ]] || { echo "No existe out/$SETUP" >&2; exit 1; }
if aws s3api head-object --bucket "$BUCKET" --key "uploader/$SETUP" >/dev/null 2>&1; then
  echo "La versión $VERSION ya está publicada" >&2; exit 1
fi
SIZE=$(wc -c < "out/$SETUP" | tr -d ' ')
SHA=$(sha256sum "out/$SETUP" 2>/dev/null | cut -d' ' -f1 || shasum -a 256 "out/$SETUP" | cut -d' ' -f1)
aws s3 cp --only-show-errors "out/$SETUP" "s3://$BUCKET/uploader/$SETUP" \
  --content-type application/vnd.microsoft.portable-executable \
  --cache-control "public, max-age=31536000, immutable" --content-disposition "attachment; filename=\"$SETUP\""
cat > out/latest.json <<JSON
{ "version": "$VERSION", "file": "$SETUP", "url": "$CDN/uploader/$SETUP", "size": $SIZE, "sha256": "$SHA", "publishedAt": "$(date -u +%Y-%m-%dT%H:%M:%SZ)" }
JSON
aws s3 cp --only-show-errors out/latest.json "s3://$BUCKET/uploader/latest.json" --content-type application/json --cache-control "public, max-age=60"
aws cloudfront create-invalidation --distribution-id "$DIST_ID" --paths /uploader/latest.json >/dev/null
echo "Publicado: $CDN/uploader/$SETUP"
