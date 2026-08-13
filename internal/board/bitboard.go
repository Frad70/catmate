package board

import "math/bits"

// Bit set helpers. Squares are bit indices, so a1 is bit 0 and h8 is bit 63.

func BitOf(s Square) Bitboard { return Bitboard(1) << uint(s) }

func (b Bitboard) Count() int { return bits.OnesCount64(uint64(b)) }

func (b Bitboard) Empty() bool { return b == 0 }

// Lsb returns the lowest set square. Callers must check Empty first.
func (b Bitboard) Lsb() Square { return Square(bits.TrailingZeros64(uint64(b))) }

// PopLsb removes and returns the lowest set square, which is how every
// generation loop walks a set of pieces.
func (b *Bitboard) PopLsb() Square {
	s := b.Lsb()
	*b &= *b - 1
	return s
}

// String renders the board with a1 at the bottom left, for debugging by eye.
func (b Bitboard) String() string {
	out := make([]byte, 0, 72)
	for rank := 7; rank >= 0; rank-- {
		for file := 0; file < 8; file++ {
			if b.Has(MakeSquare(file, rank)) {
				out = append(out, 'x')
			} else {
				out = append(out, '.')
			}
		}
		out = append(out, '\n')
	}
	return string(out)
}
