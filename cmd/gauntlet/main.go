// Command gauntlet plays two UCI engines against each other and reports what
// broke. It is the referee for gate G4 and the scoreboard for G5.
//
// The runner trusts neither engine: every move an engine returns is checked
// against the referee's own legal move list, and the game's ending is decided
// from the position, never from what an engine says about it.
package main

import (
	"bufio"
	"flag"
	"fmt"
	"io"
	"math"
	"math/rand"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"

	"github.com/Frad70/catmate/internal/board"
	"github.com/Frad70/catmate/internal/epd"
	"github.com/Frad70/catmate/internal/game"
	"github.com/Frad70/catmate/internal/movegen"
)

func main() {
	var (
		games       = flag.Int("games", 100, "number of games to play")
		moveTime    = flag.Int("movetime", 50, "milliseconds per move")
		pathA       = flag.String("a", "./bin/catmate", "engine A binary")
		pathB       = flag.String("b", "./bin/catmate", "engine B binary")
		openings    = flag.String("openings", "", "EPD file of opening positions (optional)")
		openingPlie = flag.Int("openingplies", 4, "random plies to vary openings when no file is given")
		concurrency = flag.Int("concurrency", 6, "games in flight at once")
		maxMoves    = flag.Int("maxmoves", 300, "adjudicate a draw after this many moves")
		seed        = flag.Int64("seed", 1, "seed for the opening variation")
	)
	flag.Parse()

	starts, err := openingPositions(*openings, *games, *openingPlie, *seed)
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}

	cfg := config{
		pathA: *pathA, pathB: *pathB,
		moveTime: time.Duration(*moveTime) * time.Millisecond,
		maxMoves: *maxMoves,
	}

	results := make([]gameResult, *games)
	var wg sync.WaitGroup
	slots := make(chan struct{}, *concurrency)

	started := time.Now()
	for i := 0; i < *games; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			slots <- struct{}{}
			defer func() { <-slots }()
			// Colours alternate so a first-move advantage cannot decide a match.
			results[i] = playGame(cfg, starts[i], i%2 == 1)
		}(i)
	}
	wg.Wait()

	report(results, time.Since(started))
}

type config struct {
	pathA, pathB string
	moveTime     time.Duration
	maxMoves     int
}

type gameResult struct {
	outcome  game.Outcome // in terms of white and black
	aIsWhite bool
	reason   string
	illegal  bool
	crash    bool
	timeout  bool
	moves    int
	detail   string
}

// scoreForA converts an outcome into A's share of the point.
func (r gameResult) scoreForA() float64 {
	switch r.outcome {
	case game.WhiteWins:
		if r.aIsWhite {
			return 1
		}
		return 0
	case game.BlackWins:
		if r.aIsWhite {
			return 0
		}
		return 1
	}
	return 0.5
}

func openingPositions(file string, games, plies int, seed int64) ([]string, error) {
	out := make([]string, games)

	if file != "" {
		entries, err := epd.Load(file)
		if err != nil {
			return nil, err
		}
		for i := range out {
			// Each opening is played twice, once from each side.
			out[i] = entries[(i/2)%len(entries)].FEN
		}
		return out, nil
	}

	// Without a book, vary the opening by playing a few random legal moves.
	// Two deterministic engines from one position would otherwise play the
	// same game a hundred times and prove nothing.
	rng := rand.New(rand.NewSource(seed))
	for i := 0; i < games; i += 2 {
		pos, err := board.ParseFEN(board.StartFEN)
		if err != nil {
			return nil, err
		}
		for ply := 0; ply < plies; ply++ {
			var list movegen.MoveList
			movegen.GenerateLegal(pos, &list)
			if list.Len() == 0 {
				break
			}
			movegen.Make(pos, list.At(rng.Intn(list.Len())))
		}
		fen := pos.FEN()
		out[i] = fen
		if i+1 < games {
			out[i+1] = fen
		}
	}
	return out, nil
}

func playGame(cfg config, startFEN string, aIsBlack bool) gameResult {
	res := gameResult{aIsWhite: !aIsBlack}

	whitePath, blackPath := cfg.pathA, cfg.pathB
	if aIsBlack {
		whitePath, blackPath = cfg.pathB, cfg.pathA
	}
	white, err := startEngine(whitePath)
	if err != nil {
		return crashed(res, "white failed to start: "+err.Error())
	}
	defer white.close()

	black, err := startEngine(blackPath)
	if err != nil {
		return crashed(res, "black failed to start: "+err.Error())
	}
	defer black.close()

	for _, e := range []*engine{white, black} {
		if err := e.handshake(); err != nil {
			return crashed(res, "handshake: "+err.Error())
		}
	}

	pos, err := board.ParseFEN(startFEN)
	if err != nil {
		return crashed(res, "bad opening: "+err.Error())
	}
	repetitions := map[uint64]int{pos.Key(): 1}
	var played []string

	for {
		if verdict := game.Adjudicate(pos, repetitions); verdict.Outcome != game.Ongoing {
			res.outcome, res.reason = verdict.Outcome, verdict.Reason
			res.moves = len(played)
			return res
		}
		if len(played) >= cfg.maxMoves*2 {
			res.outcome, res.reason = game.Draw, "move limit"
			res.moves = len(played)
			return res
		}

		mover := white
		if pos.Side() == board.Black {
			mover = black
		}

		text, err := mover.think(startFEN, played, cfg.moveTime)
		if err != nil {
			res.moves = len(played)
			if err == errTimeout {
				res.timeout = true
				res.reason = "timeout"
			} else {
				res.crash = true
				res.reason = "crash"
			}
			res.detail = err.Error()
			res.outcome = loserIs(pos.Side())
			return res
		}

		move, ok := findLegal(pos, text)
		if !ok {
			res.illegal = true
			res.reason = "illegal move"
			res.detail = fmt.Sprintf("%s in %s", text, pos.FEN())
			res.outcome = loserIs(pos.Side())
			res.moves = len(played)
			return res
		}

		movegen.Make(pos, move)
		played = append(played, text)
		repetitions[pos.Key()]++
	}
}

func loserIs(side board.Color) game.Outcome {
	if side == board.White {
		return game.BlackWins
	}
	return game.WhiteWins
}

func crashed(res gameResult, detail string) gameResult {
	res.crash = true
	res.reason = "crash"
	res.detail = detail
	res.outcome = game.Draw
	return res
}

func findLegal(pos *board.Position, text string) (movegen.Move, bool) {
	var list movegen.MoveList
	movegen.GenerateLegal(pos, &list)
	for _, m := range list.Slice() {
		if m.String() == text {
			return m, true
		}
	}
	return movegen.NullMove, false
}

func report(results []gameResult, elapsed time.Duration) {
	var (
		scoreA                 float64
		wins, losses, draws    int
		illegal, crashes, outs int
		moves                  int
		reasons                = map[string]int{}
	)
	for _, r := range results {
		s := r.scoreForA()
		scoreA += s
		switch {
		case s == 1:
			wins++
		case s == 0:
			losses++
		default:
			draws++
		}
		if r.illegal {
			illegal++
		}
		if r.crash {
			crashes++
		}
		if r.timeout {
			outs++
		}
		moves += r.moves
		reasons[r.reason]++
		if r.illegal || r.crash || r.timeout {
			fmt.Printf("BROKEN %s: %s\n", r.reason, r.detail)
		}
	}

	n := float64(len(results))
	percent := 100 * scoreA / n

	fmt.Printf("games: %d in %s\n", len(results), elapsed.Round(time.Millisecond))
	fmt.Printf("moves: %d total, %.1f per game\n", moves, float64(moves)/n)
	for reason, count := range reasons {
		if reason == "" {
			reason = "unfinished"
		}
		fmt.Printf("  %-22s %d\n", reason+":", count)
	}
	fmt.Printf("A: +%d =%d -%d\n", wins, draws, losses)
	fmt.Printf("score: %.1f%% (%.1f/%d) elo %+.0f +/- %.0f\n",
		percent, scoreA, len(results), elo(scoreA/n), eloError(results, scoreA/n))
	fmt.Printf("illegal: %d\n", illegal)
	fmt.Printf("crashes: %d\n", crashes)
	fmt.Printf("timeouts: %d\n", outs)
}

// elo converts a score fraction into a rating difference. A clean sweep is
// reported as a large but finite number rather than infinity.
func elo(score float64) float64 {
	if score <= 0 {
		return -800
	}
	if score >= 1 {
		return 800
	}
	return -400 * math.Log10(1/score-1)
}

// eloError is a rough 95% interval from the spread of game results. It exists
// so a 55% score over 20 games cannot masquerade as progress.
func eloError(results []gameResult, mean float64) float64 {
	n := float64(len(results))
	if n < 2 {
		return 800
	}
	var variance float64
	for _, r := range results {
		d := r.scoreForA() - mean
		variance += d * d
	}
	variance /= n - 1
	stderr := math.Sqrt(variance / n)
	low, high := mean-1.96*stderr, mean+1.96*stderr
	return (elo(math.Min(high, 0.999)) - elo(math.Max(low, 0.001))) / 2
}

// ---- the UCI client half ----

var errTimeout = fmt.Errorf("engine did not answer in time")

type engine struct {
	cmd   *exec.Cmd
	stdin io.WriteCloser
	lines chan string
	done  chan struct{}
}

func startEngine(path string) (*engine, error) {
	cmd := exec.Command(path)
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, err
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	cmd.Stderr = nil
	if err := cmd.Start(); err != nil {
		return nil, err
	}

	e := &engine{cmd: cmd, stdin: stdin, lines: make(chan string, 256), done: make(chan struct{})}
	go func() {
		defer close(e.lines)
		scanner := bufio.NewScanner(stdout)
		scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
		for scanner.Scan() {
			select {
			case e.lines <- scanner.Text():
			case <-e.done:
				return
			}
		}
	}()
	return e, nil
}

func (e *engine) send(format string, args ...any) error {
	_, err := fmt.Fprintf(e.stdin, format+"\n", args...)
	return err
}

// expect reads until a line starts with prefix. A closed pipe means the engine
// died, which is a crash and not a timeout — the difference matters in the
// report.
func (e *engine) expect(prefix string, timeout time.Duration) (string, error) {
	deadline := time.After(timeout)
	for {
		select {
		case line, ok := <-e.lines:
			if !ok {
				return "", fmt.Errorf("engine exited while waiting for %q", prefix)
			}
			if strings.HasPrefix(line, prefix) {
				return line, nil
			}
		case <-deadline:
			return "", errTimeout
		}
	}
}

func (e *engine) handshake() error {
	if err := e.send("uci"); err != nil {
		return err
	}
	if _, err := e.expect("uciok", 5*time.Second); err != nil {
		return err
	}
	if err := e.send("isready"); err != nil {
		return err
	}
	if _, err := e.expect("readyok", 5*time.Second); err != nil {
		return err
	}
	return e.send("ucinewgame")
}

// think asks for a move and returns it as UCI text. The budget is generous
// compared with movetime: an engine is allowed to be slow, but not silent.
func (e *engine) think(startFEN string, moves []string, moveTime time.Duration) (string, error) {
	position := "position fen " + startFEN
	if len(moves) > 0 {
		position += " moves " + strings.Join(moves, " ")
	}
	if err := e.send("%s", position); err != nil {
		return "", err
	}
	if err := e.send("go movetime %d", moveTime.Milliseconds()); err != nil {
		return "", err
	}

	line, err := e.expect("bestmove", moveTime*4+2*time.Second)
	if err != nil {
		return "", err
	}
	fields := strings.Fields(line)
	if len(fields) < 2 {
		return "", fmt.Errorf("malformed bestmove line %q", line)
	}
	return fields[1], nil
}

func (e *engine) close() {
	e.send("quit")
	close(e.done)
	timer := time.AfterFunc(2*time.Second, func() {
		if e.cmd.Process != nil {
			e.cmd.Process.Kill()
		}
	})
	e.stdin.Close()
	e.cmd.Wait()
	timer.Stop()
}
