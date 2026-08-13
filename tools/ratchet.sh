#!/usr/bin/env bash
# G5: HEAD must out-play the tagged baseline, or it does not ship.
#
#   tools/ratchet.sh [games] [movetime_ms]
set -euo pipefail
cd "$(dirname "$0")/.."
ROOT="$(pwd)"

GAMES="${1:-200}"
MOVETIME="${2:-100}"

git rev-parse -q --verify refs/tags/baseline >/dev/null || {
  echo "no 'baseline' tag yet. Tag the first playable engine: git tag baseline" >&2
  exit 1
}

mkdir -p bin
go build -o bin/catmate ./cmd/catmate

WORK="$(mktemp -d)"
trap 'rm -rf "$WORK"' EXIT
git worktree add -q --detach "$WORK" baseline
( cd "$WORK" && go build -o "$ROOT/bin/catmate-baseline" ./cmd/catmate )
git worktree remove --force "$WORK"
trap - EXIT

go run ./cmd/gauntlet \
  -games "$GAMES" -movetime "$MOVETIME" \
  -a ./bin/catmate -b ./bin/catmate-baseline \
  -openings testdata/perft.epd
