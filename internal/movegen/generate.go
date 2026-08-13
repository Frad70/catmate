package movegen

import "github.com/Frad70/catmate/internal/board"

// GeneratePseudoLegal fills list with every move that obeys how the pieces
// move. Some of them may leave the mover in check; filtering those out needs
// make/unmake and belongs one layer up.
func GeneratePseudoLegal(p *board.Position, list *MoveList) {
	list.Clear()
	us := p.Side()
	generatePawnMoves(p, us, list)
	generateJumpers(p, us, board.Knight, knightAttacks[:], list)
	generateJumpers(p, us, board.King, kingAttacks[:], list)
	generateSliders(p, us, board.Bishop, bishopSteps, list)
	generateSliders(p, us, board.Rook, rookSteps, list)
	generateSliders(p, us, board.Queen, queenSteps, list)
	generateCastles(p, us, list)
}

func generateJumpers(p *board.Position, us board.Color, pt board.PieceType, table []board.Bitboard, list *MoveList) {
	pieces := p.PiecesOf(us, pt)
	enemies := p.Occupied(us.Opposite())
	for !pieces.Empty() {
		from := pieces.PopLsb()
		targets := table[from] &^ p.Occupied(us)
		addTargets(from, targets, enemies, list)
	}
}

func generateSliders(p *board.Position, us board.Color, pt board.PieceType, steps []step, list *MoveList) {
	pieces := p.PiecesOf(us, pt)
	occupied := p.OccupiedAll()
	enemies := p.Occupied(us.Opposite())
	for !pieces.Empty() {
		from := pieces.PopLsb()
		targets := slidingAttacks(from, occupied, steps) &^ p.Occupied(us)
		addTargets(from, targets, enemies, list)
	}
}

func addTargets(from board.Square, targets, enemies board.Bitboard, list *MoveList) {
	for !targets.Empty() {
		to := targets.PopLsb()
		if enemies.Has(to) {
			list.Add(New(from, to, FlagCapture))
		} else {
			list.Add(New(from, to, FlagQuiet))
		}
	}
}

func generatePawnMoves(p *board.Position, us board.Color, list *MoveList) {
	pawns := p.PiecesOf(us, board.Pawn)
	occupied := p.OccupiedAll()
	enemies := p.Occupied(us.Opposite())

	forward, startRank, promoRank := 1, 1, 7
	if us == board.Black {
		forward, startRank, promoRank = -1, 6, 0
	}

	for !pawns.Empty() {
		from := pawns.PopLsb()

		if to := offset(from, 0, forward); to != board.SquareNone && !occupied.Has(to) {
			if to.Rank() == promoRank {
				addPromotions(from, to, false, list)
			} else {
				list.Add(New(from, to, FlagQuiet))
				// A double push is only possible when the single push was.
				if from.Rank() == startRank {
					if two := offset(from, 0, 2*forward); two != board.SquareNone && !occupied.Has(two) {
						list.Add(New(from, two, FlagDoublePush))
					}
				}
			}
		}

		captures := pawnAttacks[us][from] & enemies
		for !captures.Empty() {
			to := captures.PopLsb()
			if to.Rank() == promoRank {
				addPromotions(from, to, true, list)
			} else {
				list.Add(New(from, to, FlagCapture))
			}
		}

		if ep := p.EP(); ep != board.SquareNone && pawnAttacks[us][from].Has(ep) {
			list.Add(New(from, ep, FlagEnPassant))
		}
	}
}

func addPromotions(from, to board.Square, capture bool, list *MoveList) {
	base := FlagPromoKnight
	if capture {
		base = FlagPromoCaptureKnight
	}
	for i := Flag(0); i < 4; i++ {
		list.Add(New(from, to, base+i))
	}
}

// Castling squares, standard chess only. Chess960 would need the rook files
// read from the position instead of assumed.
type castleSpec struct {
	right board.CastleRights
	king  board.Square // king's start square
	to    board.Square // king's destination
	empty []board.Square
	flag  Flag
}

var castleSpecs = [2][2]castleSpec{
	board.White: {
		{board.WhiteKingside, 4, 6, []board.Square{5, 6}, FlagKingCastle},
		{board.WhiteQueenside, 4, 2, []board.Square{1, 2, 3}, FlagQueenCastle},
	},
	board.Black: {
		{board.BlackKingside, 60, 62, []board.Square{61, 62}, FlagKingCastle},
		{board.BlackQueenside, 60, 58, []board.Square{57, 58, 59}, FlagQueenCastle},
	},
}

// generateCastles emits a castle when the right survives and the path is clear.
// Whether the king starts, crosses or lands in check is a legality question,
// answered where every other legality question is answered.
func generateCastles(p *board.Position, us board.Color, list *MoveList) {
	occupied := p.OccupiedAll()
	for _, spec := range castleSpecs[us] {
		if p.Castle()&spec.right == 0 {
			continue
		}
		clear := true
		for _, sq := range spec.empty {
			if occupied.Has(sq) {
				clear = false
				break
			}
		}
		if clear {
			list.Add(New(spec.king, spec.to, spec.flag))
		}
	}
}
