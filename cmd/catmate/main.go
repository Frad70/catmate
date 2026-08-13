// Command catmate is the engine binary. For now it counts; playing comes later.
package main

import (
	"fmt"
	"os"
	"runtime"
	"sort"
	"strconv"
	"sync"
	"time"

	"github.com/Frad70/catmate/internal/board"
	"github.com/Frad70/catmate/internal/epd"
	"github.com/Frad70/catmate/internal/perft"
)

const usage = `catmate — a chess engine

usage:
  catmate perft "<fen>" <depth>        node count for one position
  catmate divide "<fen>" <depth>       nodes under each root move, then the total
  catmate perftsuite <file.epd>        every position in the corpus, every depth
`

func main() {
	if len(os.Args) < 2 {
		fmt.Fprint(os.Stderr, usage)
		os.Exit(2)
	}

	var err error
	switch os.Args[1] {
	case "perft":
		err = cmdPerft(os.Args[2:])
	case "divide":
		err = cmdDivide(os.Args[2:])
	case "perftsuite":
		err = cmdPerftSuite(os.Args[2:])
	case "-h", "--help", "help":
		fmt.Print(usage)
		return
	default:
		fmt.Fprintf(os.Stderr, "unknown command %q\n\n%s", os.Args[1], usage)
		os.Exit(2)
	}

	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

func parsePositionArgs(args []string) (*board.Position, int, error) {
	if len(args) != 2 {
		return nil, 0, fmt.Errorf("need a FEN and a depth")
	}
	pos, err := board.ParseFEN(args[0])
	if err != nil {
		return nil, 0, err
	}
	depth, err := strconv.Atoi(args[1])
	if err != nil || depth < 0 {
		return nil, 0, fmt.Errorf("bad depth %q", args[1])
	}
	return pos, depth, nil
}

func cmdPerft(args []string) error {
	pos, depth, err := parsePositionArgs(args)
	if err != nil {
		return err
	}
	start := time.Now()
	nodes := perft.Perft(pos, depth)
	elapsed := time.Since(start)
	fmt.Println(nodes)
	fmt.Fprintf(os.Stderr, "%d nodes in %s (%.0f nps)\n",
		nodes, elapsed.Round(time.Millisecond), float64(nodes)/elapsed.Seconds())
	return nil
}

// cmdDivide prints "<uci> <nodes>" lines then "total <n>", matching
// tools/perft_oracle.py divide exactly so the two outputs can be diffed.
func cmdDivide(args []string) error {
	pos, depth, err := parsePositionArgs(args)
	if err != nil {
		return err
	}
	var total uint64
	for _, s := range perft.Divide(pos, depth) {
		fmt.Printf("%s %d\n", s.Move, s.Nodes)
		total += s.Nodes
	}
	fmt.Printf("total %d\n", total)
	return nil
}

type mismatch struct {
	line  int
	fen   string
	depth int
	want  uint64
	got   uint64
}

// cmdPerftSuite checks every position at every depth the oracle recorded.
// Positions are independent, so they run in parallel; the report is sorted back
// into file order afterwards to stay reproducible.
func cmdPerftSuite(args []string) error {
	if len(args) != 1 {
		return fmt.Errorf("need one .epd file")
	}
	entries, err := epd.Load(args[0])
	if err != nil {
		return err
	}

	var (
		wg        sync.WaitGroup
		mu        sync.Mutex
		bad       []mismatch
		nodes     uint64
		positions = make(chan epd.Entry)
	)

	workers := runtime.NumCPU()
	if workers > len(entries) {
		workers = len(entries)
	}
	start := time.Now()

	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for e := range positions {
				pos, err := board.ParseFEN(e.FEN)
				if err != nil {
					mu.Lock()
					bad = append(bad, mismatch{line: e.Line, fen: e.FEN})
					mu.Unlock()
					continue
				}
				for _, depth := range e.Depths() {
					want := e.Nodes[depth]
					got := perft.Perft(pos, depth)
					mu.Lock()
					nodes += got
					if got != want {
						bad = append(bad, mismatch{e.Line, e.FEN, depth, want, got})
					}
					mu.Unlock()
				}
			}
		}()
	}
	for _, e := range entries {
		positions <- e
	}
	close(positions)
	wg.Wait()

	sort.Slice(bad, func(i, j int) bool {
		if bad[i].line != bad[j].line {
			return bad[i].line < bad[j].line
		}
		return bad[i].depth < bad[j].depth
	})
	for _, m := range bad {
		fmt.Printf("FAIL line %d depth %d: want %d, got %d\n  %s\n", m.line, m.depth, m.want, m.got, m.fen)
	}

	elapsed := time.Since(start)
	fmt.Printf("positions: %d\n", len(entries))
	fmt.Printf("nodes: %d in %s (%.0f nps)\n",
		nodes, elapsed.Round(time.Millisecond), float64(nodes)/elapsed.Seconds())
	fmt.Printf("mismatches: %d\n", len(bad))
	if len(bad) > 0 {
		return fmt.Errorf("%d mismatches against the oracle", len(bad))
	}
	return nil
}
