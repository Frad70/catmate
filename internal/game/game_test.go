package game

import (
	"testing"

	"github.com/Frad70/catmate/internal/board"
	"github.com/Frad70/catmate/internal/movegen"
)

func parse(t *testing.T, fen string) *board.Position {
	t.Helper()
	pos, err := board.ParseFEN(fen)
	if err != nil {
		t.Fatalf("parse %q: %v", fen, err)
	}
	return pos
}

// Every expectation below was checked against python-chess before it was
// written down. One of them — two same-coloured bishops against a bare king —
// disagreed with the first implementation, which is why the rule is what it is.
func TestInsufficientMaterial(t *testing.T) {
	cases := []struct {
		fen  string
		want bool
		what string
	}{
		{"4k3/8/8/8/8/8/8/4K3 w - - 0 1", true, "bare kings"},
		{"4k3/8/8/8/8/8/8/3BK3 w - - 0 1", true, "king and bishop against a king"},
		{"4k3/8/8/8/8/8/8/3NK3 w - - 0 1", true, "king and knight against a king"},
		{"4k3/8/8/8/8/8/8/2NNK3 w - - 0 1", false, "two knights can mate, though not by force"},
		{"3bk3/8/8/8/8/8/8/2B1K3 w - - 0 1", true, "bishops on squares of one colour"},
		{"2b1k3/8/8/8/8/8/8/2B1K3 w - - 0 1", false, "bishops on opposite colours can mate"},
		{"4k3/8/8/8/8/8/8/1B1BK3 w - - 0 1", true, "two bishops, both light, cannot mate"},
		{"4k3/8/8/8/8/8/4P3/4K3 w - - 0 1", false, "a pawn can promote"},
		{"4k3/8/8/8/8/8/8/R3K3 w - - 0 1", false, "a rook mates"},
	}
	for _, c := range cases {
		if got := InsufficientMaterial(parse(t, c.fen)); got != c.want {
			t.Errorf("%s: got %v, want %v (%s)", c.what, got, c.want, c.fen)
		}
	}
}

func TestAdjudicateEndings(t *testing.T) {
	cases := []struct {
		fen     string
		outcome Outcome
		reason  string
	}{
		{"6k1/5ppp/8/8/8/8/8/R5K1 w - - 0 1", Ongoing, ""},
		{"R5k1/5ppp/8/8/8/8/8/6K1 b - - 0 1", WhiteWins, "checkmate"},
		{"7k/5Q2/6K1/8/8/8/8/8 b - - 0 1", Draw, "stalemate"},
		{"4k3/8/8/8/8/8/8/4K3 w - - 0 1", Draw, "insufficient material"},
		{"6k1/5ppp/8/8/8/8/5PPP/6K1 w - - 100 80", Draw, "fifty-move rule"},
	}
	for _, c := range cases {
		got := Adjudicate(parse(t, c.fen), map[uint64]int{})
		if got.Outcome != c.outcome || got.Reason != c.reason {
			t.Errorf("%s: got %v %q, want %v %q", c.fen, got.Outcome, got.Reason, c.outcome, c.reason)
		}
	}
}

// TestCheckmateGoesToTheOtherSide guards the sign: the side to move is the one
// that is mated, so the point goes to its opponent. Getting this backwards
// would invert every result in a match.
func TestCheckmateGoesToTheOtherSide(t *testing.T) {
	// Black to move and mated by the rook on a8.
	if got := Adjudicate(parse(t, "R5k1/5ppp/8/8/8/8/8/6K1 b - - 0 1"), nil); got.Outcome != WhiteWins {
		t.Errorf("black is mated but the result was %v", got.Outcome)
	}
	// White to move and mated by the rook on a1.
	if got := Adjudicate(parse(t, "6k1/8/8/8/8/8/5PPP/r5K1 w - - 0 1"), nil); got.Outcome != BlackWins {
		t.Errorf("white is mated but the result was %v", got.Outcome)
	}
}

// TestThreefoldNeedsThree walks a real shuffle: the position must be seen three
// times, not two, and the count is over position keys rather than move lists.
func TestThreefoldNeedsThree(t *testing.T) {
	pos := parse(t, "4k3/8/8/8/8/8/8/R3K3 w - - 0 1")
	repetitions := map[uint64]int{pos.Key(): 1}

	if got := Adjudicate(pos, repetitions); got.Outcome != Ongoing {
		t.Fatalf("fresh position already over: %v", got)
	}

	// Ra1-a2-a1-a2-a1 with the black king shuffling back and forth.
	for _, uciMove := range []string{"a1a2", "e8d8", "a2a1", "d8e8", "a1a2", "e8d8", "a2a1", "d8e8"} {
		var list movegen.MoveList
		movegen.GenerateLegal(pos, &list)
		found := false
		for _, m := range list.Slice() {
			if m.String() == uciMove {
				movegen.Make(pos, m)
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("%s is not legal in %s", uciMove, pos.FEN())
		}
		repetitions[pos.Key()]++
	}

	if got := Adjudicate(pos, repetitions); got.Outcome != Draw || got.Reason != "threefold repetition" {
		t.Errorf("after two full cycles: got %v %q, want a threefold draw (counts %d)",
			got.Outcome, got.Reason, repetitions[pos.Key()])
	}
}
