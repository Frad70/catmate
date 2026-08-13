// Package movegen turns a position into the moves that could be played in it.
//
// This layer is *pseudo-legal*: it obeys how pieces move, not whether the mover
// is left in check. Legality filtering and make/unmake come next; perft is what
// will prove both.
package movegen

import "github.com/Frad70/catmate/internal/board"

// Move packs everything a move needs into 16 bits:
//
//	bits  0-5   from square
//	bits  6-11  to square
//	bits 12-15  flag
//
// The flag layout is the usual one: within the flag nibble, bit 2 marks a
// capture and bit 3 marks a promotion, so both questions are a single mask.
type Move uint16

type Flag uint16

const (
	FlagQuiet       Flag = 0
	FlagDoublePush  Flag = 1
	FlagKingCastle  Flag = 2
	FlagQueenCastle Flag = 3
	FlagCapture     Flag = 4
	FlagEnPassant   Flag = 5

	FlagPromoKnight Flag = 8
	FlagPromoBishop Flag = 9
	FlagPromoRook   Flag = 10
	FlagPromoQueen  Flag = 11

	FlagPromoCaptureKnight Flag = 12
	FlagPromoCaptureBishop Flag = 13
	FlagPromoCaptureRook   Flag = 14
	FlagPromoCaptureQueen  Flag = 15

	flagCaptureBit   Flag = 4
	flagPromotionBit Flag = 8
)

const NullMove Move = 0

func New(from, to board.Square, flag Flag) Move {
	return Move(uint16(from)&0x3F | (uint16(to)&0x3F)<<6 | uint16(flag)<<12)
}

func (m Move) From() board.Square { return board.Square(m & 0x3F) }
func (m Move) To() board.Square   { return board.Square((m >> 6) & 0x3F) }
func (m Move) Flag() Flag         { return Flag(m >> 12) }

func (m Move) IsCapture() bool   { return m.Flag()&flagCaptureBit != 0 }
func (m Move) IsPromotion() bool { return m.Flag()&flagPromotionBit != 0 }
func (m Move) IsEnPassant() bool { return m.Flag() == FlagEnPassant }
func (m Move) IsCastle() bool    { f := m.Flag(); return f == FlagKingCastle || f == FlagQueenCastle }

// PromoType is meaningful only when IsPromotion reports true.
func (m Move) PromoType() board.PieceType {
	switch m.Flag() & 3 {
	case 0:
		return board.Knight
	case 1:
		return board.Bishop
	case 2:
		return board.Rook
	default:
		return board.Queen
	}
}

const promoChars = "nbrq"

// String is UCI long algebraic: e2e4, e7e8q.
func (m Move) String() string {
	if m == NullMove {
		return "0000"
	}
	s := m.From().String() + m.To().String()
	if m.IsPromotion() {
		s += string(promoChars[m.Flag()&3])
	}
	return s
}

// MoveList is a fixed-capacity buffer. 256 is comfortably above the widest
// known position, and a fixed array keeps generation allocation-free.
type MoveList struct {
	moves [256]Move
	count int
}

func (l *MoveList) Add(m Move)    { l.moves[l.count] = m; l.count++ }
func (l *MoveList) Len() int      { return l.count }
func (l *MoveList) At(i int) Move { return l.moves[i] }
func (l *MoveList) Clear()        { l.count = 0 }
func (l *MoveList) Slice() []Move { return l.moves[:l.count] }
