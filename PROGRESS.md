# PROGRESS — catmate

The loop's only memory. Newest state at the top. Keep it short and true:
a gate is GREEN only if the pasted output below proves it *right now*.

## Gate board

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
- `testdata/perft.epd` — 7 classic positions (startpos to D5, kiwipete to D4,
  and friends) plus ~150 random-walk positions to D3–D5.
- `tools/fuzz.sh <seed> [count]` — G3. `tools/ratchet.sh` — G5.
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
