package board

import (
	"path/filepath"
	"testing"

	"github.com/Frad70/catmate/internal/epd"
)

const corpus = "../../testdata/perft.epd"

// TestFENRoundTripCorpus is the real test: every position the oracle ever
// produced must survive parse -> serialize unchanged. The corpus is full of
// awkward cases (en passant squares, half-stripped castling rights, promotion
// tangles), which is exactly why it is worth round-tripping all of it.
func TestFENRoundTripCorpus(t *testing.T) {
	entries, err := epd.Load(filepath.Clean(corpus))
	if err != nil {
		t.Fatalf("load corpus: %v", err)
	}
	if len(entries) < 100 {
		t.Fatalf("corpus holds only %d positions; something truncated it", len(entries))
	}

	for _, e := range entries {
		pos, err := ParseFEN(e.FEN)
		if err != nil {
			t.Errorf("line %d: parse %q: %v", e.Line, e.FEN, err)
			continue
		}
		if got := pos.FEN(); got != e.FEN {
			t.Errorf("line %d: round trip\n  want %q\n  got  %q", e.Line, e.FEN, got)
		}
	}
	t.Logf("round-tripped %d positions", len(entries))
}

func TestFENStartPosition(t *testing.T) {
	pos, err := ParseFEN(StartFEN)
	if err != nil {
		t.Fatalf("parse start position: %v", err)
	}
	if pos.Side != White {
		t.Errorf("side to move = %v, want White", pos.Side)
	}
	if pos.Castle != WhiteKingside|WhiteQueenside|BlackKingside|BlackQueenside {
		t.Errorf("castling rights = %04b, want all four", pos.Castle)
	}
	if pos.EP != SquareNone {
		t.Errorf("en passant = %v, want none", pos.EP)
	}
	if pos.Fullmove != 1 || pos.Halfmove != 0 {
		t.Errorf("clocks = %d/%d, want 0/1", pos.Halfmove, pos.Fullmove)
	}
	if got := pos.PieceAt(0); got != MakePiece(White, Rook) {
		t.Errorf("a1 holds %c, want white rook", got.Char())
	}
	if got := pos.PieceAt(60); got != MakePiece(Black, King) {
		t.Errorf("e8 holds %c, want black king", got.Char())
	}
	if got := pos.OccupiedAll(); got != 0xFFFF00000000FFFF {
		t.Errorf("occupancy = %016x, want 0xffff00000000ffff", uint64(got))
	}
}

func TestSquareNaming(t *testing.T) {
	cases := []struct {
		sq   Square
		name string
	}{
		{0, "a1"}, {7, "h1"}, {56, "a8"}, {63, "h8"}, {28, "e4"},
	}
	for _, c := range cases {
		if got := c.sq.String(); got != c.name {
			t.Errorf("Square(%d) = %q, want %q", c.sq, got, c.name)
		}
		parsed, err := ParseSquare(c.name)
		if err != nil {
			t.Errorf("ParseSquare(%q): %v", c.name, err)
			continue
		}
		if parsed != c.sq {
			t.Errorf("ParseSquare(%q) = %d, want %d", c.name, parsed, c.sq)
		}
	}
}

func TestFENRejectsGarbage(t *testing.T) {
	bad := []string{
		"",
		"8/8/8/8/8/8/8 w - - 0 1",
		"rnbqkbnr/pppppppp/8/8/8/8/PPPPPPPP/RNBQKBNR x KQkq - 0 1",
		"rnbqkbnr/pppppppp/8/8/8/8/PPPPPPPP/RNBQKBNR w KQkX - 0 1",
		"rnbqkbnr/pppppppp/8/8/8/8/PPPPPPPP/RNBQKBNR w KQkq z9 0 1",
		"9/8/8/8/8/8/8/8 w - - 0 1",
	}
	for _, fen := range bad {
		if _, err := ParseFEN(fen); err == nil {
			t.Errorf("ParseFEN(%q) accepted garbage", fen)
		}
	}
}
