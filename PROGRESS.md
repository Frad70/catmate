# PROGRESS — catmate

The loop's only memory. Newest state at the top. Keep it short and true:
a gate is GREEN only if the pasted output below proves it *right now*.

## Gate board

Verdict comes from `tools/gates.sh`.

| Gate | What it proves | Status | Evidence |
|------|----------------|--------|----------|
| G0 | build + vet + tests clean, ≥ 4 packages | RED | `only 2 packages; the engine is not built yet (need >= 4)` |
| G1 | FEN round-trip, zobrist incremental == recomputed | RED | `no passing Zobrist test — half of G1 is unwritten` |
| G2 | perft matches the whole corpus | RED | `cmd/catmate` does not exist |
| G3 | fresh oracle positions, corpus ≥ 1000 | RED | corpus holds 157 |
| G4 | plays 100 clean self-play games over UCI | RED | `cmd/gauntlet` does not exist |
| G5 | HEAD ≥ 55% vs tagged `baseline` over 200 games | RED | no baseline tag yet |

## Where things stand

**Iteration 1** laid the first brick and found a hole in the ruler.

- `internal/board` — `Position` (bitboards by type and colour, plus a mailbox),
  `Piece`/`Square`/`Bitboard`/`CastleRights` types, `ParseFEN`, `Position.FEN()`.
- `internal/epd` — reader for the ground-truth corpus, shared by the board test
  now and by `cmd/catmate perftsuite` later.
- Tests: every one of the **157** corpus FENs round-trips byte-for-byte
  (`round-tripped 157 positions`), start position sanity, square naming, and six
  malformed FENs that must be rejected.
- `go build ./... && go vet ./...` clean, `gofmt` clean.

**The hole:** G1 reported GREEN with no zobrist code anywhere. `go test -run
'FEN|Zobrist'` exits 0 when the filter matches nothing, so an unwritten half of
a gate read as a passing half. `tools/gates.sh` now *counts* passing FEN and
Zobrist tests instead of trusting the exit code. Same species as the earlier
`go build ./...`-on-an-empty-module hole: a command that says nothing is not a
command that says yes. Anything else in the board that leans on an exit code
deserves the same suspicion — G4 and G5 parse text, so they are probably fine,
but nobody has run them yet.

## Next step (exactly one)

Zobrist hashing in `internal/board`: a `[12][64]` piece-square table plus keys
for side to move, castling rights, and the en passant file; `Position.Key()`
computed from scratch, and an incrementally maintained key kept equal to it.
The test has to walk positions and compare the two at every ply — which needs
`Put`/`Remove` to update the key, and, once moves exist, make/unmake. Until
move generation lands, drive it with random legal-ish edits: place, remove and
move pieces around, flip side/castling/EP, and compare after each edit.

## Suspicions, unverified

- `tools/fuzz.sh` and `tools/ratchet.sh` have never run end-to-end; they call
  binaries that do not exist. Expect flag mismatches on first contact.
- G0 needs ≥ 4 packages, so it stays red until `movegen` and `cmd/catmate`
  exist. That is the gate working as intended, not a blocker: the natural build
  order is board → movegen → cmd/catmate → search/eval, and G0 goes green on
  the way.

## Iteration log

- **1** — `internal/board` (FEN parse/serialize) + `internal/epd`; 157/157 FENs
  round-trip. Hardened G1, which had been green on an unwritten half.
- _(setup)_ repo, spec, gates, oracle, corpus, prompt. No engine code.
