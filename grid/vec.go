package grid

import (
	"math"
	"unsafe"
)

// Bulk operations over whole grids, numpy style. Each one has a plain loop here and a SIMD version
// (vec_simd.go: AVX2, compiled only with GOEXPERIMENT=simd for amd64). The SIMD version runs when the CPU has
// AVX2 (SIMDOn); it gives exactly the same bits as the plain loop, which vec_test.go checks on random grids.
// GODEBUG=cpu.avx2=off makes a SIMD build use the plain loops.

// Range16 is the inclusive range Lo..Hi of int16 values. A range with Lo > Hi is empty.
type Range16 struct{ Lo, Hi int16 }

// NoRange16 matches no value.
func NoRange16() Range16 { return Range16{1, 0} }

// InRanges marks the cells whose value lies in one of the ranges (numpy isin for a set of runs of values).
func InRanges(g *I16Grid, rs []Range16) (out BoolGrid) {
	if !SIMDOn() || !simdRanges(rs) {
		inRangesPlain(g, rs, &out)
		return
	}
	for i := 0; i == 0 || i < len(rs); i += 4 { // up to 4 ranges per pass, the passes OR-ed
		r := quad(rs[i:])
		if len(rs)-i <= 2 {
			inRanges2SIMD(g, r[0], r[1], &out, i > 0)
		} else {
			inRanges4SIMD(g, r, &out, i > 0)
		}
	}
	return
}

// AnyInRanges reports whether some cell's value lies in one of the ranges.
func AnyInRanges(g *I16Grid, rs []Range16) bool {
	if !SIMDOn() || !simdRanges(rs) {
		return anyInRangesPlain(g, rs)
	}
	for i := 0; i < len(rs); i += 4 {
		r := quad(rs[i:])
		if len(rs)-i <= 2 && anyInRanges2SIMD(g, r[0], r[1]) || len(rs)-i > 2 && anyInRanges4SIMD(g, r) {
			return true
		}
	}
	return false
}

// quad is the first 4 ranges of rs, padded with empty ones.
func quad(rs []Range16) (q [4]Range16) {
	for i := range q {
		q[i] = NoRange16()
		if i < len(rs) {
			q[i] = rs[i]
		}
	}
	return
}

// simdRanges: the vector code compares v > Lo-1 and Hi+1 > v, which needs Lo-1 and Hi+1 to fit in int16.
func simdRanges(rs []Range16) bool {
	for _, r := range rs {
		if r.Lo == math.MinInt16 || r.Hi == math.MaxInt16 {
			return false
		}
	}
	return true
}

func inRange(v int16, r Range16) bool { return v >= r.Lo && v <= r.Hi }

func inAny(v int16, rs []Range16) bool {
	for _, r := range rs {
		if inRange(v, r) {
			return true
		}
	}
	return false
}

func inRangesPlain(g *I16Grid, rs []Range16, out *BoolGrid) {
	src, dst := i16Cells(g), cellBytes(out)
	for i := range src {
		dst[i] = b2u8(inAny(src[i], rs))
	}
}

func anyInRangesPlain(g *I16Grid, rs []Range16) bool {
	for _, v := range i16Cells(g) {
		if inAny(v, rs) {
			return true
		}
	}
	return false
}

// ---- int16 grids

// Eq is numpy `g == v`.
func (g *I16Grid) Eq(v int16) BoolGrid { return InRanges(g, []Range16{{v, v}}) }

// SetWhere sets the cells where m is set to v (numpy g[m] = v).
func (g *I16Grid) SetWhere(m *BoolGrid, v int16) {
	var src I16Grid
	if SIMDOn() {
		selectI16SIMD(g, &src, m, v, true)
	} else {
		selectI16Plain(g, &src, m, v, true)
	}
}

// CopyWhere copies the cells of src where m is set (numpy g[m] = src[m]).
func (g *I16Grid) CopyWhere(src *I16Grid, m *BoolGrid) {
	if SIMDOn() {
		selectI16SIMD(g, src, m, 0, false)
	} else {
		selectI16Plain(g, src, m, 0, false)
	}
}

// selectI16Plain: g = m ? (constant ? v : src) : g.
func selectI16Plain(g, src *I16Grid, m *BoolGrid, v int16, constant bool) {
	r, s, b := i16Cells(g), i16Cells(src), cellBytes(m)
	for i := range r {
		if b[i] != 0 {
			if constant {
				r[i] = v
			} else {
				r[i] = s[i]
			}
		}
	}
}

// ---- int32 grids compared with a number

type cmpOp int

const (
	cmpEq cmpOp = iota // g == v
	cmpGt              // g > v
	cmpLt              // g < v
)

// cmpI32 is g <op> v, or its negation when not is set (!=, <=, >=).
func cmpI32(g *I32Grid, v int32, op cmpOp, not bool) (out BoolGrid) {
	if SIMDOn() {
		cmpI32SIMD(g, v, op, not, &out)
	} else {
		cmpI32Plain(g, v, op, not, &out)
	}
	return
}

func cmpI32Plain(g *I32Grid, v int32, op cmpOp, not bool, out *BoolGrid) {
	src, dst := i32Cells(g), cellBytes(out)
	for i, c := range src {
		var r bool
		switch op {
		case cmpEq:
			r = c == v
		case cmpGt:
			r = c > v
		case cmpLt:
			r = c < v
		}
		dst[i] = b2u8(r != not)
	}
}

func (g *I32Grid) Eq(v int32) BoolGrid { return cmpI32(g, v, cmpEq, false) }
func (g *I32Grid) Ne(v int32) BoolGrid { return cmpI32(g, v, cmpEq, true) }
func (g *I32Grid) Gt(v int32) BoolGrid { return cmpI32(g, v, cmpGt, false) }
func (g *I32Grid) Le(v int32) BoolGrid { return cmpI32(g, v, cmpGt, true) }
func (g *I32Grid) Lt(v int32) BoolGrid { return cmpI32(g, v, cmpLt, false) }
func (g *I32Grid) Ge(v int32) BoolGrid { return cmpI32(g, v, cmpLt, true) }

// SetWhere sets the cells where m is set to v (numpy g[m] = v).
func (g *I32Grid) SetWhere(m *BoolGrid, v int32) {
	if SIMDOn() {
		setWhereI32SIMD(g, m, v)
	} else {
		setWhereI32Plain(g, m, v)
	}
}

func setWhereI32Plain(g *I32Grid, m *BoolGrid, v int32) {
	r, b := i32Cells(g), cellBytes(m)
	for i := range r {
		if b[i] != 0 {
			r[i] = v
		}
	}
}

// Mul is the cell-wise product (wrapping like numpy int32).
func (g *I32Grid) Mul(o *I32Grid) (out I32Grid) {
	if SIMDOn() {
		mulI32SIMD(g, o, &out)
	} else {
		mulI32Plain(g, o, &out)
	}
	return
}

func mulI32Plain(g, o, out *I32Grid) {
	a, b, r := i32Cells(g), i32Cells(o), i32Cells(out)
	for i := range r {
		r[i] = a[i] * b[i]
	}
}

// MulScalar is g * c (wrapping like numpy int32).
func (g *I32Grid) MulScalar(c int32) (out I32Grid) {
	if SIMDOn() {
		mulScalarI32SIMD(g, c, &out)
	} else {
		mulScalarI32Plain(g, c, &out)
	}
	return
}

func mulScalarI32Plain(g *I32Grid, c int32, out *I32Grid) {
	a, r := i32Cells(g), i32Cells(out)
	for i := range r {
		r[i] = a[i] * c
	}
}

// AppendPositive appends the values > 0 in row-major order (numpy g[g > 0]).
func (g *I32Grid) AppendPositive(dst []int32) []int32 {
	if SIMDOn() {
		return appendPositiveSIMD(g, dst)
	}
	return appendPositivePlain(g, dst)
}

func appendPositivePlain(g *I32Grid, dst []int32) []int32 {
	for _, v := range i32Cells(g) {
		if v > 0 {
			dst = append(dst, v)
		}
	}
	return dst
}

// ---- float32 grids

// FullF32 is a grid with every cell set to v (numpy full).
func FullF32(v float32) (out F32Grid) {
	if SIMDOn() {
		fullF32SIMD(v, &out)
	} else {
		fullF32Plain(v, &out)
	}
	return
}

func fullF32Plain(v float32, out *F32Grid) {
	r := f32Cells(out)
	for i := range r {
		r[i] = v
	}
}

// SubI32 is numpy's in-place g -= v for a float32 g and an int32 v: the difference is computed in float64
// and rounded to float32 once.
func (g *F32Grid) SubI32(v *I32Grid) {
	if SIMDOn() {
		subI32SIMD(g, v)
	} else {
		subI32Plain(g, v)
	}
}

func subI32Plain(g *F32Grid, v *I32Grid) {
	r, s := f32Cells(g), i32Cells(v)
	for i := range r {
		r[i] = float32(float64(r[i]) - float64(s[i]))
	}
}

// AddWhere adds c to the cells where m is set. This is numpy's g += m * c: adding 0 elsewhere changes no bits
// (the grids this is used on hold no -0), and a float32 sum rounded through float64 is the float32 sum.
func (g *F32Grid) AddWhere(m *BoolGrid, c float32) {
	if SIMDOn() {
		addWhereSIMD(g, m, c)
	} else {
		addWherePlain(g, m, c)
	}
}

func addWherePlain(g *F32Grid, m *BoolGrid, c float32) {
	r, b := f32Cells(g), cellBytes(m)
	for i := range r {
		if b[i] != 0 {
			r[i] += c
		}
	}
}

// SetWhere sets the cells where m is set to c (numpy g[m] = c).
func (g *F32Grid) SetWhere(m *BoolGrid, c float32) {
	if SIMDOn() {
		setWhereSIMD(g, m, c)
	} else {
		setWherePlain(g, m, c)
	}
}

func setWherePlain(g *F32Grid, m *BoolGrid, c float32) {
	r, b := f32Cells(g), cellBytes(m)
	for i := range r {
		if b[i] != 0 {
			r[i] = c
		}
	}
}

// GreaterEqual is g >= v with v compared exactly (numpy compares a float32 array with a Python number in
// float64). NaN cells are false.
func (g *F32Grid) GreaterEqual(v float64) (out BoolGrid) {
	if SIMDOn() {
		geF32SIMD(g, v, &out)
	} else {
		geF32Plain(g, v, &out)
	}
	return
}

func geF32Plain(g *F32Grid, v float64, out *BoolGrid) {
	r, dst := f32Cells(g), cellBytes(out)
	for i := range r {
		dst[i] = b2u8(float64(r[i]) >= v)
	}
}

// Less is g < v with v compared exactly (like GreaterEqual). NaN cells are false.
func (g *F32Grid) Less(v float64) (out BoolGrid) {
	if SIMDOn() {
		ltF32SIMD(g, v, &out)
	} else {
		ltF32Plain(g, v, &out)
	}
	return
}

func ltF32Plain(g *F32Grid, v float64, out *BoolGrid) {
	r, dst := f32Cells(g), cellBytes(out)
	for i := range r {
		dst[i] = b2u8(float64(r[i]) < v)
	}
}

// Eq is g == v (NaN is equal to nothing, -0 == +0).
func (g *F32Grid) Eq(v float32) (out BoolGrid) {
	if SIMDOn() {
		eqF32SIMD(g, v, &out)
	} else {
		eqF32Plain(g, v, &out)
	}
	return
}

func eqF32Plain(g *F32Grid, v float32, out *BoolGrid) {
	r, dst := f32Cells(g), cellBytes(out)
	for i := range r {
		dst[i] = b2u8(r[i] == v)
	}
}

// IsFinite is numpy isfinite: neither infinite nor NaN.
func (g *F32Grid) IsFinite() (out BoolGrid) {
	if SIMDOn() {
		isFiniteF32SIMD(g, &out)
	} else {
		isFiniteF32Plain(g, &out)
	}
	return
}

func isFiniteF32Plain(g *F32Grid, out *BoolGrid) {
	r, dst := f32Cells(g), cellBytes(out)
	for i := range r {
		f := float64(r[i])
		dst[i] = b2u8(!math.IsInf(f, 0) && !math.IsNaN(f))
	}
}

// MaxWhere is the largest value where m is set, compared with > in row-major order (numpy g[m].max() when
// there is no NaN); ok is false when m is empty.
func (g *F32Grid) MaxWhere(m *BoolGrid) (max float32, ok bool) {
	r, b := f32Cells(g), cellBytes(m)
	for i := range r {
		if b[i] != 0 && (!ok || r[i] > max) {
			max, ok = r[i], true
		}
	}
	return
}

// f32Threshold is the smallest float32 t with float64(t) >= v, so that float64(x) >= v is x >= t for every
// float32 x. ok is false when v is NaN (nothing is >= NaN).
func f32Threshold(v float64) (t float32, ok bool) {
	if math.IsNaN(v) {
		return 0, false
	}
	t = float32(v)
	if float64(t) < v {
		t = math.Nextafter32(t, float32(math.Inf(1)))
	}
	return t, true
}

// ---- flat views of the grids (row-major, no padding)

func i16Cells(g *I16Grid) *[cells]int16   { return (*[cells]int16)(unsafe.Pointer(g)) }
func i32Cells(g *I32Grid) *[cells]int32   { return (*[cells]int32)(unsafe.Pointer(g)) }
func f32Cells(g *F32Grid) *[cells]float32 { return (*[cells]float32)(unsafe.Pointer(g)) }

func b2u8(b bool) uint8 {
	if b {
		return 1
	}
	return 0
}
