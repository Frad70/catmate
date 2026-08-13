package board

// Zobrist keys. Generated from a fixed seed at init so that two builds of the
// engine agree on every hash — an opening book, a transposition table dump or
// a regression test would otherwise be worthless across binaries.

var (
	zobristPiece  [13][64]uint64 // indexed by Piece, NoPiece included and left zero
	zobristSide   uint64
	zobristCastle [16]uint64 // indexed by the full CastleRights bitmask
	zobristEPFile [8]uint64
)

// splitmix64 is a tiny, well-distributed generator. It needs no state beyond a
// counter, which makes the key table trivially reproducible.
type splitmix64 uint64

func (s *splitmix64) next() uint64 {
	*s += 0x9E3779B97F4A7C15
	z := uint64(*s)
	z = (z ^ (z >> 30)) * 0xBF58476D1CE4E5B9
	z = (z ^ (z >> 27)) * 0x94D049BB133111EB
	return z ^ (z >> 31)
}

func init() {
	rng := splitmix64(0xC0FFEE1CEDCAFE)

	for pc := Piece(0); pc < NoPiece; pc++ {
		for sq := 0; sq < 64; sq++ {
			zobristPiece[pc][sq] = rng.next()
		}
	}
	zobristSide = rng.next()
	for i := range zobristCastle {
		zobristCastle[i] = rng.next()
	}
	for i := range zobristEPFile {
		zobristEPFile[i] = rng.next()
	}
}

// computeKey hashes the position from scratch. The incrementally maintained
// Key() must always equal this; that equality is gate G1.
func (p *Position) computeKey() uint64 {
	var k uint64
	for sq := 0; sq < 64; sq++ {
		if pc := p.mailbox[sq]; pc != NoPiece {
			k ^= zobristPiece[pc][sq]
		}
	}
	if p.side == Black {
		k ^= zobristSide
	}
	k ^= zobristCastle[p.castle]
	if p.ep != SquareNone {
		k ^= zobristEPFile[p.ep.File()]
	}
	return k
}

// ComputeKey exposes the from-scratch hash for tests and for verifying a
// position built by hand.
func (p *Position) ComputeKey() uint64 { return p.computeKey() }
