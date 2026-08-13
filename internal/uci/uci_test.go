package uci

import (
	"bytes"
	"io"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Frad70/catmate/internal/board"
	"github.com/Frad70/catmate/internal/movegen"
)

// syncBuffer collects engine output. The search goroutine and the command
// reader both write, so the test's own sink has to be safe too.
type syncBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *syncBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *syncBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

// run drives an engine the way a GUI does: commands go in one at a time, and
// quit waits for the bestmove of every outstanding go first. Sending quit on
// top of a running search is legal but tells you nothing — the engine is
// entitled to abandon the search, and the test would be racing it.
func run(t *testing.T, script string) string {
	t.Helper()
	reader, writer := io.Pipe()
	var out syncBuffer
	done := make(chan error, 1)
	go func() { done <- NewEngine(&out).Run(reader) }()

	pending := 0
	for _, line := range strings.Split(strings.TrimSpace(script), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if line == "quit" && pending > 0 {
			waitForBestmove(t, &out, pending)
			pending = 0
		}
		if _, err := io.WriteString(writer, line+"\n"); err != nil {
			t.Fatalf("write %q: %v", line, err)
		}
		if strings.HasPrefix(line, "go") {
			pending++
		}
	}
	writer.Close()

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("engine: %v", err)
		}
	case <-time.After(10 * time.Second):
		t.Fatalf("engine did not finish; said:\n%s", out.String())
	}
	return out.String()
}

func waitForBestmove(t *testing.T, out *syncBuffer, want int) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		if strings.Count(out.String(), "bestmove") >= want {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("waited for %d bestmove lines; engine said:\n%s", want, out.String())
}

func lastField(t *testing.T, output, prefix string) string {
	t.Helper()
	for _, line := range strings.Split(output, "\n") {
		if strings.HasPrefix(line, prefix) {
			fields := strings.Fields(line)
			return fields[len(fields)-1]
		}
	}
	t.Fatalf("no %q line in:\n%s", prefix, output)
	return ""
}

func TestHandshake(t *testing.T) {
	out := run(t, "uci\nisready\nquit\n")
	for _, want := range []string{"id name catmate", "id author", "uciok", "readyok"} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in:\n%s", want, out)
		}
	}
}

func TestSearchReturnsALegalMove(t *testing.T) {
	out := run(t, "position startpos moves e2e4 e7e5\ngo depth 4\nquit\n")
	best := lastField(t, out, "bestmove")

	pos, err := board.ParseFEN(board.StartFEN)
	if err != nil {
		t.Fatal(err)
	}
	for _, uciMove := range []string{"e2e4", "e7e5"} {
		playMove(t, pos, uciMove)
	}
	if !isLegal(pos, best) {
		t.Errorf("bestmove %s is not legal in %s\n%s", best, pos.FEN(), out)
	}
	if !strings.Contains(out, "info depth 4") {
		t.Errorf("no depth-4 info line:\n%s", out)
	}
}

func TestPositionFromFEN(t *testing.T) {
	fen := "4k3/8/8/3q4/4P3/8/8/4K3 w - - 0 1"
	out := run(t, "position fen "+fen+"\ngo depth 4\nquit\n")
	if best := lastField(t, out, "bestmove"); best != "e4d5" {
		t.Errorf("bestmove %s, want e4d5 taking the queen\n%s", best, out)
	}
}

func TestIllegalMoveIsRefused(t *testing.T) {
	out := run(t, "position startpos moves e2e5\ngo depth 1\nquit\n")
	if !strings.Contains(out, "illegal move e2e5") {
		t.Errorf("engine accepted an illegal move:\n%s", out)
	}
	// It must still be on the start position, not on some half-applied board.
	best := lastField(t, out, "bestmove")
	pos, _ := board.ParseFEN(board.StartFEN)
	if !isLegal(pos, best) {
		t.Errorf("after refusing the move, bestmove %s is not legal from the start position", best)
	}
}

func TestMateIsReportedInMoves(t *testing.T) {
	out := run(t, "position fen 6k1/5ppp/8/8/8/8/8/R6K w - - 0 1\ngo depth 3\nquit\n")
	if best := lastField(t, out, "bestmove"); best != "a1a8" {
		t.Errorf("bestmove %s, want a1a8\n%s", best, out)
	}
	if !strings.Contains(out, "score mate 1") {
		t.Errorf("mate not reported in moves:\n%s", out)
	}
}

// TestStopEndsAnInfiniteSearch is the one that matters for a real game: a
// match runner sends "go infinite" and later "stop", and the engine must
// answer with a move rather than think forever.
func TestStopEndsAnInfiniteSearch(t *testing.T) {
	reader, writer := io.Pipe()
	var out syncBuffer
	done := make(chan struct{})
	go func() {
		NewEngine(&out).Run(reader)
		close(done)
	}()

	io.WriteString(writer, "position startpos\ngo infinite\n")
	time.Sleep(150 * time.Millisecond) // let the search actually get going
	io.WriteString(writer, "stop\n")

	deadline := time.After(3 * time.Second)
	for !strings.Contains(out.String(), "bestmove") {
		select {
		case <-deadline:
			writer.Close()
			t.Fatalf("stop did not end the search; engine said:\n%s", out.String())
		default:
			time.Sleep(10 * time.Millisecond)
		}
	}
	io.WriteString(writer, "quit\n")
	writer.Close()
	<-done
}

// TestStopBeforeTheSearchStarts covers the race the other test cannot: the
// command arrives while the search goroutine is still being scheduled. If the
// stop flag is cleared inside the search, that stop is lost and an infinite
// search never ends.
func TestStopBeforeTheSearchStarts(t *testing.T) {
	outCh := make(chan string, 1)
	go func() { outCh <- run(t, "position startpos\ngo infinite\nstop\nquit\n") }()

	select {
	case out := <-outCh:
		if !strings.Contains(out, "bestmove") {
			t.Errorf("no bestmove after stop:\n%s", out)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("an immediate stop did not end the search")
	}
}

func TestGoWithClockPicksABudget(t *testing.T) {
	start := time.Now()
	out := run(t, "position startpos\ngo wtime 3000 btime 3000 winc 0 binc 0\nquit\n")
	elapsed := time.Since(start)

	if elapsed > 2*time.Second {
		t.Errorf("spent %s of a 3s clock on one move", elapsed.Round(time.Millisecond))
	}
	if best := lastField(t, out, "bestmove"); best == "0000" {
		t.Errorf("no move returned:\n%s", out)
	}
}

func playMove(t *testing.T, pos *board.Position, uciMove string) {
	t.Helper()
	var list movegen.MoveList
	movegen.GenerateLegal(pos, &list)
	for _, m := range list.Slice() {
		if m.String() == uciMove {
			movegen.Make(pos, m)
			return
		}
	}
	t.Fatalf("%s is not legal in %s", uciMove, pos.FEN())
}

func isLegal(pos *board.Position, uciMove string) bool {
	var list movegen.MoveList
	movegen.GenerateLegal(pos, &list)
	for _, m := range list.Slice() {
		if m.String() == uciMove {
			return true
		}
	}
	return false
}
