package movegen

import "github.com/Frad70/catmate/internal/board"

// Precomputed jump attacks. Sliders are walked ray by ray instead, because a
// correct walk is easier to trust than a magic table and nothing has measured
// the difference yet.
var (
	knightAttacks [64]board.Bitboard
	kingAttacks   [64]board.Bitboard
	pawnAttacks   [2][64]board.Bitboard
)

type step struct{ df, dr int }

var (
	knightSteps = []step{{1, 2}, {2, 1}, {2, -1}, {1, -2}, {-1, -2}, {-2, -1}, {-2, 1}, {-1, 2}}
	kingSteps   = []step{{0, 1}, {1, 1}, {1, 0}, {1, -1}, {0, -1}, {-1, -1}, {-1, 0}, {-1, 1}}
	bishopSteps = []step{{1, 1}, {1, -1}, {-1, -1}, {-1, 1}}
	rookSteps   = []step{{0, 1}, {1, 0}, {0, -1}, {-1, 0}}
	queenSteps  = append(append([]step{}, bishopSteps...), rookSteps...)
)

// offset returns the square df files and dr ranks away, or SquareNone if that
// walks off the board. Working in file/rank terms is what keeps a knight on h1
// from wrapping around to a2.
func offset(s board.Square, df, dr int) board.Square {
	f, r := s.File()+df, s.Rank()+dr
	if f < 0 || f > 7 || r < 0 || r > 7 {
		return board.SquareNone
	}
	return board.MakeSquare(f, r)
}

func init() {
	for sq := board.Square(0); sq < 64; sq++ {
		for _, st := range knightSteps {
			if to := offset(sq, st.df, st.dr); to != board.SquareNone {
				knightAttacks[sq] |= board.BitOf(to)
			}
		}
		for _, st := range kingSteps {
			if to := offset(sq, st.df, st.dr); to != board.SquareNone {
				kingAttacks[sq] |= board.BitOf(to)
			}
		}
		for _, df := range []int{-1, 1} {
			if to := offset(sq, df, 1); to != board.SquareNone {
				pawnAttacks[board.White][sq] |= board.BitOf(to)
			}
			if to := offset(sq, df, -1); to != board.SquareNone {
				pawnAttacks[board.Black][sq] |= board.BitOf(to)
			}
		}
	}
}

// slidingAttacks walks each direction until the board edge or a blocker. The
// blocker's own square is included: whether it may be captured is the caller's
// business.
func slidingAttacks(sq board.Square, occupied board.Bitboard, steps []step) board.Bitboard {
	var attacks board.Bitboard
	for _, st := range steps {
		for to := offset(sq, st.df, st.dr); to != board.SquareNone; to = offset(to, st.df, st.dr) {
			attacks |= board.BitOf(to)
			if occupied.Has(to) {
				break
			}
		}
	}
	return attacks
}

// AttacksFrom is every square the given piece attacks from sq. Pawns attack
// diagonally only; their pushes are not attacks.
func AttacksFrom(pc board.Piece, sq board.Square, occupied board.Bitboard) board.Bitboard {
	switch pc.Type() {
	case board.Pawn:
		return pawnAttacks[pc.Color()][sq]
	case board.Knight:
		return knightAttacks[sq]
	case board.Bishop:
		return slidingAttacks(sq, occupied, bishopSteps)
	case board.Rook:
		return slidingAttacks(sq, occupied, rookSteps)
	case board.Queen:
		return slidingAttacks(sq, occupied, queenSteps)
	case board.King:
		return kingAttacks[sq]
	}
	return 0
}

// Attacked reports whether side by attacks sq. Asked once per legality check
// and once per castling square, so it walks outward from sq rather than
// scanning every enemy piece.
func Attacked(p *board.Position, sq board.Square, by board.Color) bool {
	occupied := p.OccupiedAll()

	if pawnAttacks[by.Opposite()][sq]&p.PiecesOf(by, board.Pawn) != 0 {
		return true
	}
	if knightAttacks[sq]&p.PiecesOf(by, board.Knight) != 0 {
		return true
	}
	if kingAttacks[sq]&p.PiecesOf(by, board.King) != 0 {
		return true
	}
	diagonal := p.PiecesOf(by, board.Bishop) | p.PiecesOf(by, board.Queen)
	if slidingAttacks(sq, occupied, bishopSteps)&diagonal != 0 {
		return true
	}
	straight := p.PiecesOf(by, board.Rook) | p.PiecesOf(by, board.Queen)
	return slidingAttacks(sq, occupied, rookSteps)&straight != 0
}

// KingSquare returns where the given side's king stands, or SquareNone in the
// contrived positions that have no king at all.
func KingSquare(p *board.Position, c board.Color) board.Square {
	kings := p.PiecesOf(c, board.King)
	if kings.Empty() {
		return board.SquareNone
	}
	return kings.Lsb()
}

// InCheck reports whether the side to move is in check.
func InCheck(p *board.Position) bool {
	ksq := KingSquare(p, p.Side())
	if ksq == board.SquareNone {
		return false
	}
	return Attacked(p, ksq, p.Side().Opposite())
}
