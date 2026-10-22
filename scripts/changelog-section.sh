#!/usr/bin/env bash
# Imprime una parte de la versión de CHANGELOG.md (formato en scripts/changelog-check.sh):
#   scripts/changelog-section.sh 0.2.0 public      → «Para todos» (lo que se anuncia en Discord)
#   scripts/changelog-section.sh 0.2.0 technical   → «Técnico»
#   scripts/changelog-section.sh 0.2.0 all         → las dos, con sus títulos (notas del release)
set -euo pipefail
cd "$(dirname "$0")/.."
v="${1:?versión}"; part="${2:-all}"
case "$part" in public) want="Para todos" ;; technical) want="Técnico" ;; all) want="" ;; *) echo "parte: public|technical|all" >&2; exit 2 ;; esac
out=$(awk -v v="$v" -v want="$want" '
  /^## / { on = (index($0, "## [" v "] ") == 1); next }
  !on { next }
  /^### / { sec = substr($0, 5); sub(/[ \t]+$/, "", sec); if (want == "") print; next }
  want == "" || sec == want { print }
' CHANGELOG.md | sed -e '/./,$!d' | sed -e :a -e '/^\n*$/{$d;N;ba' -e '}')
[[ -n "${out//[$'\n ']/}" ]] || { echo "CHANGELOG.md no tiene «$part» para la versión $v" >&2; exit 1; }
printf '%s\n' "$out"
