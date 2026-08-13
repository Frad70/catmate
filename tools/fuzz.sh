#!/usr/bin/env bash
# G3: hunt with fresh mice. Generates unseen positions with the oracle,
# appends them to the corpus, then makes the engine prove itself again.
#
#   tools/fuzz.sh <seed> [count] [budget]
set -euo pipefail
cd "$(dirname "$0")/.."

SEED="${1:?usage: tools/fuzz.sh <seed> [count] [budget]}"
COUNT="${2:-100}"
BUDGET="${3:-50000}"
PY=".venv/bin/python"

[[ -x "$PY" ]] || { echo "no oracle: python3 -m venv .venv && .venv/bin/pip install -r tools/requirements.txt" >&2; exit 1; }

before=$(wc -l < testdata/perft.epd)
"$PY" tools/perft_oracle.py gen \
  --out testdata/perft.epd --append \
  --count "$COUNT" --seed "$SEED" --max-depth 5 --budget "$BUDGET"
after=$(wc -l < testdata/perft.epd)
echo "corpus: $before -> $after positions"

go run ./cmd/catmate perftsuite testdata/perft.epd
