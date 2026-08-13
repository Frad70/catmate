// Package search finds a move. Negamax with alpha-beta, iterative deepening
// and a quiescence search at the leaves.
package search

import (
	"time"

	"github.com/Frad70/catmate/internal/board"
	"github.com/Frad70/catmate/internal/eval"
	"github.com/Frad70/catmate/internal/movegen"
)

const (
	// MateScore is the value of being mated at the root. Mates are stored as
	// MateScore minus the ply they happen at, so a mate in two beats a mate in
	// three without any special comparison.
	MateScore = 30000
	Infinity  = 40000

	maxPly = 64
)

// IsMate reports whether a score means forced mate, and in how many plies.
func IsMate(score int) (bool, int) {
	if score > MateScore-maxPly {
		return true, MateScore - score
	}
	if score < -MateScore+maxPly {
		return true, -MateScore - score
	}
	return false, 0
}

type Limits struct {
	Depth    int           // hard depth cap; 0 means "as deep as time allows"
	MoveTime time.Duration // wall clock for this move; 0 means no clock
	Nodes    uint64        // node cap; 0 means unlimited
}

type Result struct {
	Best  movegen.Move
	Score int
	Depth int
	Nodes uint64
	PV    []movegen.Move
}

type Searcher struct {
	nodes    uint64
	deadline time.Time
	maxNodes uint64
	stopped  bool

	// staticLeaves swaps quiescence for a plain evaluation at depth 0. Tests
	// use it to compare alpha-beta against unpruned negamax, where the leaf
	// value has to be a fixed function for the comparison to mean anything.
	staticLeaves bool

	pv       [maxPly][maxPly]movegen.Move
	pvLength [maxPly]int
}

func New() *Searcher { return &Searcher{} }

// Search runs iterative deepening and returns the best move it had time for.
// A depth that runs out of clock is discarded: a half-searched depth can
// easily be worse than the full depth below it.
func (s *Searcher) Search(p *board.Position, lim Limits) Result {
	s.nodes = 0
	s.stopped = false
	s.maxNodes = lim.Nodes
	if lim.MoveTime > 0 {
		s.deadline = time.Now().Add(lim.MoveTime)
	} else {
		s.deadline = time.Time{}
	}

	maxDepth := lim.Depth
	if maxDepth <= 0 || maxDepth > maxPly-2 {
		maxDepth = maxPly - 2
	}

	var best Result
	for depth := 1; depth <= maxDepth; depth++ {
		score := s.alphaBeta(p, depth, -Infinity, Infinity, 0)
		if s.stopped && depth > 1 {
			break
		}
		best = Result{
			Best:  s.pv[0][0],
			Score: score,
			Depth: depth,
			Nodes: s.nodes,
			PV:    append([]movegen.Move(nil), s.pv[0][:s.pvLength[0]]...),
		}
		if mate, _ := IsMate(score); mate {
			break // no point looking past a forced mate
		}
	}
	best.Nodes = s.nodes
	return best
}

// outOfTime is checked on a node counter rather than every node, because
// time.Now costs more than a few nodes of search.
func (s *Searcher) outOfTime() bool {
	if s.stopped {
		return true
	}
	if s.maxNodes > 0 && s.nodes >= s.maxNodes {
		s.stopped = true
		return true
	}
	if !s.deadline.IsZero() && s.nodes&2047 == 0 && time.Now().After(s.deadline) {
		s.stopped = true
		return true
	}
	return false
}

func (s *Searcher) alphaBeta(p *board.Position, depth, alpha, beta, ply int) int {
	s.pvLength[ply] = 0
	s.nodes++

	if ply > 0 && s.outOfTime() {
		return alpha
	}
	if depth <= 0 {
		if s.staticLeaves {
			return eval.Evaluate(p)
		}
		return s.quiescence(p, alpha, beta, ply)
	}

	var list movegen.MoveList
	movegen.GenerateLegal(p, &list)
	if list.Len() == 0 {
		if movegen.InCheck(p) {
			return -MateScore + ply // mated: the sooner, the worse
		}
		return 0 // stalemate
	}
	s.order(p, &list)

	best := -Infinity
	for _, m := range list.Slice() {
		undo := movegen.Make(p, m)
		score := -s.alphaBeta(p, depth-1, -beta, -alpha, ply+1)
		movegen.Unmake(p, m, undo)

		if score > best {
			best = score
		}
		if score > alpha {
			alpha = score
			s.savePV(m, ply)
		}
		if alpha >= beta {
			break // the opponent would never allow this line
		}
	}
	return best
}

// quiescence keeps searching captures until the position is quiet, so the
// engine does not stop counting in the middle of an exchange and call a hanging
// queen material.
func (s *Searcher) quiescence(p *board.Position, alpha, beta, ply int) int {
	s.nodes++
	if s.outOfTime() || ply >= maxPly-1 {
		return eval.Evaluate(p)
	}

	standPat := eval.Evaluate(p)
	if standPat >= beta {
		return standPat
	}
	if standPat > alpha {
		alpha = standPat
	}

	var list movegen.MoveList
	movegen.GenerateLegal(p, &list)
	s.order(p, &list)

	best := standPat
	for _, m := range list.Slice() {
		if !m.IsCapture() && !m.IsPromotion() {
			continue
		}
		undo := movegen.Make(p, m)
		score := -s.quiescence(p, -beta, -alpha, ply+1)
		movegen.Unmake(p, m, undo)

		if score > best {
			best = score
		}
		if score > alpha {
			alpha = score
		}
		if alpha >= beta {
			break
		}
	}
	return best
}

// order sorts moves so the likely-best are tried first, which is what makes
// alpha-beta cut anything at all. Captures go by most valuable victim, least
// valuable attacker; promotions ride near the top.
func (s *Searcher) order(p *board.Position, list *movegen.MoveList) {
	moves := list.Slice()
	scores := make([]int, len(moves))
	for i, m := range moves {
		scores[i] = moveScore(p, m)
	}
	// Insertion sort: lists are short and almost always partly ordered.
	for i := 1; i < len(moves); i++ {
		m, sc, j := moves[i], scores[i], i-1
		for j >= 0 && scores[j] < sc {
			moves[j+1], scores[j+1] = moves[j], scores[j]
			j--
		}
		moves[j+1], scores[j+1] = m, sc
	}
}

func moveScore(p *board.Position, m movegen.Move) int {
	score := 0
	if m.IsCapture() {
		victim := eval.Value(p.PieceAt(m.To()).Type())
		if m.IsEnPassant() {
			victim = eval.Value(board.Pawn)
		}
		attacker := eval.Value(p.PieceAt(m.From()).Type())
		score += 10_000 + 10*victim - attacker
	}
	if m.IsPromotion() {
		score += 9_000 + eval.Value(m.PromoType())
	}
	return score
}

// savePV stitches the move onto the line found one ply deeper, so the root ends
// up holding the whole principal variation.
func (s *Searcher) savePV(m movegen.Move, ply int) {
	s.pv[ply][0] = m
	copy(s.pv[ply][1:], s.pv[ply+1][:s.pvLength[ply+1]])
	s.pvLength[ply] = s.pvLength[ply+1] + 1
}
