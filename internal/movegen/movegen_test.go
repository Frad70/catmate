package movegen

import (
	"path/filepath"
	"testing"

	"github.com/Frad70/catmate/internal/board"
	"github.com/Frad70/catmate/internal/epd"
)

const corpus = "../../testdata/perft.epd"

func generate(t *testing.T, fen string) *MoveList {
	t.Helper()
	pos, err := board.ParseFEN(fen)
	if err != nil {
		t.Fatalf("parse %q: %v", fen, err)
	}
	var list MoveList
	GeneratePseudoLegal(pos, &list)
	return &list
}

// TestPseudoLegalCoversLegal is the strongest claim available before make/unmake
// exists: every legal move is pseudo-legal, so the generated count can never be
// below the oracle's depth-1 count. If it is, the generator is blind to a move
// that genuinely exists — and perft would fail for the same reason later.
func TestPseudoLegalCoversLegal(t *testing.T) {
	entries, err := epd.Load(filepath.Clean(corpus))
	if err != nil {
		t.Fatalf("load corpus: %v", err)
	}
	worstSlack, worstFEN := 0, ""
	for _, e := range entries {
		legal, ok := e.Nodes[1]
		if !ok {
			continue
		}
		got := generate(t, e.FEN).Len()
		if uint64(got) < legal {
			t.Errorf("line %d: %d pseudo-legal moves but %d legal ones exist\n  %s",
				e.Line, got, legal, e.FEN)
			continue
		}
		if slack := got - int(legal); slack > worstSlack {
			worstSlack, worstFEN = slack, e.FEN
		}
	}
	t.Logf("checked %d positions; largest pseudo-legal surplus %d at %q",
		len(entries), worstSlack, worstFEN)
}

func TestStartPositionHasTwentyMoves(t *testing.T) {
	list := generate(t, board.StartFEN)
	if list.Len() != 20 {
		t.Errorf("start position: %d pseudo-legal moves, want 20", list.Len())
	}
	seen := map[string]bool{}
	for _, m := range list.Slice() {
		if seen[m.String()] {
			t.Errorf("duplicate move %s", m)
		}
		seen[m.String()] = true
	}
	for _, want := range []string{"e2e4", "e2e3", "g1f3", "b1a3", "h2h4"} {
		if !seen[want] {
			t.Errorf("missing %s from the start position", want)
		}
	}
	if seen["e1g1"] || seen["d1d2"] {
		t.Error("generated a move through occupied squares")
	}
}

func TestJumperCountsOnEmptyBoard(t *testing.T) {
	cases := []struct {
		fen  string
		want int
		what string
	}{
		{"8/8/8/8/8/8/8/N6k w - - 0 1", 2, "knight in the corner"},
		{"7k/8/8/3N4/8/8/8/K7 w - - 0 1", 8 + 3, "knight in the centre plus a king in the corner"},
		{"7k/8/8/8/8/8/8/K7 w - - 0 1", 3, "king in the corner"},
		{"7k/8/8/3K4/8/8/8/8 w - - 0 1", 8, "king in the centre"},
		{"7k/8/8/3R4/8/8/8/K7 w - - 0 1", 14 + 3, "rook on an open board"},
		{"7k/8/8/3B4/8/8/8/K7 w - - 0 1", 13 + 3, "bishop on d5"},
		{"7k/8/8/3Q4/8/8/8/K7 w - - 0 1", 27 + 3, "queen on d5"},
	}
	for _, c := range cases {
		if got := generate(t, c.fen).Len(); got != c.want {
			t.Errorf("%s: %d moves, want %d (%s)", c.what, got, c.want, c.fen)
		}
	}
}

func TestPromotionsGenerateFourPieces(t *testing.T) {
	list := generate(t, "7k/P7/8/8/8/8/8/K7 w - - 0 1")
	promos := map[string]bool{}
	for _, m := range list.Slice() {
		if m.IsPromotion() {
			promos[m.String()] = true
			if m.IsCapture() {
				t.Errorf("%s flagged as a capture on an empty square", m)
			}
		}
	}
	for _, want := range []string{"a7a8q", "a7a8r", "a7a8b", "a7a8n"} {
		if !promos[want] {
			t.Errorf("missing promotion %s, got %v", want, promos)
		}
	}
	if len(promos) != 4 {
		t.Errorf("got %d promotions, want 4: %v", len(promos), promos)
	}
}

func TestPromotionCapturesAreFlagged(t *testing.T) {
	list := generate(t, "1r5k/P7/8/8/8/8/8/K7 w - - 0 1")
	var captures int
	for _, m := range list.Slice() {
		if m.String() == "a7b8q" {
			captures++
			if !m.IsCapture() || !m.IsPromotion() {
				t.Errorf("a7b8q: capture=%v promotion=%v, want both", m.IsCapture(), m.IsPromotion())
			}
		}
	}
	if captures != 1 {
		t.Errorf("found %d a7b8q moves, want exactly 1", captures)
	}
}

func TestEnPassantIsGeneratedAndFlagged(t *testing.T) {
	// Black has just played d7d5; white's e5 pawn may take it in passing.
	list := generate(t, "rnbqkbnr/ppp1pppp/8/3pP3/8/8/PPPP1PPP/RNBQKBNR w KQkq d6 0 3")
	found := false
	for _, m := range list.Slice() {
		if m.String() == "e5d6" {
			found = true
			if !m.IsEnPassant() || !m.IsCapture() {
				t.Errorf("e5d6: enPassant=%v capture=%v, want both", m.IsEnPassant(), m.IsCapture())
			}
		}
	}
	if !found {
		t.Error("en passant capture e5d6 was not generated")
	}
}

func TestCastlesNeedRightsAndAClearPath(t *testing.T) {
	kiwipete := "r3k2r/p1ppqpb1/bn2pnp1/3PN3/1p2P3/2N2Q1p/PPPBBPPP/R3K2R w KQkq - 0 1"
	seen := map[string]bool{}
	for _, m := range generate(t, kiwipete).Slice() {
		seen[m.String()] = m.IsCastle() || seen[m.String()]
	}
	if !seen["e1g1"] || !seen["e1c1"] {
		t.Errorf("kiwipete: expected both castles, got %v", seen)
	}

	blocked := generate(t, "r3k2r/8/8/8/8/8/8/RN2K1nR w KQkq - 0 1")
	for _, m := range blocked.Slice() {
		if m.IsCastle() {
			t.Errorf("castled through an occupied square: %s", m)
		}
	}

	norights := generate(t, "r3k2r/8/8/8/8/8/8/R3K2R w - - 0 1")
	for _, m := range norights.Slice() {
		if m.IsCastle() {
			t.Errorf("castled without the right: %s", m)
		}
	}
}

func TestAttackedSeesEveryAttacker(t *testing.T) {
	cases := []struct {
		fen  string
		sq   string
		by   board.Color
		want bool
		what string
	}{
		{"7k/8/8/8/8/5n2/8/4K3 b - - 0 1", "e1", board.Black, true, "knight checks the king"},
		{"7k/8/8/8/8/8/4p3/4K3 b - - 0 1", "d1", board.Black, true, "pawn attacks diagonally"},
		{"7k/8/8/8/8/8/4p3/4K3 b - - 0 1", "e1", board.Black, false, "pawn does not attack forward"},
		{"7k/8/8/8/8/8/8/r3K3 b - - 0 1", "e1", board.Black, true, "rook along the rank"},
		{"7k/8/8/8/8/8/8/r1B1K3 b - - 0 1", "e1", board.Black, false, "rook blocked by a bishop"},
		{"7k/8/8/8/1b6/8/8/4K3 b - - 0 1", "e1", board.Black, true, "bishop along the diagonal"},
	}
	for _, c := range cases {
		pos, err := board.ParseFEN(c.fen)
		if err != nil {
			t.Fatalf("parse %q: %v", c.fen, err)
		}
		sq, err := board.ParseSquare(c.sq)
		if err != nil {
			t.Fatal(err)
		}
		if got := Attacked(pos, sq, c.by); got != c.want {
			t.Errorf("%s: Attacked(%s) = %v, want %v (%s)", c.what, c.sq, got, c.want, c.fen)
		}
	}
}

func TestAttackTablesAreSymmetric(t *testing.T) {
	for a := board.Square(0); a < 64; a++ {
		for b := board.Square(0); b < 64; b++ {
			if knightAttacks[a].Has(b) != knightAttacks[b].Has(a) {
				t.Fatalf("knight attacks asymmetric between %s and %s", a, b)
			}
			if kingAttacks[a].Has(b) != kingAttacks[b].Has(a) {
				t.Fatalf("king attacks asymmetric between %s and %s", a, b)
			}
		}
	}
	// A knight on h1 must not wrap around the edge onto the a-file.
	h1, _ := board.ParseSquare("h1")
	a2, _ := board.ParseSquare("a2")
	if knightAttacks[h1].Has(a2) {
		t.Error("knight on h1 wrapped around the board")
	}
	if knightAttacks[h1].Count() != 2 {
		t.Errorf("knight on h1 attacks %d squares, want 2", knightAttacks[h1].Count())
	}
}

func TestMoveEncodingRoundTrips(t *testing.T) {
	flags := []Flag{
		FlagQuiet, FlagDoublePush, FlagKingCastle, FlagQueenCastle, FlagCapture,
		FlagEnPassant, FlagPromoKnight, FlagPromoQueen, FlagPromoCaptureRook,
		FlagPromoCaptureQueen,
	}
	for from := board.Square(0); from < 64; from++ {
		for _, to := range []board.Square{0, 7, 28, 63, from} {
			for _, f := range flags {
				m := New(from, to, f)
				if m.From() != from || m.To() != to || m.Flag() != f {
					t.Fatalf("New(%v,%v,%v) decoded as (%v,%v,%v)",
						from, to, f, m.From(), m.To(), m.Flag())
				}
			}
		}
	}
	if got := New(12, 28, FlagDoublePush).String(); got != "e2e4" {
		t.Errorf("String() = %q, want e2e4", got)
	}
	if got := New(52, 60, FlagPromoQueen).String(); got != "e7e8q" {
		t.Errorf("String() = %q, want e7e8q", got)
	}
}
