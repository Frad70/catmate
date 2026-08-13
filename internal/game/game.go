// Package game is the referee: it decides when a game is over and why.
//
// The engine playing the game is not asked for its opinion. Everything here is
// derived from the position itself, which is what lets a match runner catch an
// engine that claims a win it does not have.
package game

import (
	"github.com/Frad70/catmate/internal/board"
	"github.com/Frad70/catmate/internal/movegen"
)

type Outcome int

const (
	Ongoing Outcome = iota
	WhiteWins
	BlackWins
	Draw
)

func (o Outcome) String() string {
	switch o {
	case WhiteWins:
		return "1-0"
	case BlackWins:
		return "0-1"
	case Draw:
		return "1/2-1/2"
	}
	return "*"
}

type Result struct {
	Outcome Outcome
	Reason  string
}

// Adjudicate reports whether the game has ended in this position. repetitions
// counts how many times each position key has been seen, including this one.
func Adjudicate(p *board.Position, repetitions map[uint64]int) Result {
	var list movegen.MoveList
	movegen.GenerateLegal(p, &list)

	if list.Len() == 0 {
		if movegen.InCheck(p) {
			// The side to move is mated, so the other one won.
			if p.Side() == board.White {
				return Result{BlackWins, "checkmate"}
			}
			return Result{WhiteWins, "checkmate"}
		}
		return Result{Draw, "stalemate"}
	}

	if p.Halfmove() >= 100 {
		return Result{Draw, "fifty-move rule"}
	}
	if repetitions[p.Key()] >= 3 {
		return Result{Draw, "threefold repetition"}
	}
	if InsufficientMaterial(p) {
		return Result{Draw, "insufficient material"}
	}
	return Result{Ongoing, ""}
}

// InsufficientMaterial reports positions where neither side can deliver mate
// however badly both play. Verified case by case against python-chess.
//
// The rule is not "few pieces left": two knights against a bare king is *not*
// dead, because mate can happen even though it cannot be forced. Two bishops on
// squares of one colour against a bare king *is* dead, however many there are.
func InsufficientMaterial(p *board.Position) bool {
	if !p.Pieces(board.Pawn).Empty() || !p.Pieces(board.Rook).Empty() || !p.Pieces(board.Queen).Empty() {
		return false
	}

	knights := p.Pieces(board.Knight)
	bishops := p.Pieces(board.Bishop)

	if knights.Empty() {
		return allOneColor(bishops)
	}
	// A lone knight cannot mate; anything more can, with help.
	return knights.Count() == 1 && bishops.Empty()
}

// allOneColor reports whether every bishop on the board, of either side, stands
// on squares of a single colour — in which case none of them can ever cover the
// squares needed to mate.
func allOneColor(bishops board.Bitboard) bool {
	if bishops.Empty() {
		return true
	}
	first := squareColor(bishops.Lsb())
	for !bishops.Empty() {
		if squareColor(bishops.PopLsb()) != first {
			return false
		}
	}
	return true
}

// squareColor is true for light squares. File and rank share a parity on
// squares of the same colour.
func squareColor(s board.Square) bool { return (s.File()+s.Rank())%2 == 1 }
