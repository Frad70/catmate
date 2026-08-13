# JOURNAL — the mice

One entry per hunted bug. Symptom → divide trace → root cause → regression case.
This file exists so the same mouse is never caught twice.

_(empty — the hunt has not started)_

## 1 — a gate that was green because nothing ran

**Symptom.** With `internal/board` holding FEN code and no zobrist code at all,
`tools/gates.sh G1` printed GREEN.

**Trace.** G1 ran `go test ./internal/board/... -run 'FEN|Zobrist'`. The filter
matched the FEN tests, matched no Zobrist test, and `go test` exits 0 when a
`-run` pattern matches nothing. So the gate asked "did anything fail?" when it
meant to ask "did both halves pass?".

**Root cause.** The gate trusted an exit code to prove presence. An exit code
proves absence of failure, which is not the same claim — the same mistake as
`go build ./...` exiting 0 on a module with no packages.

**Fix.** G1 now counts `--- PASS:` lines for FEN and for Zobrist separately and
demands at least one of each, and it fails loudly if either is missing.

**Regression case.** The gate itself: with zobrist unwritten, G1 must read
`no passing Zobrist test`, and it does.

## 2 — the stop that arrived too early

**Symptom.** `TestStopBeforeTheSearchStarts` hung for its full five seconds:
the engine was sent `go infinite` immediately followed by `stop`, and never
answered with a move.

**Trace.** `cmdGo` spawned a goroutine and returned; the reader then handled
`stop`, which set the searcher's stop flag. Meanwhile the search goroutine had
not started yet. When it finally ran, `Search` began with
`s.stopped.Store(false)` — clearing the very stop that had just arrived. An
infinite search then had nothing left to end it.

**Root cause.** State that a caller sets before an operation begins must not be
reset *inside* that operation. The reset and the arrival raced, and the reset
won whenever the goroutine scheduled late.

**Fix.** `Search` no longer clears the flag. `Searcher.Ready()` does, and the
UCI layer calls it on the reader goroutine *before* launching the search, so a
stop can only ever land after the arming.

**Regression case.** `TestStopBeforeTheSearchStarts` — it fails in exactly the
old way if the reset moves back inside `Search`.

**Why it mattered.** In a real match this is an engine that thinks forever on
one move: the game is lost on time and the whole gauntlet stalls behind it.

## 3 — a test that lied about the protocol

**Symptom.** After fixing the stop race, `TestMateIsReportedInMoves` failed
with `info depth 1 score mate -5000 nodes 2`. Two nodes for a depth-3 search,
and a mate score of minus five thousand moves.

**Trace.** The score decoded as `+Infinity` reaching the root: a child returned
`alpha` from its out-of-time check, the parent negated it, and `IsMate` dutifully
read the sentinel as a forced mate. The out-of-time flag was set because the
test script fed `go depth 3` and `quit` in the same breath, and `quit` stops the
search — correctly.

**Root cause.** The test harness, not the engine. A GUI never sends `quit` on
top of a running search; it waits for `bestmove`. The old harness had been
racing every search, and only looked correct because the previous bug made stops
disappear.

**Fix.** The harness now drives the engine like a GUI: it waits for the
`bestmove` of every outstanding `go` before sending `quit`.

**Note kept.** The engine still lets `±Infinity` reach the root through an
aborted search, and `IsMate` reads it as a mate. Harmless today because an
aborted depth is discarded, but it is a sentinel escaping its own guard rails.
Worth cornering when the search grows a transposition table.

## 4 — the referee called a dead position alive

**Symptom.** None yet: the rule was checked against the oracle before the test
was written, which is the only reason it was caught at all.

**Trace.** `.venv/bin/python` over eight constructed endings:

    True  bare kings                4k3/8/8/8/8/8/8/4K3 w - - 0 1
    False K+2N vs K                 4k3/8/8/8/8/8/8/2NNK3 w - - 0 1
    True  two bishops one side      4k3/8/8/8/8/8/8/1B1BK3 w - - 0 1

The first implementation counted minor pieces and only accepted "one bishop
each, same colour". It therefore called `1B1BK3` — two light-squared bishops
against a bare king — a live position. The oracle says dead, and the oracle is
right: no arrangement of two same-coloured bishops can mate.

**Root cause.** The rule was written as "few pieces left" rather than "mate is
impossible". Piece *count* is a proxy; square colour is the real constraint.

**Fix.** No pawns, rooks or queens, then: with no knights, every bishop on the
board must stand on one colour; with knights, only a single knight and no
bishops. Two knights stay live, because mate can happen even though it cannot
be forced.

**Regression case.** `TestInsufficientMaterial` — all nine cases carry the
oracle's verdict, including the one that disagreed.

## 5 — the loop that would not stop

**Symptom.** With every gate green and the completion promise printed verbatim,
the ralph loop fed the prompt back twice instead of ending.

**Trace.** Replayed the stop hook's own pipeline against the session transcript:

    grep '"role":"assistant"' "$T" | tail -n 100 \
      | jq -rs 'map(.message.content[]? | select(.type=="text") | .text) | last // ""'

Then located every text block carrying the tag:

    total text blocks: 92
      block 83 holds the tag, 8 blocks from the end
      block 87 holds the tag, 4 blocks from the end

Blocks 83 and 87 are the two final messages that carried the promise. In both
turns the shape was: preamble text -> tool calls -> final text with the promise.

**Root cause.** The hook reads the transcript when the turn stops, and the final
text block is not in the file yet at that moment. It therefore takes the
*previous* text block — the preamble written before the tool calls — which
naturally holds no promise. Not a matching bug: a flush race between the writer
and the reader.

**Fix.** None available inside the engine; the hook is fixed to the harness.
The workaround is a shape, not a retry: put the promise in the last **two**
text blocks — one before the final tool call and one after — so that a
transcript lagging by one block still contains it. Repeating a true statement
to defeat a race is fair; repeating a false one to escape a loop is not, which
is why this only applies with the board green.

**Regression case.** None possible from here. Recorded so the next reader does
not conclude the promise machinery is broken, or worse, that the gates lied.
