package grid

import (
	"math/bits"
	"unsafe"

	. "aago/internal/pyrt"
)

// BoolGrid operations eight cells at a time. A Go bool is one byte holding 0 or 1, so a grid is 1659 bytes
// of 0/1, and one uint64 holds eight cells: & | ^ on words are the cell-wise and/or/xor, a popcount counts
// true cells. The results are exactly those of the cell-by-cell loops (see bits_test.go).

const (
	cells  = SY * SX
	nWords = cells / 8 // full words; the last cells%8 cells are done one by one
	ones   = 0x0101010101010101
)

func cellBytes(g *BoolGrid) *[cells]byte { return (*[cells]byte)(unsafe.Pointer(g)) }

func loadWord(b *[cells]byte, i int) uint64     { return *(*uint64)(unsafe.Pointer(&b[i*8])) }
func storeWord(b *[cells]byte, i int, v uint64) { *(*uint64)(unsafe.Pointer(&b[i*8])) = v }

// ---- BoolGrid ops (value semantics: results are new grids)

func (g *BoolGrid) Any() bool {
	b := cellBytes(g)
	var acc uint64
	for i := 0; i < nWords; i++ {
		acc |= loadWord(b, i)
	}
	for i := nWords * 8; i < cells; i++ {
		acc |= uint64(b[i])
	}
	return acc != 0
}

func (g *BoolGrid) All() bool {
	b := cellBytes(g)
	acc := uint64(ones)
	for i := 0; i < nWords; i++ {
		acc &= loadWord(b, i)
	}
	for i := nWords * 8; i < cells; i++ {
		acc &= uint64(b[i]) * ones
	}
	return acc == ones
}

func (g *BoolGrid) Count() int {
	b := cellBytes(g)
	n := 0
	for i := 0; i < nWords; i++ {
		n += bits.OnesCount64(loadWord(b, i))
	}
	for i := nWords * 8; i < cells; i++ {
		n += int(b[i])
	}
	return n
}

// Nonzero mirrors zip(*mask.nonzero()) (row-major order).
func (g *BoolGrid) Nonzero() []Pos {
	n := g.Count()
	if n == 0 {
		return nil
	}
	r := make([]Pos, 0, n)
	b := cellBytes(g)
	for i := 0; i < nWords; i++ {
		for w := loadWord(b, i); w != 0; w &= w - 1 {
			c := i*8 + bits.TrailingZeros64(w)>>3
			r = append(r, Pos{c / SX, c % SX})
		}
	}
	for c := nWords * 8; c < cells; c++ {
		if b[c] != 0 {
			r = append(r, Pos{c / SX, c % SX})
		}
	}
	return r
}

// And is numpy `g & o`.
func (g BoolGrid) And(o BoolGrid) BoolGrid {
	boolOp(cellBytes(&g), cellBytes(&o), opAnd)
	return g
}

// Or is numpy `g | o`.
func (g BoolGrid) Or(o BoolGrid) BoolGrid {
	boolOp(cellBytes(&g), cellBytes(&o), opOr)
	return g
}

// AndNot is numpy `g & ~o`.
func (g BoolGrid) AndNot(o BoolGrid) BoolGrid {
	boolOp(cellBytes(&g), cellBytes(&o), opAndNot)
	return g
}

// Not is numpy `~g`.
func (g BoolGrid) Not() BoolGrid {
	a := cellBytes(&g)
	boolOp(a, a, opNot)
	return g
}

type boolOpKind int

const (
	opAnd boolOpKind = iota
	opOr
	opAndNot
	opNot // a = ~a (b unused)
)

// boolOp is a = a <op> b cell by cell (SIMD: vec_simd.go).
func boolOp(a, b *[cells]byte, op boolOpKind) {
	if SIMDOn() {
		boolOpSIMD(a, b, op)
	} else {
		boolOpWords(a, b, op, 0)
	}
}

// boolOpWords does boolOp from cell `from` (a multiple of 8) eight cells per word.
func boolOpWords(a, b *[cells]byte, op boolOpKind, from int) {
	for i := from / 8; i < nWords; i++ {
		x, y := loadWord(a, i), loadWord(b, i)
		switch op {
		case opAnd:
			x &= y
		case opOr:
			x |= y
		case opAndNot:
			x &^= y
		case opNot:
			x ^= ones
		}
		storeWord(a, i, x)
	}
	for i := nWords * 8; i < cells; i++ {
		switch op {
		case opAnd:
			a[i] &= b[i]
		case opOr:
			a[i] |= b[i]
		case opAndNot:
			a[i] &^= b[i]
		case opNot:
			a[i] ^= 1
		}
	}
}

// First is the first true cell in row-major order: numpy `list(zip(*g.nonzero()))[0]`.
// Raises IndexError when the grid has no true cell.
func (g *BoolGrid) First() Pos {
	b := cellBytes(g)
	for i := 0; i < nWords; i++ {
		if w := loadWord(b, i); w != 0 {
			c := i*8 + bits.TrailingZeros64(w)>>3
			return Pos{c / SX, c % SX}
		}
	}
	for c := nWords * 8; c < cells; c++ {
		if b[c] != 0 {
			return Pos{c / SX, c % SX}
		}
	}
	Raise("IndexError", "list index out of range")
	return Pos{}
}

// ---- neighbour counts

// padRow is a grid row with a zero cell on each side and room for a word load past the end.
type padRow [SX + 10]uint8

// padded copies m into a zero-bordered byte grid (a bool is a 0/1 byte, so a row is one copy).
func padded(m *BoolGrid) (p [SY + 2]padRow) {
	for y := 0; y < SY; y++ {
		copy(p[y+1][1:SX+1], (*[SX]uint8)(unsafe.Pointer(&m[y]))[:])
	}
	return
}

// at loads the eight cells r[i..i+7]; each byte is 0 or 1, so sums of up to eight loads stay per byte.
func at(r *padRow, i int) uint64 { return *(*uint64)(unsafe.Pointer(&r[i])) }

// spread writes the eight per-byte counts of s to o[x..], as far as the row goes.
func spread(o *[SX]int32, x int, s uint64) {
	for k := 0; k < 8 && x+k < SX; k++ {
		o[x+k] = int32(s >> (8 * k) & 0xff)
	}
}

// NeighborCount8 sets out[y][x] to the number of true cells among the 8 neighbours of (y, x).
func NeighborCount8(m *BoolGrid, out *I32Grid) {
	p := padded(m)
	for y := 0; y < SY; y++ {
		up, mid, dn := &p[y], &p[y+1], &p[y+2]
		for x := 0; x < SX; x += 8 {
			spread(&out[y], x, at(up, x)+at(up, x+1)+at(up, x+2)+at(mid, x)+at(mid, x+2)+at(dn, x)+at(dn, x+1)+at(dn, x+2))
		}
	}
}

// NeighborCount4 sets out[y][x] to the number of true cells among the 4 orthogonal neighbours of (y, x).
func NeighborCount4(m *BoolGrid, out *I32Grid) {
	p := padded(m)
	for y := 0; y < SY; y++ {
		up, mid, dn := &p[y], &p[y+1], &p[y+2]
		for x := 0; x < SX; x += 8 {
			spread(&out[y], x, at(up, x+1)+at(mid, x)+at(mid, x+2)+at(dn, x+1))
		}
	}
}

// storeRow writes the eight 0/1 bytes of v to out[y][x..], as far as the row goes.
func storeRow(out *BoolGrid, y, x int, v uint64) {
	b := cellBytes(out)
	if x+8 <= SX {
		*(*uint64)(unsafe.Pointer(&b[y*SX+x])) = v
		return
	}
	for k := 0; x+k < SX; k++ {
		b[y*SX+x+k] = byte(v >> (8 * k))
	}
}

// NeighborAny8 is true where some of the 8 neighbours is true (numpy: the OR of m translated in 8 directions).
func NeighborAny8(m *BoolGrid) (out BoolGrid) {
	p := padded(m)
	for y := 0; y < SY; y++ {
		up, mid, dn := &p[y], &p[y+1], &p[y+2]
		for x := 0; x < SX; x += 8 {
			storeRow(&out, y, x, at(up, x)|at(up, x+1)|at(up, x+2)|at(mid, x)|at(mid, x+2)|at(dn, x)|at(dn, x+1)|at(dn, x+2))
		}
	}
	return
}

// NeighborAny4 is true where some of the 4 orthogonal neighbours is true.
func NeighborAny4(m *BoolGrid) (out BoolGrid) {
	p := padded(m)
	for y := 0; y < SY; y++ {
		up, mid, dn := &p[y], &p[y+1], &p[y+2]
		for x := 0; x < SX; x += 8 {
			storeRow(&out, y, x, at(up, x+1)|at(mid, x)|at(mid, x+2)|at(dn, x+1))
		}
	}
	return
}
