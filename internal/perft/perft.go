// Package perft counts leaf nodes of the move tree. It is the engine's polygraph:
// the numbers are compared against an independent implementation, so a single
// wrong node means a real bug in generation, legality or make/unmake.
package perft

import (
	"sort"

	"github.com/Frad70/catmate/internal/board"
	"github.com/Frad70/catmate/internal/movegen"
)

// Perft walks the whole tree. Deliberately plain: no transposition table, no
// bulk counting at depth 1. Both would hide exactly the bugs this exists to
// find — a TT by merging positions that differ in a detail the key forgot, bulk
// counting by never making the leaf moves at all.
func Perft(p *board.Position, depth int) uint64 {
	if depth <= 0 {
		return 1
	}
	var list movegen.MoveList
	movegen.GenerateLegal(p, &list)

	var nodes uint64
	for _, m := range list.Slice() {
		undo := movegen.Make(p, m)
		nodes += Perft(p, depth-1)
		movegen.Unmake(p, m, undo)
	}
	return nodes
}

// Split is one root move and the nodes beneath it.
type Split struct {
	Move  movegen.Move
	Nodes uint64
}

// Divide reports the node count under each root move, sorted by move string so
// its output lines up with tools/perft_oracle.py divide and the two can be
// diffed straight across.
func Divide(p *board.Position, depth int) []Split {
	var list movegen.MoveList
	movegen.GenerateLegal(p, &list)

	splits := make([]Split, 0, list.Len())
	for _, m := range list.Slice() {
		undo := movegen.Make(p, m)
		splits = append(splits, Split{Move: m, Nodes: Perft(p, depth-1)})
		movegen.Unmake(p, m, undo)
	}
	sort.Slice(splits, func(i, j int) bool {
		return splits[i].Move.String() < splits[j].Move.String()
	})
	return splits
}
