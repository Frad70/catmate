# PROGRESS — catmate

The loop's only memory. Newest state at the top. Keep it short and true:
a gate is GREEN only if the pasted output below proves it *right now*.

## Gate board

Verdict comes from `tools/gates.sh` — verified working, currently all RED.

| Gate | What it proves | Status | Evidence |
|------|----------------|--------|----------|
| G0 | build + vet + tests clean | RED | no Go code yet |
| G1 | FEN round-trip, zobrist incremental == recomputed | RED | not started |
| G2 | perft matches the whole corpus | RED | not started |
| G3 | fresh oracle-generated positions, corpus ≥ 1000 | RED | corpus seeded, see below |
| G4 | plays 100 clean self-play games over UCI | RED | not started |
| G5 | HEAD ≥ 55% vs tagged `baseline` over 200 games | RED | no baseline tag yet |

## Where things stand

Scaffolding only. The oracle works and the ground-truth corpus is seeded:

- `tools/perft_oracle.py` (python-chess 1.11.2, in `.venv`) — the only writer of
  `testdata/perft.epd`. Verified against the published node counts for the six
  classic positions; they match exactly, so the oracle is trustworthy.
- `testdata/perft.epd` currently holds **157** positions (D1–D4, a few to D5).
- `tools/fuzz.sh <seed> [count]` — G3. `tools/ratchet.sh` — G5.
- `tools/gates.sh` — the gate board; it ran clean on the empty repo and its
  anti-tamper sampling was tested by corrupting a copy of the corpus (caught).
- No Go code exists yet. `go.mod` declares module `github.com/Frad70/catmate`.

## Next step (exactly one)

Start G1: `internal/board` — bitboard `Position`, FEN parse + serialize, and a
test that round-trips every FEN in `testdata/perft.epd`. No move generation yet.

## Suspicions, unverified

- `tools/ratchet.sh` and `tools/fuzz.sh` have never run end-to-end; they call
  `cmd/catmate` and `cmd/gauntlet`, which do not exist yet. Expect to fix their
  flag names once those binaries are real.

## Iteration log

- _(setup)_ repo, spec, gates, oracle, corpus, prompt. No engine code.
