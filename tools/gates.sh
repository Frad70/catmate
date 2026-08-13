#!/usr/bin/env bash
# The gate board. One command, one verdict per gate, no room for opinion.
#
#   tools/gates.sh            run every gate
#   tools/gates.sh G0 G1 G2   run a subset
#
# A gate is GREEN only if its command says so here. Never edit a threshold in
# this file to make a gate pass; that is the one thing the loop must not do.
set -uo pipefail
cd "$(dirname "$0")/.."
PY=".venv/bin/python"
WANT=("$@")

want() { [[ ${#WANT[@]} -eq 0 ]] && return 0; local g; for g in "${WANT[@]}"; do [[ "$g" == "$1" ]] && return 0; done; return 1; }
declare -A RESULT
green() { RESULT[$1]=GREEN; echo "  ✅ $1 GREEN"; }
red()   { RESULT[$1]=RED;   echo "  ❌ $1 RED — $2"; }

# --- G0: hygiene ------------------------------------------------------------
if want G0; then
  echo "G0 hygiene"
  pkgs=$(go list ./... 2>/dev/null | wc -l)
  skips=$(grep -rn --include='*_test.go' -E '\bt\.Skip' . | grep -v '^./.venv' | wc -l)
  if [[ $pkgs -lt 4 ]]; then red G0 "only $pkgs packages; the engine is not built yet (need >= 4)"
  elif [[ $skips -gt 0 ]]; then red G0 "$skips t.Skip in tests — gates may not be skipped"
  elif ! go build ./... 2>&1; then red G0 "go build failed"
  elif ! go vet ./... 2>&1; then red G0 "go vet failed"
  elif ! go test ./... 2>&1 | tail -20; then red G0 "go test failed"
  else green G0; fi
fi

# --- G1: board & FEN --------------------------------------------------------
if want G1; then
  echo "G1 board & zobrist"
  if [[ ! -d internal/board ]]; then red G1 "internal/board does not exist"
  elif go test ./internal/board/... -run 'FEN|Zobrist' -count=1 2>&1 | tail -10; then green G1
  else red G1 "board tests failed"; fi
fi

# --- corpus integrity (guards G2 and G3) ------------------------------------
if want G2 || want G3; then
  echo "corpus integrity (anti-tamper sample)"
  if [[ -x "$PY" ]] && "$PY" tools/perft_oracle.py check testdata/perft.epd --count 5 --seed "$$"; then
    echo "  corpus verified against the oracle"
  else
    echo "  ❌ CORPUS TAMPERED OR ORACLE MISSING — fix this before anything else"
    RESULT[G2]=RED; RESULT[G3]=RED
  fi
fi

# --- G2: perft --------------------------------------------------------------
if want G2 && [[ "${RESULT[G2]:-}" != RED ]]; then
  echo "G2 perft vs corpus"
  out=$(go run ./cmd/catmate perftsuite testdata/perft.epd 2>&1); echo "$out" | tail -15
  if grep -q 'mismatches: 0' <<<"$out"; then green G2; else red G2 "perft disagrees with the oracle"; fi
fi

# --- G3: corpus breadth -----------------------------------------------------
if want G3 && [[ "${RESULT[G3]:-}" != RED ]]; then
  echo "G3 corpus breadth"
  n=$(wc -l < testdata/perft.epd)
  if [[ $n -lt 1000 ]]; then red G3 "corpus holds $n positions, need >= 1000 (tools/fuzz.sh <seed> 200)"
  elif [[ "${RESULT[G2]:-}" == GREEN ]]; then green G3
  else red G3 "G2 must be green first"; fi
fi

# --- G4: it plays -----------------------------------------------------------
if want G4; then
  echo "G4 self-play sanity (100 games)"
  if [[ ! -d cmd/gauntlet ]]; then red G4 "cmd/gauntlet does not exist"
  else
    mkdir -p bin && go build -o bin/catmate ./cmd/catmate || { red G4 "build failed"; }
    out=$(go run ./cmd/gauntlet -games 100 -movetime 50 -a ./bin/catmate -b ./bin/catmate 2>&1); echo "$out" | tail -15
    if grep -qE 'illegal: 0' <<<"$out" && grep -qE 'crashes: 0' <<<"$out" && grep -qE 'timeouts: 0' <<<"$out"
      then green G4; else red G4 "illegal moves, crashes or timeouts occurred"; fi
  fi
fi

# --- G5: strength ratchet ---------------------------------------------------
if want G5; then
  echo "G5 strength ratchet vs tag 'baseline'"
  out=$(tools/ratchet.sh 200 100 2>&1); echo "$out" | tail -15
  score=$(grep -oE 'score: [0-9.]+%' <<<"$out" | grep -oE '[0-9.]+' | head -1)
  if [[ -n "$score" ]] && awk "BEGIN{exit !($score >= 55)}"; then green G5
  else red G5 "HEAD scored ${score:-?}% vs baseline, need >= 55%"; fi
fi

echo
echo "=== gate board ==="
allgreen=1
for g in G0 G1 G2 G3 G4 G5; do
  want "$g" || continue
  s="${RESULT[$g]:-RED}"; printf '  %s %s\n' "$g" "$s"
  [[ "$s" == GREEN ]] || allgreen=0
done
[[ ${#WANT[@]} -eq 0 && $allgreen -eq 1 ]] && echo "ALL GATES GREEN" || echo "not all green"
exit 0
