// Package epd reads the perft corpus: one position per line, as
//
//	<FEN> ;D1 <nodes> ;D2 <nodes> ...
//
// The file is ground truth written by tools/perft_oracle.py. Nothing here ever
// writes it.
package epd

import (
	"bufio"
	"fmt"
	"os"
	"strconv"
	"strings"
)

type Entry struct {
	FEN string
	// Nodes[d] is the expected perft node count at depth d. Index 0 is unused
	// so a depth reads as itself.
	Nodes map[int]uint64
	Line  int
}

// Depths returns the depths present, ascending.
func (e Entry) Depths() []int {
	out := make([]int, 0, len(e.Nodes))
	for d := 1; d <= 64; d++ {
		if _, ok := e.Nodes[d]; ok {
			out = append(out, d)
		}
	}
	return out
}

func Load(path string) ([]Entry, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	var entries []Entry
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	lineNo := 0
	for sc.Scan() {
		lineNo++
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		entry, err := parseLine(line, lineNo)
		if err != nil {
			return nil, err
		}
		entries = append(entries, entry)
	}
	if err := sc.Err(); err != nil {
		return nil, err
	}
	if len(entries) == 0 {
		return nil, fmt.Errorf("%s holds no positions", path)
	}
	return entries, nil
}

func parseLine(line string, lineNo int) (Entry, error) {
	parts := strings.Split(line, ";")
	entry := Entry{
		FEN:   strings.TrimSpace(parts[0]),
		Nodes: make(map[int]uint64, len(parts)-1),
		Line:  lineNo,
	}
	if entry.FEN == "" {
		return entry, fmt.Errorf("line %d: no FEN", lineNo)
	}
	for _, spec := range parts[1:] {
		fields := strings.Fields(spec)
		if len(fields) != 2 || len(fields[0]) < 2 || (fields[0][0] != 'D' && fields[0][0] != 'd') {
			return entry, fmt.Errorf("line %d: bad depth spec %q", lineNo, strings.TrimSpace(spec))
		}
		depth, err := strconv.Atoi(fields[0][1:])
		if err != nil {
			return entry, fmt.Errorf("line %d: bad depth %q", lineNo, fields[0])
		}
		nodes, err := strconv.ParseUint(fields[1], 10, 64)
		if err != nil {
			return entry, fmt.Errorf("line %d: bad node count %q", lineNo, fields[1])
		}
		entry.Nodes[depth] = nodes
	}
	if len(entry.Nodes) == 0 {
		return entry, fmt.Errorf("line %d: no depths", lineNo)
	}
	return entry, nil
}
