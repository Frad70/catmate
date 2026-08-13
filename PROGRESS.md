# PROGRESS — catmate

The loop's only memory. Newest state at the top. Keep it short and true:
a gate is GREEN only if the pasted output below proves it *right now*.

## Gate board

Verdict comes from `tools/gates.sh`.

| Gate | What it proves | Status | Evidence |
|------|----------------|--------|----------|
| G0 | build + vet + tests clean, ≥ 4 packages | **GREEN** | `✅ G0 GREEN` (5 packages) |
| G1 | FEN round-trip, zobrist incremental == recomputed | **GREEN** | `✅ G1 GREEN` — 157 FENs round-trip, `incremental key survived 10000 random mutations` |
| G2 | perft matches the whole corpus | **GREEN** | `nodes: 759394690 in 19.593s` · `mismatches: 0` |
| G3 | fresh oracle positions, corpus ≥ 1000 | **GREEN** | `✅ G3 GREEN` — 1007 positions |
| G4 | plays 100 clean self-play games over UCI | **GREEN** | `illegal: 0` `crashes: 0` `timeouts: 0` over 100 games |
| G5 | HEAD ≥ 55% vs tagged `baseline` over 200 games | RED | no baseline tag yet |

## Where things stand

**Iteration 9** built the referee. G4 is green — the engine plays real games.

- `internal/game` decides endings from the position alone: checkmate,
  stalemate, the fifty-move rule, threefold repetition by position key, and
  insufficient material. No engine is asked for its opinion about the result.
- `cmd/gauntlet` runs two engine processes against each other, alternates
  colours, varies openings with a seeded random walk (two deterministic engines
  from one position would play the same game a hundred times), plays games
  concurrently, and **checks every returned move against its own legal move
  list**. It separates a dead process (crash) from a silent one (timeout),
  because the report is useless if those blur together.

    games: 100 in 1m17.052s
    moves: 8776 total, 87.8 per game
      threefold repetition:  53
      checkmate:             45
      insufficient material: 2
    A: +19 =55 -26
    score: 46.5% (46.5/100) elo -24 +/- 46
    illegal: 0
    crashes: 0
    timeouts: 0
    ✅ G4 GREEN

Every one of the hundred games ended by a real chess rule. Nothing was
adjudicated by the move limit.

**The number that matters is not the score — it is 53.** More than half the
games ended in threefold repetition, which is the search shuffling because it
has no idea repetition exists. It will happily repeat a position it is winning.
That is the first thing to fix before the ratchet measures anything, or G5 will
be measuring noise.

**Iteration 8** gave the engine a mouth: `internal/uci`, wired into
`cmd/catmate` (no arguments now means "speak UCI", which is how a match runner
launches it).

- Handshake, `isready` answered even mid-search, `ucinewgame`, `position
  startpos|fen ... moves ...`, `go depth|nodes|movetime|infinite|wtime btime
  winc binc`, `stop`, `quit`. `info depth ... score cp|mate ... nodes ... pv`
  each iteration, then `bestmove`.
- `position ... moves` matches each UCI string against the *generated legal
  moves* rather than trusting it, so a protocol slip cannot put the engine on a
  board its opponent is not playing. An unmatched move is refused and the
  position left untouched.
- The search runs on its own goroutine; the reader keeps reading, which is what
  makes `stop` and `isready` answerable at all.

**Two mice, both caught by tests written before they were believed in** — see
`docs/JOURNAL.md` 2 and 3:

1. A `stop` arriving before the search goroutine was scheduled was **erased** by
   `Search` clearing the stop flag on entry. In a real game that is an engine
   thinking forever and losing on time. Fixed at the root: `Ready()` arms the
   flag on the reader goroutine, and `Search` never resets it.
2. With that fixed, a mate test began failing with `score mate -5000` — and the
   fault was the *test*, which sent `quit` on top of a running search. No GUI
   does that. The harness now waits for `bestmove` first, as a GUI does.

All eight UCI tests pass under `-race`.

    ✅ G0 GREEN  ✅ G1 GREEN  ✅ G2 GREEN  ✅ G3 GREEN
    ❌ G4 RED — cmd/gauntlet does not exist
    ❌ G5 RED — HEAD scored ?% vs baseline, need >= 55%

**Iteration 7** taught the engine to think. Gates unchanged (G0–G3 green);
this is groundwork for G4.

- `internal/eval` — material plus piece-square tables, scored from the side to
  move. Two king tables: the king hides while there is heavy material on the
  board and walks to the centre once there is not, because without that a won
  K+Q ending dribbles into a fifty-move draw.
- `internal/search` — negamax with alpha-beta, iterative deepening, quiescence
  at the leaves, MVV-LVA move ordering, mate scores adjusted by ply, a
  principal variation table, and a movetime deadline checked every 2048 nodes.
- A depth that runs out of clock is thrown away, not used: half a depth is
  often worse than the whole depth below it.

The two tests that matter, and they are a pair:

    alpha-beta agreed with unpruned negamax on 26 positions at depth 3
    depth 4 from the start position: 26654 nodes with alpha-beta, 206604 without

The first says pruning never changed an answer. The second says it pruned at
all — without it, a search that "agrees with negamax" might simply *be*
negamax. Leaves are static in that comparison, because measuring against
unpruned negamax only means something when the leaf value is a fixed function.

Also passing: both mate-in-1 positions (each confirmed by the oracle to be
legal, mate, and the *only* mate there), the hanging queen taken, a defended
pawn left alone by quiescence, stalemate scored 0, a 100 ms budget respected,
and every move of the returned PV legal in turn.

**Iteration 6** widened the net. G0–G3 are green.

- `tools/perft_oracle.py gen` now takes `--jobs` and generates in worker
  processes. Each position gets a seed derived from the batch seed and its
  index, so a parallel run produces exactly what a sequential one would — the
  corpus stays reproducible. Generation also skips FENs already in the file:
  re-verifying a position the engine has already passed proves nothing.
- Corpus grown **157 → 1007** with seed 90210, a batch the engine had never
  seen. `tools/fuzz.sh` ran end-to-end for the first time.

    corpus: 157 -> 1007 positions
    positions: 1007
    nodes: 759394690 in 19.593s (38757805 nps)
    mismatches: 0
    ✅ G0 GREEN  ✅ G1 GREEN  ✅ G2 GREEN  ✅ G3 GREEN

**759,394,690 nodes against an independent implementation, zero
disagreements.** The suspicion recorded last iteration — that 157 positions
were too few to trust — has been answered: 850 unseen positions, generated by
python-chess and never by catmate, found nothing. Move generation, legality,
make/unmake and the zobrist key are correct as far as an oracle can tell.

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

G4 begins: `internal/search` and `internal/eval`, then the UCI loop in
`cmd/catmate`, then `cmd/gauntlet`. That is more than one step, so take the
first piece only:

**Teach the search about repetition and the fifty-move rule**, then run G5.

The `baseline` tag now exists at the first playable engine (iteration 9), so
`tools/ratchet.sh` can finally run. Do not start tuning before this fix: with 53
of 100 games repeating, the ratchet would mostly measure who shuffles first.

What to do:
- Thread the game history into the search — a slice of position keys, pushed by
  `Make` at the root and by the search as it descends. A position seen before
  *inside the search tree* should score 0 immediately (the usual engine
  shortcut: one repetition inside the tree is treated as a draw, since a second
  is available to whoever wants it).
- Score a draw as 0 from the side to move, which means a winning engine stops
  repeating and a losing one starts trying to.
- Return 0 for `Halfmove() >= 100` too.
- Then re-run G4 and expect the repetition count to collapse. That drop is the
  test; there is no oracle for "played better", which is exactly why G5 is a
  match rather than an opinion.

Only then run `tools/gates.sh G5`.

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

- `tools/ratchet.sh` has still never run end-to-end, but both things it needs
  now exist: `cmd/gauntlet` and the `baseline` tag.
- A 46.5% score between two *identical* binaries came with `elo -24 +/- 46`.
  That is the noise floor of a 100-game match: any strength claim below about
  50 elo needs far more games than the ratchet plays.
- Perft is verified only to depth 5, and most positions only to depth 4. Deep
  tactical horizons (a fifty-move draw, threefold repetition) are not covered by
  perft at all — they belong to the game layer and G4.
- `±Infinity` can still reach the root through an aborted search, where
  `IsMate` reads the sentinel as a forced mate. Harmless now because an aborted
  depth is thrown away, but it is a sentinel escaping its guard rails.
- The search knows nothing about repetition or the fifty-move rule yet, so it
  will happily shuffle in a won position and cannot claim a draw in a lost one.
  The gauntlet must adjudicate both, and the search should learn them before G5,
  or the ratchet will measure noise.
- No transposition table. Adding one is the obvious strength step, but it needs
  the search that exists now as a measuring stick — and a TT bug shows up as
  *nothing* until it quietly returns a wrong move, so it must land with the
  zobrist-key check that G1 already provides.
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

- **9** — `internal/game` (referee) and `cmd/gauntlet` (match runner). G4 GREEN:
  100 games, 0 illegal, 0 crashes, 0 timeouts, every game ended by a chess rule.
  Oracle caught a wrong insufficient-material rule before it shipped.
- **8** — `internal/uci` and the binary's default UCI mode. Caught and fixed a
  lost-`stop` race that would have hung real games, plus a test harness that
  raced every search. Eight UCI tests green under `-race`.
- **7** — `internal/eval` (material + PST, two king tables) and
  `internal/search` (alpha-beta, iterative deepening, quiescence, MVV-LVA, PV).
  Alpha-beta proven equal to unpruned negamax and proven to actually prune.
- **6** — parallel oracle generation (`--jobs`), corpus 157 → 1007 with a fresh
  seed. G3 GREEN. 759,394,690 nodes, `mismatches: 0`.
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
