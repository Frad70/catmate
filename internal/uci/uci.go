// Package uci speaks the Universal Chess Interface: the protocol every chess
// GUI and match runner uses to drive an engine over stdin and stdout.
package uci

import (
	"bufio"
	"fmt"
	"io"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/Frad70/catmate/internal/board"
	"github.com/Frad70/catmate/internal/movegen"
	"github.com/Frad70/catmate/internal/search"
)

const (
	engineName   = "catmate"
	engineAuthor = "Claude"
)

type Engine struct {
	// outMu serialises writes: the search goroutine and the command reader
	// both talk to the GUI, and a half-written info line is a protocol error.
	outMu sync.Mutex
	out   *bufio.Writer

	mu       sync.Mutex
	pos      *board.Position
	searcher *search.Searcher

	// searching is held for the lifetime of one go/bestmove pair so quit can
	// wait for a search to finish instead of cutting it off mid-write.
	searching sync.WaitGroup
}

func NewEngine(out io.Writer) *Engine {
	pos, _ := board.ParseFEN(board.StartFEN)
	return &Engine{
		out:      bufio.NewWriter(out),
		pos:      pos,
		searcher: search.New(),
	}
}

// Run reads commands until stdin closes or quit arrives.
func (e *Engine) Run(in io.Reader) error {
	scanner := bufio.NewScanner(in)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)

	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		if len(fields) == 0 {
			continue
		}
		switch fields[0] {
		case "uci":
			e.send("id name %s", engineName)
			e.send("id author %s", engineAuthor)
			e.send("uciok")
		case "isready":
			// Answering while a search runs is required by the protocol, so
			// this must not take the lock the search holds.
			e.send("readyok")
		case "ucinewgame":
			e.searcher.Stop()
			e.searching.Wait()
			e.setPosition(board.StartFEN, nil)
		case "position":
			e.cmdPosition(fields[1:])
		case "go":
			e.cmdGo(fields[1:])
		case "stop":
			e.searcher.Stop()
		case "quit":
			e.searcher.Stop()
			e.searching.Wait()
			return nil
		}
	}
	e.searcher.Stop()
	e.searching.Wait()
	return scanner.Err()
}

func (e *Engine) send(format string, args ...any) {
	e.outMu.Lock()
	defer e.outMu.Unlock()
	fmt.Fprintf(e.out, format+"\n", args...)
	e.out.Flush()
}

// cmdPosition handles "position [startpos | fen <6 fields>] [moves ...]".
func (e *Engine) cmdPosition(args []string) {
	if len(args) == 0 {
		return
	}
	fen := board.StartFEN
	rest := args

	switch args[0] {
	case "startpos":
		rest = args[1:]
	case "fen":
		// A FEN is six fields, but some senders drop the clocks.
		end := 1
		for end < len(args) && args[end] != "moves" {
			end++
		}
		fen = strings.Join(args[1:end], " ")
		rest = args[end:]
	default:
		return
	}

	var moves []string
	if len(rest) > 0 && rest[0] == "moves" {
		moves = rest[1:]
	}
	e.setPosition(fen, moves)
}

// setPosition replays the move list, matching each UCI string against the
// moves actually generated. Trusting the string instead would let a typo or a
// protocol slip put the engine on a board the opponent is not playing.
func (e *Engine) setPosition(fen string, moves []string) {
	pos, err := board.ParseFEN(fen)
	if err != nil {
		e.send("info string bad fen: %v", err)
		return
	}
	for _, text := range moves {
		var list movegen.MoveList
		movegen.GenerateLegal(pos, &list)
		found := false
		for _, m := range list.Slice() {
			if m.String() == text {
				movegen.Make(pos, m)
				found = true
				break
			}
		}
		if !found {
			e.send("info string illegal move %s in %s", text, pos.FEN())
			return
		}
	}
	e.mu.Lock()
	e.pos = pos
	e.mu.Unlock()
}

func (e *Engine) cmdGo(args []string) {
	limits := parseGo(args, e.sideToMove())

	e.mu.Lock()
	pos, err := board.ParseFEN(e.pos.FEN()) // the search gets its own copy
	e.mu.Unlock()
	if err != nil {
		e.send("info string cannot copy position: %v", err)
		return
	}
	e.searching.Wait() // one search at a time
	// Arm the stop flag here, on the reader goroutine, so a stop sent right
	// behind this go command cannot be cleared out from under itself.
	e.searcher.Ready()
	e.searching.Add(1)
	go func() {
		defer e.searching.Done()
		e.runSearch(pos, limits)
	}()
}

// runSearch is the whole reason go runs on its own goroutine: while it works,
// Run keeps reading, so stop and isready still get answers.
func (e *Engine) runSearch(pos *board.Position, limits search.Limits) {
	start := time.Now()
	result := e.searcher.Search(pos, limits)
	elapsed := time.Since(start)

	e.send("info depth %d score %s nodes %d time %d pv %s",
		result.Depth, scoreString(result.Score), result.Nodes,
		elapsed.Milliseconds(), pvString(result.PV))

	if result.Best == movegen.NullMove {
		// No legal move: mated or stalemated. Saying so beats going silent,
		// which would hang the opponent's clock.
		e.send("bestmove 0000")
		return
	}
	e.send("bestmove %s", result.Best)
}

func (e *Engine) sideToMove() board.Color {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.pos.Side()
}

// parseGo reads the search limits. Clock arguments are turned into a movetime
// by a deliberately dull rule: a thirtieth of what is left, plus most of the
// increment. Nothing here is tuned; G5 will decide whether it deserves to be.
func parseGo(args []string, side board.Color) search.Limits {
	var (
		limits     search.Limits
		wtime      time.Duration
		btime      time.Duration
		winc, binc time.Duration
		haveClock  bool
		haveMove   bool
	)

	for i := 0; i < len(args); i++ {
		value := func() int {
			if i+1 < len(args) {
				n, err := strconv.Atoi(args[i+1])
				if err == nil {
					i++
					return n
				}
			}
			return 0
		}
		switch args[i] {
		case "depth":
			limits.Depth = value()
		case "nodes":
			limits.Nodes = uint64(value())
		case "movetime":
			limits.MoveTime = time.Duration(value()) * time.Millisecond
			haveMove = true
		case "wtime":
			wtime, haveClock = time.Duration(value())*time.Millisecond, true
		case "btime":
			btime, haveClock = time.Duration(value())*time.Millisecond, true
		case "winc":
			winc = time.Duration(value()) * time.Millisecond
		case "binc":
			binc = time.Duration(value()) * time.Millisecond
		case "infinite":
			limits.Depth = 0
			limits.MoveTime = 0
		}
	}

	if !haveMove && haveClock {
		remaining, increment := wtime, winc
		if side == board.Black {
			remaining, increment = btime, binc
		}
		budget := remaining/30 + increment*3/4
		// Never spend the last of the clock, and never think for zero time.
		if budget > remaining*3/4 {
			budget = remaining * 3 / 4
		}
		if budget < 10*time.Millisecond {
			budget = 10 * time.Millisecond
		}
		limits.MoveTime = budget
	}
	return limits
}

// scoreString reports mates in moves, as the protocol expects, not in plies.
func scoreString(score int) string {
	if mate, plies := search.IsMate(score); mate {
		moves := (plies + 1) / 2
		if plies < 0 {
			moves = -((-plies + 1) / 2)
		}
		return fmt.Sprintf("mate %d", moves)
	}
	return fmt.Sprintf("cp %d", score)
}

func pvString(pv []movegen.Move) string {
	parts := make([]string, 0, len(pv))
	for _, m := range pv {
		parts = append(parts, m.String())
	}
	return strings.Join(parts, " ")
}
