package movegen

import "github.com/Frad70/catmate/internal/board"

// Make and Unmake live here rather than in package board because Move lives
// here, and board must not import its own consumer. Everything they touch goes
// through the position's exported mutators, which is what keeps the zobrist key
// in step.

// Undo carries what a move destroys and cannot be derived back: the captured
// piece, the rights and en passant square that existed before, and the clocks.
// The key is kept only so a test can prove the incremental key came home on its
// own; Unmake never assigns it.
type Undo struct {
	captured board.Piece
	castle   board.CastleRights
	ep       board.Square
	halfmove int
	fullmove int
	key      uint64
}

// Captured reports the piece this move removed, or board.NoPiece.
func (u Undo) Captured() board.Piece { return u.captured }

// KeyBefore is the position key from before the move was made.
func (u Undo) KeyBefore() uint64 { return u.key }

// rightsLostFrom returns the castling rights that die when a piece leaves or is
// captured on this square. A rook that moves, and a rook that is taken where it
// stands, cost the same right.
func rightsLostFrom(sq board.Square) board.CastleRights {
	switch sq {
	case 0:
		return board.WhiteQueenside
	case 7:
		return board.WhiteKingside
	case 56:
		return board.BlackQueenside
	case 63:
		return board.BlackKingside
	}
	return 0
}

// rookHop is where the rook starts and lands for each castle.
func rookHop(us board.Color, flag Flag) (from, to board.Square) {
	if us == board.White {
		if flag == FlagKingCastle {
			return 7, 5
		}
		return 0, 3
	}
	if flag == FlagKingCastle {
		return 63, 61
	}
	return 56, 59
}

// Make applies a move and returns what Unmake will need. The move must be
// pseudo-legal for this position.
func Make(p *board.Position, m Move) Undo {
	us := p.Side()
	from, to := m.From(), m.To()
	piece := p.PieceAt(from)

	undo := Undo{
		captured: board.NoPiece,
		castle:   p.Castle(),
		ep:       p.EP(),
		halfmove: p.Halfmove(),
		fullmove: p.Fullmove(),
		key:      p.Key(),
	}

	// The captured pawn of an en passant sits beside the destination, not on
	// it, which is the whole reason this case exists.
	if m.IsEnPassant() {
		capSq := to - 8
		if us == board.Black {
			capSq = to + 8
		}
		undo.captured = p.PieceAt(capSq)
		p.Remove(capSq)
	} else if m.IsCapture() {
		undo.captured = p.PieceAt(to)
	}

	p.Remove(from)
	if m.IsPromotion() {
		p.Put(to, board.MakePiece(us, m.PromoType()))
	} else {
		p.Put(to, piece)
	}

	if m.IsCastle() {
		rookFrom, rookTo := rookHop(us, m.Flag())
		rook := p.PieceAt(rookFrom)
		p.Remove(rookFrom)
		p.Put(rookTo, rook)
	}

	rights := p.Castle()
	switch piece.Type() {
	case board.King:
		if us == board.White {
			rights &^= board.WhiteKingside | board.WhiteQueenside
		} else {
			rights &^= board.BlackKingside | board.BlackQueenside
		}
	case board.Rook:
		rights &^= rightsLostFrom(from)
	}
	rights &^= rightsLostFrom(to) // a captured rook loses its side the right
	p.SetCastle(rights)

	if m.Flag() == FlagDoublePush {
		behind := from + 8
		if us == board.Black {
			behind = from - 8
		}
		p.SetEP(behind)
	} else {
		p.SetEP(board.SquareNone)
	}

	if piece.Type() == board.Pawn || undo.captured != board.NoPiece {
		p.SetHalfmove(0)
	} else {
		p.SetHalfmove(undo.halfmove + 1)
	}
	if us == board.Black {
		p.SetFullmove(undo.fullmove + 1)
	}
	p.SetSide(us.Opposite())

	return undo
}

// Unmake reverses Make exactly. It restores the board by inverse operations and
// never writes the key directly, so the incremental key has to find its own way
// home — which is precisely what the tests check.
func Unmake(p *board.Position, m Move, undo Undo) {
	us := p.Side().Opposite() // the side that moved
	from, to := m.From(), m.To()

	p.SetSide(us)
	p.SetCastle(undo.castle)
	p.SetEP(undo.ep)
	p.SetHalfmove(undo.halfmove)
	p.SetFullmove(undo.fullmove)

	if m.IsCastle() {
		rookFrom, rookTo := rookHop(us, m.Flag())
		rook := p.PieceAt(rookTo)
		p.Remove(rookTo)
		p.Put(rookFrom, rook)
	}

	moved := p.PieceAt(to)
	p.Remove(to)
	if m.IsPromotion() {
		p.Put(from, board.MakePiece(us, board.Pawn))
	} else {
		p.Put(from, moved)
	}

	if undo.captured == board.NoPiece {
		return
	}
	if m.IsEnPassant() {
		capSq := to - 8
		if us == board.Black {
			capSq = to + 8
		}
		p.Put(capSq, undo.captured)
		return
	}
	p.Put(to, undo.captured)
}

// GenerateLegal keeps only the moves that leave our own king safe. The cheap
// route: make it, ask, unmake it. Castling is the exception — the king may not
// start in check nor cross an attacked square, and neither shows up in the
// position after the move.
func GenerateLegal(p *board.Position, list *MoveList) {
	var pseudo MoveList
	GeneratePseudoLegal(p, &pseudo)

	us := p.Side()
	them := us.Opposite()
	list.Clear()

	for _, m := range pseudo.Slice() {
		if m.IsCastle() && !castlePathIsSafe(p, us, them, m) {
			continue
		}
		undo := Make(p, m)
		if ksq := KingSquare(p, us); ksq == board.SquareNone || !Attacked(p, ksq, them) {
			list.Add(m)
		}
		Unmake(p, m, undo)
	}
}

// castlePathIsSafe checks the squares the king occupies on its way: where it
// stands, what it crosses, and where it lands. The b-file square of a long
// castle only has to be empty, which generation already ensured.
func castlePathIsSafe(p *board.Position, us, them board.Color, m Move) bool {
	from, to := m.From(), m.To()
	step := board.Square(1)
	if to < from {
		step = -1
	}
	for sq := from; ; sq += step {
		if Attacked(p, sq, them) {
			return false
		}
		if sq == to {
			return true
		}
	}
}
