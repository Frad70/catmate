# PROGRESS — catmate

The loop's only memory. Newest state at the top. Keep it short and true:
a gate is GREEN only if the pasted output below proves it *right now*.

## Gate board

Verdict comes from `tools/gates.sh`.

| Gate | What it proves | Status | Evidence |
|------|----------------|--------|----------|
| G0 | build + vet + tests clean, ≥ 4 packages | **GREEN** | `✅ G0 GREEN` (5 packages) |
| G1 | FEN round-trip, zobrist incremental == recomputed | **GREEN** | `✅ G1 GREEN` — 157 FENs round-trip, `incremental key survived 10000 random mutations` |
| G2 | perft matches the whole corpus | **GREEN** | `nodes: 130649892 in 3.458s` · `mismatches: 0` |
| G3 | fresh oracle positions, corpus ≥ 1000 | RED | `corpus holds 157 positions, need >= 1000` |
| G4 | plays 100 clean self-play games over UCI | RED | `cmd/gauntlet` does not exist |
| G5 | HEAD ≥ 55% vs tagged `baseline` over 200 games | RED | no baseline tag yet |

## Where things stand

**Iteration 5** put the polygraph on the engine. G0, G1 and G2 are green.

- `internal/perft` — plain recursive `Perft` and a `Divide` sorted by move
  string so its output lines up with the oracle's and the two can be diffed
  straight across. No transposition table and no bulk counting at depth 1: a TT
  would merge positions that differ in something the key forgot, and bulk
  counting never makes the leaf moves at all. Both hide the bugs perft exists
  to find.
- `cmd/catmate` — `perft`, `divide`, `perftsuite`. The suite runs positions in
  parallel (they are independent) and sorts the report back into file order so
  a failure list is reproducible.

    ✅ G0 GREEN
    ✅ G1 GREEN
    positions: 157
    nodes: 130649892 in 3.458s (37783889 nps)
    mismatches: 0
    ✅ G2 GREEN

**130,649,892 nodes and not one disagreement with the oracle**, at every depth
the corpus records. Move generation, legality, make/unmake and the zobrist key
all held on the first serious contact — which is pleasant and slightly
suspicious. The corpus is only 157 positions, mostly depth 4; G3 exists exactly
to stop that from being the whole story.

**Iteration 4** made moves reversible, and legality is in.

- `movegen.Make` / `movegen.Unmake` with an `Undo` record (captured piece, prior
  rights, prior en passant square, both clocks, prior key). They live in
  `movegen` rather than `board` because `Move` lives here and `board` must not
  import its own consumer.
- `Unmake` **never assigns the key**. It reverses the board through the same
  mutators `Make` used and lets the incremental key find its own way home, so
  the tests below are proving something rather than watching a value be copied.
- Special cases handled: en passant removes a pawn that is not on the
  destination square; castling hops the rook; a king or rook that moves loses
  the matching right; a rook *captured where it stands* costs its owner the
  right too; a double push arms the en passant square; promotion swaps the piece
  and unpromotes on the way back.
- `GenerateLegal` filters by make-ask-unmake, with castling handled separately:
  the king may not start in check, cross an attacked square, or land on one —
  none of which is visible in the position after the move.

Evidence, all from the corpus rather than from taste:

    depth-1 node counts match the oracle on all 157 positions
    made and unmade 4679 legal moves with no drift
    made and unmade 142283 move pairs

The first line is already perft(1) against ground truth. The other two say the
position — pieces, side, rights, en passant, both clocks, and the key — comes
back byte-identical after every single move and every move pair.

**Iteration 3** built `internal/movegen`: pseudo-legal moves, nothing filtered.

- `Move` is a uint16 — from, to, and a 4-bit flag whose bit 2 means capture and
  bit 3 means promotion, so both questions are one mask. `MoveList` is a fixed
  256-move array, so generation allocates nothing.
- Knights, kings and pawn attacks come from tables built at init by stepping in
  file/rank terms, which is what stops a knight on h1 wrapping onto the a-file.
  Sliders walk rays; no magic bitboards until a benchmark asks for them.
- Pawns: pushes, double pushes, captures, en passant, and all four promotion
  pieces (capture and quiet). Castling checks rights and an empty path only —
  king-in-check, crossing check and landing in check are legality questions and
  belong with the legality filter.
- `Attacked()` walks outward from the square instead of scanning every enemy
  piece, and `InCheck()` sits on top of it. Both are needed for legality next.
- Tests, all passing: the corpus invariant below, 20 moves from the start
  position, jumper and slider counts on an empty board, four promotion pieces,
  promotion-captures flagged as both, en passant generated and flagged, castles
  needing rights *and* a clear path, six attack cases including a blocked rook,
  attack-table symmetry, and move encode/decode over every from-square.

**The strongest check available before make/unmake exists:** every legal move is
pseudo-legal, so the generated count can never fall below the oracle's depth-1
count. Across all 157 corpus positions it never does:

    checked 157 positions; largest pseudo-legal surplus 41 at
    "r1b1k1nr/8/Qp2pb1p/p2p1B1p/1PPP1P2/1N2P1q1/nB6/R3K1NR w Q - 0 26"

A surplus of 41 is exactly what a position with a king in check should look
like: nearly every move is pseudo-legal and nearly none is legal.

**Iteration 2** closed G1 honestly.

- `internal/board/zobrist.go` — `[13][64]` piece-square keys plus side, castling
  mask and en passant file keys, generated by splitmix64 from a fixed seed so
  two builds agree on every hash.
- The position's state fields are now **unexported** behind `Side()`, `Castle()`,
  `EP()`, `Halfmove()`, `Fullmove()` and their setters. That is what makes the
  incremental key safe: a stray `p.Side = Black` would have desynced the hash
  silently, and the transposition table would have lied about it later.
- `Put` now replaces rather than overwrites, so dropping a piece on an occupied
  square cannot leave a stale bit or a stale key term.
- Tests: 10 000 random mutations (relocate, remove, put, flip side, rewrite
  castling, set/clear ep) with `Key() == ComputeKey()` checked after **every**
  one; undo-restores-key; no key collision across the corpus; side to move
  changes the key; the halfmove/fullmove clocks do not leak into it.
- `go vet` caught two `t.Errorf("%v", pos.Side)` leftovers — method values
  printed instead of called. Worth remembering that vet reads format strings.

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

G3: grow the corpus past 1000 positions with **fresh** oracle-generated
positions and rerun G2 against them.

    tools/fuzz.sh <new-seed> 200        # repeat with different seeds

Generation is the slow half — python-chess computes every node itself, roughly
50k nodes per position — so run it in the background and let it finish rather
than shrinking the count. Do **not** reuse seed 20260813; that is the batch
already in the file, and re-verifying positions the engine has already passed
proves nothing.

This is where the remaining mice live, if any do: 157 positions cannot cover
under-promotion under check, double check with an en passant escape, or a
castling right lost to a rook captured on its home square. Every mismatch gets
a `divide` bisect and a journal entry.

OLD (kept for context): Make/unmake in `internal/board` (or a new `internal/position` layer if the
import graph fights back): apply a `Move` to a `Position` and undo it exactly,
maintaining the zobrist key, castling rights, en passant square and both clocks
through every special case — double push sets the ep square, en passant removes
a pawn that is *not* on the destination square, castling moves the rook too, a
rook moving or being captured strips the matching right, and promotion swaps
the piece.

Keep an undo record (captured piece, previous rights, previous ep, previous
halfmove clock, previous key) so unmake needs no recomputation. Then extend the
zobrist test to walk *legal moves* rather than random edits, asserting
`Key() == ComputeKey()` and full state restoration after every unmake. That
walk is where castling and en passant desyncs actually surface.

Legality itself follows the cheap route first: make the move, ask whether our
king is attacked, unmake. Perft will say soon enough whether it is fast enough.

## Suspicions, unverified

- `tools/ratchet.sh` has never run end-to-end; it calls `cmd/gauntlet`, which
  does not exist. `tools/fuzz.sh` should work now that `cmd/catmate` is real,
  but it has still never been run.
- Perft speed is 6.6M nps single-threaded, 37M aggregate across 12 cores. Fast
  enough that no optimisation is justified yet; revisit only if a benchmark
  complains.
- G0 needs ≥ 4 packages; three exist now (`board`, `epd`, `movegen`), so it
  goes green with `cmd/catmate`. That is the gate working as intended, not a blocker: the natural build
  order is board → movegen → cmd/catmate → search/eval, and G0 goes green on
  the way.
- The zobrist walk uses random *edits*, not legal moves, because move generation
  does not exist yet. It exercises the same mutators make/unmake will use, but
  it has never seen a castling rook hop, an en passant capture or a promotion
  performed as a single move. Re-check the key against `ComputeKey()` inside
  make/unmake when they land — that is where the real desyncs hide.
- `zobristCastle[0]` is a non-zero constant, so a position with no castling
  rights carries it. Consistent between both code paths, therefore harmless,
  but surprising if anyone ever compares keys with an external engine.

## Iteration log

- **5** — `internal/perft` + `cmd/catmate` (perft/divide/perftsuite). G0, G1,
  G2 GREEN: 130,649,892 nodes, `mismatches: 0`.
- **4** — `Make`/`Unmake`/`Undo` and `GenerateLegal`. Depth-1 counts match the
  oracle on all 157 positions; 4679 moves and 142283 move pairs undo cleanly.
- **3** — `internal/movegen`: Move/MoveList, attack tables, pseudo-legal
  generation, `Attacked`/`InCheck`. Pseudo-legal ⊇ legal verified on all 157
  corpus positions. G0 still red at 3 packages.
- **2** — zobrist keys, state hidden behind accessors, `Put` made replacing.
  G1 GREEN: 157/157 FENs round-trip and the incremental key survived 10 000
  mutations.
- **1** — `internal/board` (FEN parse/serialize) + `internal/epd`; 157/157 FENs
  round-trip. Hardened G1, which had been green on an unwritten half.
- _(setup)_ repo, spec, gates, oracle, corpus, prompt. No engine code.
