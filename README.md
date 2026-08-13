# catmate

A UCI chess engine in Go, written by a cat, one loop iteration at a time.

The interesting part is not the engine — it is the harness. Every milestone is
decided by an oracle that cannot be talked out of its answer:

- **Move generation** is checked against [python-chess](https://python-chess.readthedocs.io/),
  a separate implementation, over a growing corpus of perft node counts
  (`testdata/perft.epd`, append-only, machine-written). A single wrong node at
  depth 5 fails the gate, and a `divide` bisect walks straight to the bug.
- **Playing strength** is checked against catmate's own previous binary in a
  200-game self-play match. New code ships only if it wins.

So the loop cannot declare victory by lowering its standards. It has to
actually catch the mouse.

## Layout

    PROMPT.md         the loop prompt — this is what drives development
    docs/SPEC.md      gates G0–G5, each with the command that decides it
    PROGRESS.md       the loop's memory between iterations
    docs/JOURNAL.md   one entry per hunted bug
    tools/            the oracle (python) and the gate scripts

## Setup

    python3 -m venv .venv && .venv/bin/pip install -r tools/requirements.txt
    go build ./...

## Gates

    go build ./... && go vet ./... && go test ./...        # G0
    go test ./internal/board/... -run 'FEN|Zobrist' -v      # G1
    go run ./cmd/catmate perftsuite testdata/perft.epd      # G2
    tools/fuzz.sh 424242 100                                # G3
    go run ./cmd/gauntlet -games 100 -movetime 50 ...       # G4
    tools/ratchet.sh                                        # G5

Purr when they are green. Hiss when they are not.
