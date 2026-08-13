package board

import (
	"math/rand"
	"path/filepath"
	"testing"

	"github.com/Frad70/catmate/internal/epd"
)

// TestZobristIncrementalMatchesRecompute is the heart of G1: the key carried
// along by every mutation must equal the key computed from the board as it
// stands. Move generation does not exist yet, so the walk is driven by random
// edits — put, remove, relocate, flip side, rewrite castling rights, set and
// clear the en passant square — which exercise the same mutators make/unmake
// will use.
func TestZobristIncrementalMatchesRecompute(t *testing.T) {
	entries, err := epd.Load(filepath.Clean(corpus))
	if err != nil {
		t.Fatalf("load corpus: %v", err)
	}

	rng := rand.New(rand.NewSource(20260813))
	const plies = 10000
	done := 0

	for done < plies {
		e := entries[rng.Intn(len(entries))]
		pos, err := ParseFEN(e.FEN)
		if err != nil {
			t.Fatalf("line %d: parse %q: %v", e.Line, e.FEN, err)
		}
		if got, want := pos.Key(), pos.ComputeKey(); got != want {
			t.Fatalf("line %d: key after parse = %016x, recompute = %016x", e.Line, got, want)
		}

		for i := 0; i < 40 && done < plies; i++ {
			what := mutate(pos, rng)
			done++
			if got, want := pos.Key(), pos.ComputeKey(); got != want {
				t.Fatalf("ply %d: after %s on %q\n  incremental %016x\n  recomputed  %016x",
					done, what, e.FEN, got, want)
			}
		}
	}
	t.Logf("incremental key survived %d random mutations", done)
}

// mutate applies one random change and reports what it did, so a failure names
// the mutation that broke the key.
func mutate(p *Position, rng *rand.Rand) string {
	switch rng.Intn(6) {
	case 0: // relocate an occupied square
		from := occupiedSquare(p, rng)
		if from == SquareNone {
			return "relocate (no pieces)"
		}
		to := Square(rng.Intn(64))
		pc := p.PieceAt(from)
		p.Remove(from)
		p.Put(to, pc)
		return "relocate " + from.String() + "->" + to.String()
	case 1: // clear a square
		sq := occupiedSquare(p, rng)
		if sq == SquareNone {
			return "remove (no pieces)"
		}
		p.Remove(sq)
		return "remove " + sq.String()
	case 2: // drop a piece, possibly onto an occupied square
		sq := Square(rng.Intn(64))
		pc := Piece(rng.Intn(int(NoPiece)))
		p.Put(sq, pc)
		return "put " + string(pc.Char()) + " on " + sq.String()
	case 3:
		p.SetSide(p.Side().Opposite())
		return "flip side"
	case 4:
		p.SetCastle(CastleRights(rng.Intn(16)))
		return "set castling"
	default:
		if rng.Intn(2) == 0 {
			p.SetEP(SquareNone)
			return "clear ep"
		}
		p.SetEP(Square(rng.Intn(64)))
		return "set ep"
	}
}

func occupiedSquare(p *Position, rng *rand.Rand) Square {
	occ := p.OccupiedAll()
	if occ == 0 {
		return SquareNone
	}
	for {
		sq := Square(rng.Intn(64))
		if occ.Has(sq) {
			return sq
		}
	}
}

// TestZobristUndoRestoresKey pins the xor property the incremental scheme rests
// on: any mutation, reversed, must bring the key back exactly.
func TestZobristUndoRestoresKey(t *testing.T) {
	pos, err := ParseFEN(StartFEN)
	if err != nil {
		t.Fatalf("parse start position: %v", err)
	}
	before := pos.Key()

	pc := pos.PieceAt(12) // e2 pawn
	pos.Remove(12)
	pos.Put(28, pc) // e4
	if pos.Key() == before {
		t.Fatal("moving a pawn did not change the key")
	}
	pos.Remove(28)
	pos.Put(12, pc)
	if got := pos.Key(); got != before {
		t.Errorf("key after undo = %016x, want %016x", got, before)
	}

	pos.SetSide(Black)
	pos.SetSide(White)
	pos.SetCastle(0)
	pos.SetCastle(WhiteKingside | WhiteQueenside | BlackKingside | BlackQueenside)
	pos.SetEP(20)
	pos.SetEP(SquareNone)
	if got := pos.Key(); got != before {
		t.Errorf("key after state round trip = %016x, want %016x", got, before)
	}
}

// TestZobristDistinguishesPositions is a collision smoke test over the corpus:
// distinct positions should hash distinctly. A hit here is either a broken key
// table or a genuinely astronomical coincidence.
func TestZobristDistinguishesPositions(t *testing.T) {
	entries, err := epd.Load(filepath.Clean(corpus))
	if err != nil {
		t.Fatalf("load corpus: %v", err)
	}
	seen := make(map[uint64]string, len(entries))
	for _, e := range entries {
		pos, err := ParseFEN(e.FEN)
		if err != nil {
			t.Fatalf("line %d: %v", e.Line, err)
		}
		if prev, dup := seen[pos.Key()]; dup && prev != e.FEN {
			t.Errorf("key collision:\n  %q\n  %q", prev, e.FEN)
		}
		seen[pos.Key()] = e.FEN
	}

	// Side to move is part of the position, so it must be part of the key.
	white, _ := ParseFEN("rnbqkbnr/pppppppp/8/8/8/8/PPPPPPPP/RNBQKBNR w KQkq - 0 1")
	black, _ := ParseFEN("rnbqkbnr/pppppppp/8/8/8/8/PPPPPPPP/RNBQKBNR b KQkq - 0 1")
	if white.Key() == black.Key() {
		t.Error("side to move does not affect the key")
	}

	// Clocks are not part of the position for repetition purposes.
	a, _ := ParseFEN("rnbqkbnr/pppppppp/8/8/8/8/PPPPPPPP/RNBQKBNR w KQkq - 0 1")
	b, _ := ParseFEN("rnbqkbnr/pppppppp/8/8/8/8/PPPPPPPP/RNBQKBNR w KQkq - 7 42")
	if a.Key() != b.Key() {
		t.Error("halfmove and fullmove clocks leaked into the key")
	}
}
