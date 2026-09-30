//go:build goexperiment.simd && amd64

package grid

import (
	"math"
	"math/bits"
	"simd/archsimd"
	"unsafe"
)

// AVX2 versions of the bulk operations in vec.go: 16 int16 cells, 8 int32 or 8 float32 cells per step, the
// last cells%16 (or %8, %4) one by one. Only element-wise operations are used (no FMA, no reordering), so each
// cell gets exactly the bits of the plain loop.

// SIMDOn reports whether the bulk operations of vec.go run their SIMD versions.
func SIMDOn() bool { return archsimd.X86.AVX2() }

func loadWordAt(b *[cells]byte, i int) uint64     { return *(*uint64)(unsafe.Pointer(&b[i])) }
func storeWordAt(b *[cells]byte, i int, v uint64) { *(*uint64)(unsafe.Pointer(&b[i])) = v }

// spread8 turns the 8 bits of a mask into 8 bool bytes: byte i is 1 when bit i is set.
func spread8(m uint8) uint64 {
	x := uint64(m) * ones & 0x8040201008040201  // byte i keeps bit i of m
	return (x + 0x7f7f7f7f7f7f7f7f) >> 7 & ones // a nonzero byte gets its top bit set (no carry out of a byte)
}

// boolMask8 is the mask of the 8 bool bytes in w (one 32-bit lane per byte).
func boolMask8(w uint64) archsimd.Mask32x8 {
	v := archsimd.Uint64x2{}.SetElem(0, w).AsUint8x16().ExtendLo8ToUint32().AsInt32x8()
	return v.Greater(archsimd.Int32x8{})
}

func inRanges2Mask(v, lo0, hi0, lo1, hi1 archsimd.Int16x16) archsimd.Mask16x16 {
	return v.Greater(lo0).And(hi0.Greater(v)).Or(v.Greater(lo1).And(hi1.Greater(v)))
}

func bcastRange(r Range16) (lo, hi archsimd.Int16x16) {
	return archsimd.BroadcastInt16x16(r.Lo - 1), archsimd.BroadcastInt16x16(r.Hi + 1)
}

// Vector constants as package-level tables: a literal inside a loop is rebuilt on the stack every time and
// stalls the vector load that reads it.
var (
	// lowBytes picks the low byte of every 16-bit lane into the first 8 bytes of each 128-bit half (VPSHUFB)
	lowBytes = [32]int8{0, 2, 4, 6, 8, 10, 12, 14, -1, -1, -1, -1, -1, -1, -1, -1,
		0, 2, 4, 6, 8, 10, 12, 14, -1, -1, -1, -1, -1, -1, -1, -1}
	// halves16 puts those two 8-byte groups side by side (VPERMD): 16 lanes of int16 -> 16 bytes in order
	halves16 = [8]uint32{0, 1, 4, 5, 0, 1, 4, 5}
	// halves32 does the same after two int32x8 are packed to int16 (VPACKSSDW interleaves them by 4)
	halves32 = [8]uint32{0, 4, 1, 5, 0, 4, 1, 5}
)

// narrower turns 16 lanes of 0/-1 into 16 bool bytes (see narrow).
type narrower struct {
	pick archsimd.Int8x32
	join archsimd.Uint32x8
	one  archsimd.Uint8x16
}

func newNarrower(join *[8]uint32) narrower {
	return narrower{archsimd.LoadInt8x32Array(&lowBytes), archsimd.LoadUint32x8Array(join), archsimd.BroadcastUint8x16(1)}
}

func (n *narrower) narrow(v archsimd.Int16x16) archsimd.Uint8x16 {
	b := v.AsInt8x32().PermuteOrZeroGrouped(n.pick).AsInt32x8().Permute(n.join)
	return b.AsUint8x32().GetLo().And(n.one)
}

func bytes16(dst *[cells]byte, i int) *[16]uint8 { return (*[16]uint8)(dst[i : i+16]) }

// storeMask16 writes the 16 bool bytes of m at dst[i:], OR-ed with what is there when or is set.
func (n *narrower) storeMask16(dst *[cells]byte, i int, m archsimd.Mask16x16, or bool) {
	b := n.narrow(m.ToInt16x16())
	if or {
		b = b.Or(archsimd.LoadUint8x16Array(bytes16(dst, i)))
	}
	b.StoreArray(bytes16(dst, i))
}

func inRangesTail(src *[cells]int16, dst *[cells]byte, i int, rs []Range16, or bool) {
	for ; i < cells; i++ {
		dst[i] = b2u8(inAny(src[i], rs) || or && dst[i] != 0)
	}
}

func inRanges2SIMD(g *I16Grid, r0, r1 Range16, out *BoolGrid, or bool) {
	src, dst := i16Cells(g), cellBytes(out)
	lo0, hi0 := bcastRange(r0)
	lo1, hi1 := bcastRange(r1)
	n := newNarrower(&halves16)
	i := 0
	for ; i+16 <= cells; i += 16 {
		v := archsimd.LoadInt16x16Array((*[16]int16)(src[i : i+16]))
		n.storeMask16(dst, i, inRanges2Mask(v, lo0, hi0, lo1, hi1), or)
	}
	inRangesTail(src, dst, i, []Range16{r0, r1}, or)
}

func inRanges4SIMD(g *I16Grid, r [4]Range16, out *BoolGrid, or bool) {
	src, dst := i16Cells(g), cellBytes(out)
	lo0, hi0 := bcastRange(r[0])
	lo1, hi1 := bcastRange(r[1])
	lo2, hi2 := bcastRange(r[2])
	lo3, hi3 := bcastRange(r[3])
	n := newNarrower(&halves16)
	i := 0
	for ; i+16 <= cells; i += 16 {
		v := archsimd.LoadInt16x16Array((*[16]int16)(src[i : i+16]))
		m := inRanges2Mask(v, lo0, hi0, lo1, hi1).Or(inRanges2Mask(v, lo2, hi2, lo3, hi3))
		n.storeMask16(dst, i, m, or)
	}
	inRangesTail(src, dst, i, r[:], or)
}

func anyInRanges2SIMD(g *I16Grid, r0, r1 Range16) bool {
	src := i16Cells(g)
	lo0, hi0 := bcastRange(r0)
	lo1, hi1 := bcastRange(r1)
	i := 0
	for ; i+16 <= cells; i += 16 {
		v := archsimd.LoadInt16x16Array((*[16]int16)(src[i : i+16]))
		if !inRanges2Mask(v, lo0, hi0, lo1, hi1).ToInt16x16().IsZero() {
			return true
		}
	}
	for ; i < cells; i++ {
		if inRange(src[i], r0) || inRange(src[i], r1) {
			return true
		}
	}
	return false
}

func anyInRanges4SIMD(g *I16Grid, r [4]Range16) bool {
	src := i16Cells(g)
	lo0, hi0 := bcastRange(r[0])
	lo1, hi1 := bcastRange(r[1])
	lo2, hi2 := bcastRange(r[2])
	lo3, hi3 := bcastRange(r[3])
	i := 0
	for ; i+16 <= cells; i += 16 {
		v := archsimd.LoadInt16x16Array((*[16]int16)(src[i : i+16]))
		m := inRanges2Mask(v, lo0, hi0, lo1, hi1).Or(inRanges2Mask(v, lo2, hi2, lo3, hi3))
		if !m.ToInt16x16().IsZero() {
			return true
		}
	}
	for ; i < cells; i++ {
		if inAny(src[i], r[:]) {
			return true
		}
	}
	return false
}

// boolMask16 is the mask of the 16 bool bytes at b[i:] (one 16-bit lane per byte).
func boolMask16(b *[cells]byte, i int) archsimd.Mask16x16 {
	v := archsimd.LoadUint8x16Array((*[16]uint8)(b[i : i+16])).ExtendToUint16().AsInt16x16()
	return v.Greater(archsimd.Int16x16{})
}

func selectI16SIMD(g, src *I16Grid, m *BoolGrid, v int16, constant bool) {
	r, s, b := i16Cells(g), i16Cells(src), cellBytes(m)
	cv := archsimd.BroadcastInt16x16(v)
	i := 0
	for ; i+16 <= cells; i += 16 {
		if loadWordAt(b, i)|loadWordAt(b, i+8) == 0 {
			continue
		}
		p := (*[16]int16)(r[i : i+16])
		y := cv
		if !constant {
			y = archsimd.LoadInt16x16Array((*[16]int16)(s[i : i+16]))
		}
		y.IfElse(boolMask16(b, i), archsimd.LoadInt16x16Array(p)).StoreArray(p)
	}
	for ; i < cells; i++ {
		if b[i] != 0 {
			if constant {
				r[i] = v
			} else {
				r[i] = s[i]
			}
		}
	}
}

func setWhereI32SIMD(g *I32Grid, m *BoolGrid, v int32) {
	r, b := i32Cells(g), cellBytes(m)
	cv := archsimd.BroadcastInt32x8(v)
	i := 0
	for ; i+8 <= cells; i += 8 {
		w := loadWordAt(b, i)
		if w == 0 {
			continue
		}
		p := (*[8]int32)(r[i : i+8])
		cv.IfElse(boolMask8(w), archsimd.LoadInt32x8Array(p)).StoreArray(p)
	}
	for ; i < cells; i++ {
		if b[i] != 0 {
			r[i] = v
		}
	}
}

func cmpI32SIMD(g *I32Grid, v int32, op cmpOp, not bool, out *BoolGrid) {
	src, dst := i32Cells(g), cellBytes(out)
	c := archsimd.BroadcastInt32x8(v)
	n := newNarrower(&halves32)
	var flip archsimd.Uint8x16
	if not {
		flip = n.one
	}
	cmp := func(x archsimd.Int32x8) archsimd.Int32x8 {
		switch op {
		case cmpEq:
			return x.Equal(c).ToInt32x8()
		case cmpGt:
			return x.Greater(c).ToInt32x8()
		}
		return c.Greater(x).ToInt32x8()
	}
	i := 0
	for ; i+16 <= cells; i += 16 {
		m0 := cmp(archsimd.LoadInt32x8Array((*[8]int32)(src[i : i+8])))
		m1 := cmp(archsimd.LoadInt32x8Array((*[8]int32)(src[i+8 : i+16])))
		n.narrow(m0.SaturateToInt16ConcatGrouped(m1)).Xor(flip).StoreArray(bytes16(dst, i))
	}
	for ; i < cells; i++ {
		var r bool
		switch op {
		case cmpEq:
			r = src[i] == v
		case cmpGt:
			r = src[i] > v
		default:
			r = src[i] < v
		}
		dst[i] = b2u8(r != not)
	}
}

func mulI32SIMD(g, o, out *I32Grid) {
	a, b, r := i32Cells(g), i32Cells(o), i32Cells(out)
	i := 0
	for ; i+8 <= cells; i += 8 {
		x := archsimd.LoadInt32x8Array((*[8]int32)(a[i : i+8]))
		y := archsimd.LoadInt32x8Array((*[8]int32)(b[i : i+8]))
		x.Mul(y).StoreArray((*[8]int32)(r[i : i+8]))
	}
	for ; i < cells; i++ {
		r[i] = a[i] * b[i]
	}
}

func mulScalarI32SIMD(g *I32Grid, c int32, out *I32Grid) {
	a, r := i32Cells(g), i32Cells(out)
	cv := archsimd.BroadcastInt32x8(c)
	i := 0
	for ; i+8 <= cells; i += 8 {
		archsimd.LoadInt32x8Array((*[8]int32)(a[i : i+8])).Mul(cv).StoreArray((*[8]int32)(r[i : i+8]))
	}
	for ; i < cells; i++ {
		r[i] = a[i] * c
	}
}

func appendPositiveSIMD(g *I32Grid, dst []int32) []int32 {
	src := i32Cells(g)
	var zero archsimd.Int32x8
	i := 0
	for ; i+8 <= cells; i += 8 {
		m := archsimd.LoadInt32x8Array((*[8]int32)(src[i : i+8])).Greater(zero).ToBits()
		for ; m != 0; m &= m - 1 {
			dst = append(dst, src[i+bits.TrailingZeros8(m)])
		}
	}
	for ; i < cells; i++ {
		if src[i] > 0 {
			dst = append(dst, src[i])
		}
	}
	return dst
}

func fullF32SIMD(v float32, out *F32Grid) {
	r := f32Cells(out)
	cv := archsimd.BroadcastFloat32x8(v)
	i := 0
	for ; i+8 <= cells; i += 8 {
		cv.StoreArray((*[8]float32)(r[i : i+8]))
	}
	for ; i < cells; i++ {
		r[i] = v
	}
}

func subI32SIMD(g *F32Grid, v *I32Grid) {
	r, s := f32Cells(g), i32Cells(v)
	i := 0
	for ; i+4 <= cells; i += 4 {
		x := archsimd.LoadFloat32x4Array((*[4]float32)(r[i : i+4])).ConvertToFloat64()
		y := archsimd.LoadInt32x4Array((*[4]int32)(s[i : i+4])).ConvertToFloat64()
		x.Sub(y).ConvertToFloat32().StoreArray((*[4]float32)(r[i : i+4]))
	}
	for ; i < cells; i++ {
		r[i] = float32(float64(r[i]) - float64(s[i]))
	}
}

func addWhereSIMD(g *F32Grid, m *BoolGrid, c float32) {
	r, b := f32Cells(g), cellBytes(m)
	cv := archsimd.BroadcastFloat32x8(c)
	i := 0
	for ; i+8 <= cells; i += 8 {
		w := loadWordAt(b, i)
		if w == 0 {
			continue
		}
		p := (*[8]float32)(r[i : i+8])
		x := archsimd.LoadFloat32x8Array(p)
		x.Add(cv).IfElse(boolMask8(w), x).StoreArray(p)
	}
	for ; i < cells; i++ {
		if b[i] != 0 {
			r[i] += c
		}
	}
}

func setWhereSIMD(g *F32Grid, m *BoolGrid, c float32) {
	r, b := f32Cells(g), cellBytes(m)
	cv := archsimd.BroadcastFloat32x8(c)
	i := 0
	for ; i+8 <= cells; i += 8 {
		w := loadWordAt(b, i)
		if w == 0 {
			continue
		}
		p := (*[8]float32)(r[i : i+8])
		cv.IfElse(boolMask8(w), archsimd.LoadFloat32x8Array(p)).StoreArray(p)
	}
	for ; i < cells; i++ {
		if b[i] != 0 {
			r[i] = c
		}
	}
}

func geF32SIMD(g *F32Grid, v float64, out *BoolGrid) {
	t, ok := f32Threshold(v)
	if !ok {
		*out = BoolGrid{}
		return
	}
	r, dst := f32Cells(g), cellBytes(out)
	tv := archsimd.BroadcastFloat32x8(t)
	i := 0
	for ; i+8 <= cells; i += 8 {
		m := archsimd.LoadFloat32x8Array((*[8]float32)(r[i : i+8])).GreaterEqual(tv)
		storeWordAt(dst, i, spread8(m.ToBits()))
	}
	for ; i < cells; i++ {
		dst[i] = b2u8(r[i] >= t)
	}
}

func boolOpSIMD(a, b *[cells]byte, op boolOpKind) {
	// one loop per operation: a switch inside the loop keeps the compiler from keeping it tight
	i := 0
	switch op {
	case opAnd:
		for ; i+32 <= cells; i += 32 {
			p, q := bytes32(a, i), bytes32(b, i)
			archsimd.LoadUint8x32Array(p).And(archsimd.LoadUint8x32Array(q)).StoreArray(p)
		}
	case opOr:
		for ; i+32 <= cells; i += 32 {
			p, q := bytes32(a, i), bytes32(b, i)
			archsimd.LoadUint8x32Array(p).Or(archsimd.LoadUint8x32Array(q)).StoreArray(p)
		}
	case opAndNot:
		for ; i+32 <= cells; i += 32 {
			p, q := bytes32(a, i), bytes32(b, i)
			archsimd.LoadUint8x32Array(p).AndNot(archsimd.LoadUint8x32Array(q)).StoreArray(p)
		}
	case opNot:
		one := archsimd.BroadcastUint8x32(1)
		for ; i+32 <= cells; i += 32 {
			p := bytes32(a, i)
			archsimd.LoadUint8x32Array(p).Xor(one).StoreArray(p)
		}
	}
	boolOpWords(a, b, op, i)
}

func bytes32(b *[cells]byte, i int) *[32]uint8 { return (*[32]uint8)(b[i : i+32]) }

func ltF32SIMD(g *F32Grid, v float64, out *BoolGrid) {
	t, ok := f32Threshold(v) // float64(x) < v is x < t
	if !ok {
		*out = BoolGrid{}
		return
	}
	r, dst := f32Cells(g), cellBytes(out)
	tv := archsimd.BroadcastFloat32x8(t)
	i := 0
	for ; i+8 <= cells; i += 8 {
		storeWordAt(dst, i, spread8(archsimd.LoadFloat32x8Array((*[8]float32)(r[i:i+8])).Less(tv).ToBits()))
	}
	for ; i < cells; i++ {
		dst[i] = b2u8(r[i] < t)
	}
}

func eqF32SIMD(g *F32Grid, v float32, out *BoolGrid) {
	r, dst := f32Cells(g), cellBytes(out)
	cv := archsimd.BroadcastFloat32x8(v)
	i := 0
	for ; i+8 <= cells; i += 8 {
		storeWordAt(dst, i, spread8(archsimd.LoadFloat32x8Array((*[8]float32)(r[i:i+8])).Equal(cv).ToBits()))
	}
	for ; i < cells; i++ {
		dst[i] = b2u8(r[i] == v)
	}
}

// isFiniteF32SIMD: x - x is 0 for a finite x and NaN for an infinity or a NaN.
func isFiniteF32SIMD(g *F32Grid, out *BoolGrid) {
	r, dst := f32Cells(g), cellBytes(out)
	var zero archsimd.Float32x8
	i := 0
	for ; i+8 <= cells; i += 8 {
		x := archsimd.LoadFloat32x8Array((*[8]float32)(r[i : i+8]))
		storeWordAt(dst, i, spread8(x.Sub(x).Equal(zero).ToBits()))
	}
	for ; i < cells; i++ {
		f := float64(r[i])
		dst[i] = b2u8(!math.IsInf(f, 0) && !math.IsNaN(f))
	}
}
