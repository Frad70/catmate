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
