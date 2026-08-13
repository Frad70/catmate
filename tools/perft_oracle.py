#!/usr/bin/env python3
"""
catmate perft oracle — the unfakeable truth about mice.

Ground truth is produced by python-chess (an independent, well-tested
implementation), never by catmate itself. This file is the ONLY sanctioned
producer of testdata/*.epd.

Usage:
    perft_oracle.py perft  "<FEN>" <depth>
    perft_oracle.py divide "<FEN>" <depth>
    perft_oracle.py gen --out testdata/perft.epd --count 200 --seed 1 [--budget 400000]
    perft_oracle.py check testdata/perft.epd --count 5 --seed 7

EPD line format (one position per line):
    <FEN> ;D1 <nodes> ;D2 <nodes> ... ;Dn <nodes>
"""
import argparse
import random
import sys

import chess


def perft(board: chess.Board, depth: int) -> int:
    if depth == 0:
        return 1
    if depth == 1:
        return board.legal_moves.count()
    total = 0
    for move in board.legal_moves:
        board.push(move)
        total += perft(board, depth - 1)
        board.pop()
    return total


def divide(board: chess.Board, depth: int):
    out = []
    for move in board.legal_moves:
        board.push(move)
        out.append((move.uci(), perft(board, depth - 1)))
        board.pop()
    out.sort()
    return out


# Classic positions from the chess programming literature. Their node counts
# here are COMPUTED by python-chess, not recalled from memory.
CLASSICS = [
    ("startpos", "rnbqkbnr/pppppppp/8/8/8/8/PPPPPPPP/RNBQKBNR w KQkq - 0 1", 5),
    ("kiwipete", "r3k2r/p1ppqpb1/bn2pnp1/3PN3/1p2P3/2N2Q1p/PPPBBPPP/R3K2R w KQkq - 0 1", 4),
    ("pos3-endgame", "8/2p5/3p4/KP5r/1R3p1k/8/4P1P1/8 w - - 0 1", 5),
    ("pos4-promos", "r3k2r/Pppp1ppp/1b3nbN/nP6/BBP1P3/q4N2/Pp1P2PP/R2Q1RK1 w kq - 0 1", 4),
    ("pos4-mirror", "r2q1rk1/pP1p2pp/Q4n2/bbp1p3/Np6/1B3NBn/pPPP1PPP/R3K2R b KQ - 0 1", 4),
    ("pos5-talkchess", "rnbq1k1r/pp1Pbppp/2p5/8/2B5/8/PPP1NnPP/RNBQK2R w KQ - 1 8", 4),
    ("pos6-steven", "r4rk1/1pp1qppp/p1np1n2/2b1p1B1/2B1P1b1/P1NP1N2/1PP1QPPP/R4RK1 w - - 0 10", 4),
]


def epd_line(board: chess.Board, max_depth: int, budget: int) -> str:
    """Compute D1..Dn, stopping when the node budget is exhausted."""
    parts = [board.fen()]
    spent = 0
    for d in range(1, max_depth + 1):
        n = perft(board, d)
        parts.append(f";D{d} {n}")
        spent += n
        if spent >= budget:
            break
    return " ".join(parts)


def random_position(rng: random.Random) -> chess.Board:
    """Random-walk into a live position; retry if the game ended."""
    for _ in range(60):
        board = chess.Board()
        plies = rng.randint(1, 60)
        for _ in range(plies):
            moves = list(board.legal_moves)
            if not moves:
                break
            board.push(rng.choice(moves))
        if board.legal_moves.count() > 0 and not board.is_game_over(claim_draw=False):
            return board
    return chess.Board()


def cmd_check(args) -> int:
    """Re-verify random lines of an EPD file against the oracle.

    This is the anti-tamper check: if a node count was ever hand-edited to make
    the engine look right, a sampled re-verification catches it.
    """
    rng = random.Random(args.seed)
    with open(args.file) as fh:
        lines = [ln.strip() for ln in fh if ln.strip()]
    if not lines:
        print("corpus is empty", file=sys.stderr)
        return 1
    sample = rng.sample(lines, min(args.count, len(lines)))
    bad = 0
    for line in sample:
        fen, *depths = [part.strip() for part in line.split(";")]
        board = chess.Board(fen)
        for spec in depths:
            tag, claimed = spec.split()
            d = int(tag[1:])
            actual = perft(board, d)
            if actual != int(claimed):
                bad += 1
                print(f"TAMPERED {fen} D{d}: file says {claimed}, oracle says {actual}")
    print(f"checked {len(sample)} positions, tampered: {bad}")
    return 1 if bad else 0


def cmd_gen(args) -> int:
    rng = random.Random(args.seed)
    lines = []
    if args.classics:
        for name, fen, depth in CLASSICS:
            b = chess.Board(fen)
            lines.append(epd_line(b, depth, args.budget))
            print(f"  classic {name}", file=sys.stderr)
    seen = set()
    made = 0
    while made < args.count:
        b = random_position(rng)
        key = b.fen()
        if key in seen:
            continue
        seen.add(key)
        lines.append(epd_line(b, args.max_depth, args.budget))
        made += 1
        if made % 20 == 0:
            print(f"  random {made}/{args.count}", file=sys.stderr)
    mode = "a" if args.append else "w"
    with open(args.out, mode) as fh:
        for line in lines:
            fh.write(line + "\n")
    print(f"wrote {len(lines)} positions to {args.out} (mode={mode})", file=sys.stderr)
    return 0


def main() -> int:
    ap = argparse.ArgumentParser(description=__doc__)
    sub = ap.add_subparsers(dest="cmd", required=True)

    p = sub.add_parser("perft")
    p.add_argument("fen")
    p.add_argument("depth", type=int)

    d = sub.add_parser("divide")
    d.add_argument("fen")
    d.add_argument("depth", type=int)

    g = sub.add_parser("gen")
    g.add_argument("--out", required=True)
    g.add_argument("--count", type=int, default=100)
    g.add_argument("--seed", type=int, default=1)
    g.add_argument("--max-depth", type=int, default=5)
    g.add_argument("--budget", type=int, default=400000)
    g.add_argument("--append", action="store_true")
    g.add_argument("--classics", action="store_true")
    g.set_defaults(func=cmd_gen)

    c = sub.add_parser("check")
    c.add_argument("file")
    c.add_argument("--count", type=int, default=5)
    c.add_argument("--seed", type=int, default=7)
    c.set_defaults(func=cmd_check)

    args = ap.parse_args()
    if args.cmd == "perft":
        print(perft(chess.Board(args.fen), args.depth))
        return 0
    if args.cmd == "divide":
        board = chess.Board(args.fen)
        total = 0
        for mv, n in divide(board, args.depth):
            print(f"{mv} {n}")
            total += n
        print(f"total {total}")
        return 0
    return args.func(args)


if __name__ == "__main__":
    raise SystemExit(main())
