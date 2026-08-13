# catmate — specification & gates

A UCI chess engine in Go, written by a cat, verified by something that cannot
be sweet-talked: node counts.

## Why this shape

Every milestone below has an **oracle** — an independent source of truth the
engine cannot argue with. Move generation is checked against `python-chess`
(a separate implementation, never against catmate itself). Playing strength is
checked against catmate's own previous binary in a head-to-head match. There is
no gate whose verdict is "looks right to me".

## Layout

    cmd/catmate/      UCI binary + CLI subcommands (perft, perftsuite, divide)
    cmd/gauntlet/     self-play match runner (drives two UCI processes)
    internal/board/   bitboards, FEN, zobrist, make/unmake
    internal/movegen/ move generation (magic or kindergarten, engine's choice)
    internal/search/  iterative deepening, alpha-beta, quiescence, TT
    internal/eval/    evaluation
    tools/            oracle + fuzz scripts (python-chess lives here)
    testdata/         ground truth — APPEND-ONLY, oracle-generated

## Gates

Gates are ordered. Do not start gate N+1 while gate N is red.
Each gate names the exact command that decides it.

### G0 — hygiene
    go build ./... && go vet ./... && go test ./...
All three exit 0. No skipped tests, no `t.Skip`, no build tags hiding code.

### G1 — board & FEN
    go test ./internal/board/... -run 'FEN|Zobrist' -v
- every FEN in `testdata/perft.epd` survives parse → serialize → identical string
- zobrist maintained incrementally equals zobrist recomputed from scratch,
  checked at every ply of 10k random-walk plies (including castling, en passant,
  promotion, and null moves if implemented)

### G2 — move generation is exactly right
    go run ./cmd/catmate perftsuite testdata/perft.epd
Prints one line per mismatch and ends with `mismatches: 0`. Nonzero → gate red.

When a mismatch appears, hunt it: `cmd/catmate divide <fen> <depth>` against
`tools/perft_oracle.py divide "<fen>" <depth>`, descend into the first differing
move, repeat until the position is one ply deep. That is the mouse. Fix it,
then add that position to the corpus via the oracle.

### G3 — fresh fuzz, no memorisation
    tools/fuzz.sh <new-seed> <count>
Generates positions the engine has never seen, with the oracle, appends them to
`testdata/perft.epd`, and reruns G2. Gate is green when the corpus holds
**≥ 1000 positions** and `mismatches: 0`.

### G4 — it actually plays
    go run ./cmd/gauntlet -games 100 -movetime 50 -a ./bin/catmate -b ./bin/catmate
Requirements, all counted by the gauntlet itself and printed as a summary:
- 0 illegal moves, 0 protocol violations, 0 crashes, 0 timeouts
- every game ends by mate, stalemate, 50-move, threefold, or insufficient material
- UCI surface: `uci`, `isready`, `ucinewgame`, `position [startpos|fen] moves ...`,
  `go movetime|depth|wtime btime winc binc`, `stop`, `quit`
- search: iterative deepening + alpha-beta + quiescence + transposition table,
  and it must respect `movetime` (no move takes > 2× the budget)

### G5 — strength ratchet
    tools/ratchet.sh
Builds the binary at git tag `baseline` into `bin/catmate-baseline`, builds HEAD
into `bin/catmate`, plays 200 games at 100ms/move from a book of varied openings
(sample FENs from `testdata/perft.epd`), and reports score with error bars.
Gate is green when HEAD scores **≥ 55%** against `baseline`. Then move the tag
and the ratchet clicks one notch forward.

## Rules of engagement

1. `testdata/*.epd` is ground truth. It is **append-only** and may only be
   written by `tools/perft_oracle.py`. Never hand-edit it, never delete a line,
   never "fix" a count to match the engine. If the engine and the oracle
   disagree, the engine is wrong — until a `divide` trace proves otherwise, in
   writing, in `docs/JOURNAL.md`.
2. Never weaken a gate to pass it. No skipping tests, no lowering thresholds,
   no reducing corpus size, no shrinking game counts.
3. Speed is a feature but never before correctness. Optimise only what a
   benchmark says is slow (`go test -bench`).
4. Every behavioural fix gets a regression case in the corpus or a Go test.
