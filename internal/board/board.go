// Package board holds the position: what stands where, whose turn it is, and
// everything else a FEN string carries.
package board

import (
	"fmt"
	"strconv"
	"strings"
)

type Color uint8

const (
	White Color = 0
	Black Color = 1
)

// Opposite flips the side. Colors are 0 and 1 so a xor does it.
func (c Color) Opposite() Color { return c ^ 1 }

type PieceType uint8

const (
	Pawn PieceType = iota
	Knight
	Bishop
	Rook
	Queen
	King
	NoPieceType
)

// Piece packs color and type into one byte: color*6 + type.
type Piece uint8

const NoPiece Piece = 12

func MakePiece(c Color, pt PieceType) Piece { return Piece(uint8(c)*6 + uint8(pt)) }

func (p Piece) Color() Color { return Color(uint8(p) / 6) }

func (p Piece) Type() PieceType {
	if p == NoPiece {
		return NoPieceType
	}
	return PieceType(uint8(p) % 6)
}

const pieceChars = "PNBRQKpnbrqk"

func (p Piece) Char() byte {
	if p == NoPiece {
		return '.'
	}
	return pieceChars[p]
}

func pieceFromChar(c byte) (Piece, bool) {
	i := strings.IndexByte(pieceChars, c)
	if i < 0 {
		return NoPiece, false
	}
	return Piece(i), true
}

// Square is 0..63 with a1 = 0 and h8 = 63, so rank = sq/8 and file = sq%8.
type Square int8

const SquareNone Square = -1

func MakeSquare(file, rank int) Square { return Square(rank*8 + file) }

func (s Square) File() int { return int(s) % 8 }
func (s Square) Rank() int { return int(s) / 8 }

func (s Square) String() string {
	if s < 0 || s > 63 {
		return "-"
	}
	return string([]byte{byte('a' + s.File()), byte('1' + s.Rank())})
}

func ParseSquare(s string) (Square, error) {
	if len(s) != 2 || s[0] < 'a' || s[0] > 'h' || s[1] < '1' || s[1] > '8' {
		return SquareNone, fmt.Errorf("bad square %q", s)
	}
	return MakeSquare(int(s[0]-'a'), int(s[1]-'1')), nil
}

type Bitboard uint64

func (b Bitboard) Has(s Square) bool { return b&(1<<uint(s)) != 0 }

type CastleRights uint8

const (
	WhiteKingside CastleRights = 1 << iota
	WhiteQueenside
	BlackKingside
	BlackQueenside
)

// Position is a full chess position: bitboards for fast set operations plus a
// mailbox, because "what stands on this square" is asked far more often than
// any set operation.
type Position struct {
	byType  [6]Bitboard
	byColor [2]Bitboard
	mailbox [64]Piece

	Side     Color
	Castle   CastleRights
	EP       Square // target square behind a double push, or SquareNone
	Halfmove int
	Fullmove int
}

const StartFEN = "rnbqkbnr/pppppppp/8/8/8/8/PPPPPPPP/RNBQKBNR w KQkq - 0 1"

func NewPosition() *Position {
	p := &Position{}
	p.clear()
	return p
}

func (p *Position) clear() {
	p.byType = [6]Bitboard{}
	p.byColor = [2]Bitboard{}
	for i := range p.mailbox {
		p.mailbox[i] = NoPiece
	}
	p.Side = White
	p.Castle = 0
	p.EP = SquareNone
	p.Halfmove = 0
	p.Fullmove = 1
}

func (p *Position) PieceAt(s Square) Piece { return p.mailbox[s] }

func (p *Position) Pieces(pt PieceType) Bitboard { return p.byType[pt] }
func (p *Position) Occupied(c Color) Bitboard    { return p.byColor[c] }
func (p *Position) OccupiedAll() Bitboard        { return p.byColor[White] | p.byColor[Black] }
func (p *Position) PiecesOf(c Color, pt PieceType) Bitboard {
	return p.byType[pt] & p.byColor[c]
}

func (p *Position) Put(s Square, pc Piece) {
	bit := Bitboard(1) << uint(s)
	p.mailbox[s] = pc
	p.byType[pc.Type()] |= bit
	p.byColor[pc.Color()] |= bit
}

func (p *Position) Remove(s Square) {
	pc := p.mailbox[s]
	if pc == NoPiece {
		return
	}
	bit := Bitboard(1) << uint(s)
	p.mailbox[s] = NoPiece
	p.byType[pc.Type()] &^= bit
	p.byColor[pc.Color()] &^= bit
}

// ParseFEN reads a FEN string. Halfmove and fullmove counters may be omitted;
// everything before them may not.
func ParseFEN(fen string) (*Position, error) {
	fields := strings.Fields(fen)
	if len(fields) < 4 {
		return nil, fmt.Errorf("fen has %d fields, need at least 4: %q", len(fields), fen)
	}

	p := NewPosition()

	if err := p.parsePlacement(fields[0]); err != nil {
		return nil, err
	}

	switch fields[1] {
	case "w":
		p.Side = White
	case "b":
		p.Side = Black
	default:
		return nil, fmt.Errorf("bad side to move %q", fields[1])
	}

	if err := p.parseCastling(fields[2]); err != nil {
		return nil, err
	}

	if fields[3] != "-" {
		sq, err := ParseSquare(fields[3])
		if err != nil {
			return nil, fmt.Errorf("bad en passant square: %w", err)
		}
		p.EP = sq
	}

	if len(fields) > 4 {
		n, err := strconv.Atoi(fields[4])
		if err != nil || n < 0 {
			return nil, fmt.Errorf("bad halfmove clock %q", fields[4])
		}
		p.Halfmove = n
	}
	if len(fields) > 5 {
		n, err := strconv.Atoi(fields[5])
		if err != nil || n < 1 {
			return nil, fmt.Errorf("bad fullmove number %q", fields[5])
		}
		p.Fullmove = n
	}

	return p, nil
}

func (p *Position) parsePlacement(placement string) error {
	ranks := strings.Split(placement, "/")
	if len(ranks) != 8 {
		return fmt.Errorf("placement has %d ranks, need 8", len(ranks))
	}
	// FEN lists rank 8 first; our squares count up from rank 1.
	for i, row := range ranks {
		rank := 7 - i
		file := 0
		for j := 0; j < len(row); j++ {
			c := row[j]
			switch {
			case c >= '1' && c <= '8':
				file += int(c - '0')
			default:
				pc, ok := pieceFromChar(c)
				if !ok {
					return fmt.Errorf("bad piece %q in rank %d", string(c), rank+1)
				}
				if file > 7 {
					return fmt.Errorf("rank %d overflows", rank+1)
				}
				p.Put(MakeSquare(file, rank), pc)
				file++
			}
		}
		if file != 8 {
			return fmt.Errorf("rank %d covers %d files, need 8", rank+1, file)
		}
	}
	return nil
}

func (p *Position) parseCastling(s string) error {
	if s == "-" {
		return nil
	}
	for i := 0; i < len(s); i++ {
		switch s[i] {
		case 'K':
			p.Castle |= WhiteKingside
		case 'Q':
			p.Castle |= WhiteQueenside
		case 'k':
			p.Castle |= BlackKingside
		case 'q':
			p.Castle |= BlackQueenside
		default:
			return fmt.Errorf("bad castling rights %q", s)
		}
	}
	return nil
}

// FEN renders the position back out. ParseFEN(FEN(p)) must reproduce p, and
// FEN(ParseFEN(s)) must reproduce s for any canonical s.
func (p *Position) FEN() string {
	var sb strings.Builder

	for rank := 7; rank >= 0; rank-- {
		empty := 0
		for file := 0; file < 8; file++ {
			pc := p.mailbox[MakeSquare(file, rank)]
			if pc == NoPiece {
				empty++
				continue
			}
			if empty > 0 {
				sb.WriteByte(byte('0' + empty))
				empty = 0
			}
			sb.WriteByte(pc.Char())
		}
		if empty > 0 {
			sb.WriteByte(byte('0' + empty))
		}
		if rank > 0 {
			sb.WriteByte('/')
		}
	}

	if p.Side == White {
		sb.WriteString(" w ")
	} else {
		sb.WriteString(" b ")
	}

	sb.WriteString(p.castlingString())
	sb.WriteByte(' ')
	sb.WriteString(p.EP.String())
	sb.WriteByte(' ')
	sb.WriteString(strconv.Itoa(p.Halfmove))
	sb.WriteByte(' ')
	sb.WriteString(strconv.Itoa(p.Fullmove))

	return sb.String()
}

func (p *Position) castlingString() string {
	if p.Castle == 0 {
		return "-"
	}
	var sb strings.Builder
	for _, r := range []struct {
		right CastleRights
		char  byte
	}{
		{WhiteKingside, 'K'},
		{WhiteQueenside, 'Q'},
		{BlackKingside, 'k'},
		{BlackQueenside, 'q'},
	} {
		if p.Castle&r.right != 0 {
			sb.WriteByte(r.char)
		}
	}
	return sb.String()
}
