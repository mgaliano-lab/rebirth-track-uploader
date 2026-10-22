#!/usr/bin/env bash
# Valida el formato obligatorio de CHANGELOG.md (el mismo en el sitio, la app y el addon):
#   ## [X.Y.Z] - AAAA-MM-DD   (de la más nueva a la más vieja, sin repetir)
#   ### Para todos            (lo que se anuncia en Discord, sin tecnicismos; al menos un «- …»)
#   ### Técnico               (el detalle real; al menos un «- …»)
# Con una versión como argumento, además exige que sea la primera (la que se está publicando).
#   scripts/changelog-check.sh            → solo el formato
#   scripts/changelog-check.sh 0.2.0      → y que 0.2.0 esté arriba de todo
set -euo pipefail
cd "$(dirname "$0")/.."
want="${1:-}"
awk -v want="$want" '
  function close_entry() {
    if (ver == "") return
    if (!pub) { print ver ": falta «### Para todos» con al menos un ítem"; bad = 1 }
    if (!tech) { print ver ": falta «### Técnico» con al menos un ítem"; bad = 1 }
  }
  function key(v,   p) { split(v, p, "."); return sprintf("%09d%09d%09d", p[1], p[2], p[3]) }
  /^## / {
    close_entry(); ver = ""; sec = ""; pub = 0; tech = 0
    if (match($0, /^## \[[0-9]+\.[0-9]+\.[0-9]+\] - [0-9][0-9][0-9][0-9]-[0-9][0-9]-[0-9][0-9][ \t]*$/)) {
      v = $0; sub(/^## \[/, "", v); sub(/\].*/, "", v)
      n++; if (n == 1) first = v
      if (prev != "" && key(v) >= key(prev)) { print v ": las versiones van de la más nueva a la más vieja, sin repetir"; bad = 1 }
      prev = v; ver = v
    } else { print "título de versión inválido: «" $0 "» (tiene que ser «## [X.Y.Z] - AAAA-MM-DD»)"; bad = 1 }
    next
  }
  ver != "" && /^### / {
    sec = substr($0, 5); sub(/[ \t]+$/, "", sec)
    if (sec != "Para todos" && sec != "Técnico") { print ver ": sección desconocida «" sec "» (solo «Para todos» y «Técnico»)"; bad = 1 }
    next
  }
  ver != "" && /^[ \t]*- [^ ]/ { if (sec == "Para todos") pub = 1; else if (sec == "Técnico") tech = 1 }
  END {
    close_entry()
    if (n == 0) { print "no hay ninguna versión"; bad = 1 }
    if (want != "" && first != want) { print "la versión " want " tiene que ser la primera de CHANGELOG.md (hoy arriba está «" first "»)"; bad = 1 }
    exit bad
  }
' CHANGELOG.md >&2 || { echo "CHANGELOG.md no cumple el formato (ver arriba)." >&2; exit 1; }
