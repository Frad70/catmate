package search

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/Frad70/catmate/internal/board"
	"github.com/Frad70/catmate/internal/epd"
	"github.com/Frad70/catmate/internal/eval"
	"github.com/Frad70/catmate/internal/movegen"
)

const corpus = "../../testdata/perft.epd"

// plainNegamax is the reference: the same tree, no pruning at all. Whatever it
// returns is the true minimax value of the position at that depth.
func plainNegamax(p *board.Position, depth, ply int) int {
	if depth <= 0 {
		return eval.Evaluate(p)
	}
	var list movegen.MoveList
	movegen.GenerateLegal(p, &list)
	if list.Len() == 0 {
		if movegen.InCheck(p) {
			return -MateScore + ply
		}
		return 0
	}
	best := -Infinity
	for _, m := range list.Slice() {
		undo := movegen.Make(p, m)
		score := -plainNegamax(p, depth-1, ply+1)
		movegen.Unmake(p, m, undo)
		if score > best {
			best = score
		}
	}
	return best
}

// TestAlphaBetaMatchesPlainNegamax is the correctness check that matters for a
// search: cutting branches is allowed to save time, never to change the answer.
// Leaves are static here, because comparing against unpruned negamax only means
// something when the leaf value is a fixed function.
func TestAlphaBetaMatchesPlainNegamax(t *testing.T) {
	entries, err := epd.Load(filepath.Clean(corpus))
	if err != nil {
		t.Fatalf("load corpus: %v", err)
	}

	const depth = 3
	checked := 0
	for i, e := range entries {
		if i%40 != 0 { // every fortieth position keeps the test honest and quick
			continue
		}
		pos, err := board.ParseFEN(e.FEN)
		if err != nil {
			t.Fatalf("line %d: %v", e.Line, err)
		}
		want := plainNegamax(pos, depth, 0)

		s := New()
		s.staticLeaves = true
		got := s.alphaBeta(pos, depth, -Infinity, Infinity, 0)

		if got != want {
			t.Errorf("line %d: alpha-beta says %d, unpruned negamax says %d\n  %s",
				e.Line, got, want, e.FEN)
		}
		checked++
	}
	t.Logf("alpha-beta agreed with unpruned negamax on %d positions at depth %d", checked, depth)
}

// TestAlphaBetaActuallyPrunes guards the other side of the same coin: a search
// that agrees with negamax because it *is* negamax has not pruned anything.
func TestAlphaBetaActuallyPrunes(t *testing.T) {
	pos, err := board.ParseFEN(board.StartFEN)
	if err != nil {
		t.Fatal(err)
	}
	s := New()
	s.staticLeaves = true
	s.alphaBeta(pos, 4, -Infinity, Infinity, 0)

	full := countNodes(pos, 4)
	if s.nodes >= full {
		t.Errorf("alpha-beta visited %d nodes, unpruned tree has %d — nothing was cut", s.nodes, full)
	}
	t.Logf("depth 4 from the start position: %d nodes with alpha-beta, %d without", s.nodes, full)
}

func countNodes(p *board.Position, depth int) uint64 {
	if depth <= 0 {
		return 1
	}
	var list movegen.MoveList
	movegen.GenerateLegal(p, &list)
	if list.Len() == 0 {
		return 1
	}
	var n uint64 = 1
	for _, m := range list.Slice() {
		undo := movegen.Make(p, m)
		n += countNodes(p, depth-1)
		movegen.Unmake(p, m, undo)
	}
	return n
}

// The mating moves below were confirmed with the oracle to be legal, mate, and
// the only mate in the position.
func TestFindsMateInOne(t *testing.T) {
	cases := []struct {
		fen  string
		move string
	}{
		{"6k1/5ppp/8/8/8/8/8/R6K w - - 0 1", "a1a8"},
		{"7k/8/5K2/8/8/8/8/6Q1 w - - 0 1", "g1g7"},
	}
	for _, c := range cases {
		pos, err := board.ParseFEN(c.fen)
		if err != nil {
			t.Fatal(err)
		}
		res := New().Search(pos, Limits{Depth: 3})
		if res.Best.String() != c.move {
			t.Errorf("%s: played %s, want %s (score %d)", c.fen, res.Best, c.move, res.Score)
		}
		mate, plies := IsMate(res.Score)
		if !mate || plies != 1 {
			t.Errorf("%s: score %d, want mate in 1 ply", c.fen, res.Score)
		}
	}
}

func TestTakesTheHangingQueen(t *testing.T) {
	pos, err := board.ParseFEN("4k3/8/8/3q4/4P3/8/8/4K3 w - - 0 1")
	if err != nil {
		t.Fatal(err)
	}
	res := New().Search(pos, Limits{Depth: 4})
	if res.Best.String() != "e4d5" {
		t.Errorf("played %s, want e4d5 taking the queen (score %d)", res.Best, res.Score)
	}
}

// TestQuiescenceStopsMidExchange is what the quiescence search exists for: at a
// fixed depth the last ply may be a capture that hangs the capturing piece, and
// a static evaluation there reads as winning material when it is losing it.
func TestQuiescenceStopsMidExchange(t *testing.T) {
	// White queen can take a pawn on d5, but the pawn is defended by c6.
	// Taking loses a queen for a pawn; the engine must not do it.
	pos, err := board.ParseFEN("4k3/8/2p5/3p4/8/8/8/3QK3 w - - 0 1")
	if err != nil {
		t.Fatal(err)
	}
	res := New().Search(pos, Limits{Depth: 2})
	if res.Best.String() == "d1d5" {
		t.Errorf("queen grabbed a defended pawn: %s (score %d)", res.Best, res.Score)
	}
}

func TestStalemateScoresZero(t *testing.T) {
	// Confirmed stalemate with the oracle: black to move, no legal moves, no check.
	pos, err := board.ParseFEN("7k/5Q2/6K1/8/8/8/8/8 b - - 0 1")
	if err != nil {
		t.Fatal(err)
	}
	s := New()
	if got := s.alphaBeta(pos, 3, -Infinity, Infinity, 0); got != 0 {
		t.Errorf("stalemate scored %d, want 0", got)
	}
}

func TestRespectsMoveTime(t *testing.T) {
	pos, err := board.ParseFEN(board.StartFEN)
	if err != nil {
		t.Fatal(err)
	}
	const budget = 100 * time.Millisecond
	start := time.Now()
	res := New().Search(pos, Limits{MoveTime: budget})
	elapsed := time.Since(start)

	if elapsed > 2*budget {
		t.Errorf("search took %s on a %s budget", elapsed.Round(time.Millisecond), budget)
	}
	if res.Best == 0 {
		t.Error("search returned no move")
	}
	t.Logf("reached depth %d in %s (%d nodes)", res.Depth, elapsed.Round(time.Millisecond), res.Nodes)
}

func TestPrincipalVariationIsPlayable(t *testing.T) {
	pos, err := board.ParseFEN(board.StartFEN)
	if err != nil {
		t.Fatal(err)
	}
	res := New().Search(pos, Limits{Depth: 5})
	if len(res.PV) == 0 {
		t.Fatal("no principal variation")
	}
	// Every move of the PV must be legal in turn, or the line is fiction.
	for i, m := range res.PV {
		var list movegen.MoveList
		movegen.GenerateLegal(pos, &list)
		legal := false
		for _, c := range list.Slice() {
			if c == m {
				legal = true
				break
			}
		}
		if !legal {
			t.Fatalf("PV move %d (%s) is not legal in %s", i+1, m, pos.FEN())
		}
		movegen.Make(pos, m)
	}
	t.Logf("PV: %v", res.PV)
}

// TestAvoidsRepetitionWhenWinning is the whole point of feeding history into
// the search: a rook up, the engine must not step back into a position the
// game has already seen, because that line is worth nothing.
func TestAvoidsRepetitionWhenWinning(t *testing.T) {
	const fen = "4k3/8/8/8/8/8/8/R3K3 w - - 0 1"
	root, err := board.ParseFEN(fen)
	if err != nil {
		t.Fatal(err)
	}

	// Work out the key that Ra1-a2 leads to, and claim the game has been there.
	probe, _ := board.ParseFEN(fen)
	var list movegen.MoveList
	movegen.GenerateLegal(probe, &list)
	var repeat movegen.Move
	for _, m := range list.Slice() {
		if m.String() == "a1a2" {
			repeat = m
		}
	}
	if repeat == 0 {
		t.Fatal("a1a2 is not legal here")
	}
	movegen.Make(probe, repeat)
	seen := probe.Key()

	res := New().Search(root, Limits{Depth: 5, History: []uint64{root.Key(), seen}})
	if res.Best.String() == "a1a2" {
		t.Errorf("repeated a seen position while a rook up (score %d)", res.Score)
	}
	if res.Score <= 0 {
		t.Errorf("score %d while a rook up; the engine should not think this is drawn", res.Score)
	}
}

// TestFiftyMoveCounterEndsTheLine checks the other half: with the clock at 99,
// every reply reaches 100 and the position is drawn however much material is on
// the board.
func TestFiftyMoveCounterEndsTheLine(t *testing.T) {
	pos, err := board.ParseFEN("4k3/8/8/8/8/8/8/R3K3 w - - 99 60")
	if err != nil {
		t.Fatal(err)
	}
	res := New().Search(pos, Limits{Depth: 3})
	if res.Score != 0 {
		t.Errorf("score %d, want 0: every move here hits the fifty-move rule", res.Score)
	}
}
