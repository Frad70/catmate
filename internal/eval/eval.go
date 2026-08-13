// Package eval scores a position in centipawns from the point of view of the
// side to move, which is the convention negamax needs.
package eval

import (
	"github.com/Frad70/catmate/internal/board"
)

// Piece values. A bishop is worth a hair more than a knight, which is what
// makes the engine part with the knight first when it has to choose.
var pieceValue = [6]int{
	board.Pawn:   100,
	board.Knight: 320,
	board.Bishop: 330,
	board.Rook:   500,
	board.Queen:  900,
	board.King:   0,
}

// Value is the material worth of a piece type, used by move ordering too.
func Value(pt board.PieceType) int {
	if pt >= board.NoPieceType {
		return 0
	}
	return pieceValue[pt]
}

// Piece-square tables, written from White's side with a1 at the bottom left of
// each block, so the tables read like a board. Black looks them up mirrored.
var pieceSquare = [6][64]int{
	board.Pawn: {
		0, 0, 0, 0, 0, 0, 0, 0,
		5, 10, 10, -20, -20, 10, 10, 5,
		5, -5, -10, 0, 0, -10, -5, 5,
		0, 0, 0, 20, 20, 0, 0, 0,
		5, 5, 10, 25, 25, 10, 5, 5,
		10, 10, 20, 30, 30, 20, 10, 10,
		50, 50, 50, 50, 50, 50, 50, 50,
		0, 0, 0, 0, 0, 0, 0, 0,
	},
	board.Knight: {
		-50, -40, -30, -30, -30, -30, -40, -50,
		-40, -20, 0, 5, 5, 0, -20, -40,
		-30, 5, 10, 15, 15, 10, 5, -30,
		-30, 0, 15, 20, 20, 15, 0, -30,
		-30, 5, 15, 20, 20, 15, 5, -30,
		-30, 0, 10, 15, 15, 10, 0, -30,
		-40, -20, 0, 0, 0, 0, -20, -40,
		-50, -40, -30, -30, -30, -30, -40, -50,
	},
	board.Bishop: {
		-20, -10, -10, -10, -10, -10, -10, -20,
		-10, 5, 0, 0, 0, 0, 5, -10,
		-10, 10, 10, 10, 10, 10, 10, -10,
		-10, 0, 10, 10, 10, 10, 0, -10,
		-10, 5, 5, 10, 10, 5, 5, -10,
		-10, 0, 5, 10, 10, 5, 0, -10,
		-10, 0, 0, 0, 0, 0, 0, -10,
		-20, -10, -10, -10, -10, -10, -10, -20,
	},
	board.Rook: {
		0, 0, 0, 5, 5, 0, 0, 0,
		-5, 0, 0, 0, 0, 0, 0, -5,
		-5, 0, 0, 0, 0, 0, 0, -5,
		-5, 0, 0, 0, 0, 0, 0, -5,
		-5, 0, 0, 0, 0, 0, 0, -5,
		-5, 0, 0, 0, 0, 0, 0, -5,
		5, 10, 10, 10, 10, 10, 10, 5,
		0, 0, 0, 0, 0, 0, 0, 0,
	},
	board.Queen: {
		-20, -10, -10, -5, -5, -10, -10, -20,
		-10, 0, 5, 0, 0, 0, 0, -10,
		-10, 5, 5, 5, 5, 5, 0, -10,
		0, 0, 5, 5, 5, 5, 0, -5,
		-5, 0, 5, 5, 5, 5, 0, -5,
		-10, 0, 5, 5, 5, 5, 0, -10,
		-10, 0, 0, 0, 0, 0, 0, -10,
		-20, -10, -10, -5, -5, -10, -10, -20,
	},
}

// The king wants a corner while queens are on and the centre once they are
// gone. Without the second table the engine cannot walk a lone king to the
// edge, and won mating positions dribble away into fifty-move draws.
var kingMiddlegame = [64]int{
	20, 30, 10, 0, 0, 10, 30, 20,
	20, 20, 0, 0, 0, 0, 20, 20,
	-10, -20, -20, -20, -20, -20, -20, -10,
	-20, -30, -30, -40, -40, -30, -30, -20,
	-30, -40, -40, -50, -50, -40, -40, -30,
	-30, -40, -40, -50, -50, -40, -40, -30,
	-30, -40, -40, -50, -50, -40, -40, -30,
	-30, -40, -40, -50, -50, -40, -40, -30,
}

var kingEndgame = [64]int{
	-50, -30, -30, -30, -30, -30, -30, -50,
	-30, -30, 0, 0, 0, 0, -30, -30,
	-30, -10, 20, 30, 30, 20, -10, -30,
	-30, -10, 30, 40, 40, 30, -10, -30,
	-30, -10, 30, 40, 40, 30, -10, -30,
	-30, -10, 20, 30, 30, 20, -10, -30,
	-30, -20, -10, 0, 0, -10, -20, -30,
	-50, -40, -30, -20, -20, -30, -40, -50,
}

// endgameThreshold is the non-pawn material, per side counted together, below
// which the king stops hiding and starts helping.
const endgameThreshold = 2*500 + 2*330 // two rooks and two bishops

// Evaluate scores the position for the side to move. Positive means better
// for whoever is about to play.
func Evaluate(p *board.Position) int {
	var white, black, nonPawn int

	for pt := board.Pawn; pt <= board.Queen; pt++ {
		v := pieceValue[pt]
		pieces := p.Pieces(pt)

		w := pieces & p.Occupied(board.White)
		for !w.Empty() {
			sq := w.PopLsb()
			white += v + pieceSquare[pt][sq]
			if pt != board.Pawn {
				nonPawn += v
			}
		}

		b := pieces & p.Occupied(board.Black)
		for !b.Empty() {
			sq := b.PopLsb()
			black += v + pieceSquare[pt][mirror(sq)]
			if pt != board.Pawn {
				nonPawn += v
			}
		}
	}

	kings := p.Pieces(board.King)
	table := &kingMiddlegame
	if nonPawn <= endgameThreshold {
		table = &kingEndgame
	}
	if w := kings & p.Occupied(board.White); !w.Empty() {
		white += table[w.Lsb()]
	}
	if b := kings & p.Occupied(board.Black); !b.Empty() {
		black += table[mirror(b.Lsb())]
	}

	score := white - black
	if p.Side() == board.Black {
		return -score
	}
	return score
}

// mirror flips a square vertically, turning a white table lookup into a black
// one. a1 becomes a8 and the file is untouched.
func mirror(s board.Square) board.Square { return s ^ 56 }
