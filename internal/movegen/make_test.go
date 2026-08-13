package movegen

import (
	"path/filepath"
	"testing"

	"github.com/Frad70/catmate/internal/board"
	"github.com/Frad70/catmate/internal/epd"
)

// TestLegalMoveCountMatchesOracle is the first real perft: at depth 1 the node
// count *is* the number of legal moves, and the oracle already knows it for
// every position in the corpus. Nothing here can be argued with.
func TestLegalMoveCountMatchesOracle(t *testing.T) {
	entries, err := epd.Load(filepath.Clean(corpus))
	if err != nil {
		t.Fatalf("load corpus: %v", err)
	}
	bad := 0
	for _, e := range entries {
		want, ok := e.Nodes[1]
		if !ok {
			continue
		}
		pos, err := board.ParseFEN(e.FEN)
		if err != nil {
			t.Fatalf("line %d: %v", e.Line, err)
		}
		var list MoveList
		GenerateLegal(pos, &list)
		if uint64(list.Len()) != want {
			bad++
			if bad <= 5 {
				t.Errorf("line %d: %d legal moves, oracle says %d\n  %s\n  generated: %v",
					e.Line, list.Len(), want, e.FEN, list.Slice())
			}
		}
	}
	if bad > 0 {
		t.Fatalf("%d of %d positions disagree with the oracle at depth 1", bad, len(entries))
	}
	t.Logf("depth-1 node counts match the oracle on all %d positions", len(entries))
}

// TestMakeUnmakeRestoresPosition walks every legal move of every corpus
// position and demands the position come back byte-identical, key included.
// The FEN is the total-state check: pieces, side, rights, en passant and both
// clocks all live in it.
func TestMakeUnmakeRestoresPosition(t *testing.T) {
	entries, err := epd.Load(filepath.Clean(corpus))
	if err != nil {
		t.Fatalf("load corpus: %v", err)
	}
	moves := 0
	for _, e := range entries {
		pos, err := board.ParseFEN(e.FEN)
		if err != nil {
			t.Fatalf("line %d: %v", e.Line, err)
		}
		var list MoveList
		GenerateLegal(pos, &list)

		for _, m := range list.Slice() {
			beforeFEN, beforeKey := pos.FEN(), pos.Key()

			undo := Make(pos, m)
			if got, want := pos.Key(), pos.ComputeKey(); got != want {
				t.Fatalf("line %d: after %s the key drifted\n  from %s\n  incremental %016x\n  recomputed  %016x",
					e.Line, m, beforeFEN, got, want)
			}

			Unmake(pos, m, undo)
			if got := pos.FEN(); got != beforeFEN {
				t.Fatalf("line %d: %s did not undo cleanly\n  want %s\n  got  %s", e.Line, m, beforeFEN, got)
			}
			if pos.Key() != beforeKey {
				t.Fatalf("line %d: %s left the key at %016x, want %016x", e.Line, m, pos.Key(), beforeKey)
			}
			moves++
		}
	}
	t.Logf("made and unmade %d legal moves with no drift", moves)
}

// TestMakeUnmakeTwoPly does the same one ply deeper. Castling rights and en
// passant squares are destroyed by the first move and have to survive being
// restored underneath a second one.
func TestMakeUnmakeTwoPly(t *testing.T) {
	entries, err := epd.Load(filepath.Clean(corpus))
	if err != nil {
		t.Fatalf("load corpus: %v", err)
	}
	pairs := 0
	for _, e := range entries {
		pos, err := board.ParseFEN(e.FEN)
		if err != nil {
			t.Fatalf("line %d: %v", e.Line, err)
		}
		root := pos.FEN()

		var first MoveList
		GenerateLegal(pos, &first)
		for _, m1 := range first.Slice() {
			u1 := Make(pos, m1)
			afterFirst := pos.FEN()

			var second MoveList
			GenerateLegal(pos, &second)
			for _, m2 := range second.Slice() {
				u2 := Make(pos, m2)
				if got, want := pos.Key(), pos.ComputeKey(); got != want {
					t.Fatalf("line %d: %s %s drifted the key\n  %s\n  incremental %016x recomputed %016x",
						e.Line, m1, m2, e.FEN, got, want)
				}
				Unmake(pos, m2, u2)
				if got := pos.FEN(); got != afterFirst {
					t.Fatalf("line %d: %s then %s did not undo\n  want %s\n  got  %s",
						e.Line, m1, m2, afterFirst, got)
				}
				pairs++
			}

			Unmake(pos, m1, u1)
			if got := pos.FEN(); got != root {
				t.Fatalf("line %d: %s did not undo at the root\n  want %s\n  got  %s", e.Line, m1, root, got)
			}
		}
	}
	t.Logf("made and unmade %d move pairs", pairs)
}

func TestCastlingMovesTheRookAndKillsTheRights(t *testing.T) {
	pos, err := board.ParseFEN("r3k2r/8/8/8/8/8/8/R3K2R w KQkq - 4 12")
	if err != nil {
		t.Fatal(err)
	}
	var list MoveList
	GenerateLegal(pos, &list)

	var castle Move
	for _, m := range list.Slice() {
		if m.String() == "e1g1" {
			castle = m
		}
	}
	if castle == NullMove {
		t.Fatal("short castle was not generated")
	}

	Make(pos, castle)
	if got := pos.PieceAt(5); got != board.MakePiece(board.White, board.Rook) {
		t.Errorf("f1 holds %c, want the rook that castled", got.Char())
	}
	if got := pos.PieceAt(7); got != board.NoPiece {
		t.Errorf("h1 still holds %c after castling", got.Char())
	}
	if pos.Castle()&(board.WhiteKingside|board.WhiteQueenside) != 0 {
		t.Error("white kept a castling right after castling")
	}
	if pos.Castle()&(board.BlackKingside|board.BlackQueenside) == 0 {
		t.Error("castling stripped black's rights too")
	}
	if pos.Halfmove() != 5 {
		t.Errorf("halfmove clock = %d, want 5", pos.Halfmove())
	}
}

// TestCastlingThroughCheckIsIllegal pins the rule that make/unmake alone cannot
// see: the king may not cross an attacked square, even though it lands safely.
func TestCastlingThroughCheckIsIllegal(t *testing.T) {
	// A black rook on f8 rakes f1, the square the white king must cross.
	pos, err := board.ParseFEN("4kr2/8/8/8/8/8/8/R3K2R w KQ - 0 1")
	if err != nil {
		t.Fatal(err)
	}
	var list MoveList
	GenerateLegal(pos, &list)
	for _, m := range list.Slice() {
		if m.String() == "e1g1" {
			t.Error("king castled across an attacked square")
		}
	}

	// With the rook on g8 instead, only the destination is watched, which is
	// just as illegal.
	pos, err = board.ParseFEN("4k1r1/8/8/8/8/8/8/R3K2R w KQ - 0 1")
	if err != nil {
		t.Fatal(err)
	}
	GenerateLegal(pos, &list)
	for _, m := range list.Slice() {
		if m.String() == "e1g1" {
			t.Error("king castled into check")
		}
	}

	// And a king already in check may not castle out of it.
	pos, err = board.ParseFEN("4r3/8/8/8/8/8/8/R3K2R w KQ - 0 1")
	if err != nil {
		t.Fatal(err)
	}
	GenerateLegal(pos, &list)
	for _, m := range list.Slice() {
		if m.IsCastle() {
			t.Errorf("king castled out of check: %s", m)
		}
	}
}

// TestEnPassantRemovesTheRightPawn checks the one capture whose victim is not
// on the destination square.
func TestEnPassantRemovesTheRightPawn(t *testing.T) {
	pos, err := board.ParseFEN("rnbqkbnr/ppp1pppp/8/3pP3/8/8/PPPP1PPP/RNBQKBNR w KQkq d6 0 3")
	if err != nil {
		t.Fatal(err)
	}
	var list MoveList
	GenerateLegal(pos, &list)

	var ep Move
	for _, m := range list.Slice() {
		if m.String() == "e5d6" {
			ep = m
		}
	}
	if ep == NullMove {
		t.Fatal("en passant capture was not generated")
	}

	before := pos.FEN()
	undo := Make(pos, ep)
	if got := pos.PieceAt(35); got != board.NoPiece { // d5
		t.Errorf("d5 still holds %c; the captured pawn stayed on the board", got.Char())
	}
	if got := pos.PieceAt(43); got != board.MakePiece(board.White, board.Pawn) { // d6
		t.Errorf("d6 holds %c, want the white pawn", got.Char())
	}
	if undo.Captured() != board.MakePiece(board.Black, board.Pawn) {
		t.Errorf("captured %c, want a black pawn", undo.Captured().Char())
	}
	Unmake(pos, ep, undo)
	if got := pos.FEN(); got != before {
		t.Errorf("en passant did not undo\n  want %s\n  got  %s", before, got)
	}
}
