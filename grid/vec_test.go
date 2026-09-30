package grid

import (
	"math"
	"math/rand"
	"testing"
)

// The bulk operations (SIMD when SIMDOn) against their plain loops, bit for bit. Run the tests with
// GOEXPERIMENT=simd so that the SIMD versions are the ones checked.

func TestVecSIMDState(t *testing.T) { t.Logf("SIMDOn = %v", SIMDOn()) }

func randI16(r *rand.Rand) (g I16Grid) {
	c := i16Cells(&g)
	for i := range c {
		switch r.Intn(4) {
		case 0:
			c[i] = int16(r.Intn(1 << 16))
		case 1:
			c[i] = []int16{math.MinInt16, math.MaxInt16, -1, 0, 1}[r.Intn(5)]
		default:
			c[i] = int16(r.Intn(40) - 5)
		}
	}
	return
}

func randI32(r *rand.Rand) (g I32Grid) {
	c := i32Cells(&g)
	for i := range c {
		switch r.Intn(4) {
		case 0:
			c[i] = int32(r.Uint32())
		case 1:
			c[i] = []int32{math.MinInt32, math.MaxInt32, -1, 0, 1, 1 << 24, 1<<24 + 1}[r.Intn(7)]
		default:
			c[i] = int32(r.Intn(12) - 3)
		}
	}
	return
}

func randF32(r *rand.Rand) (g F32Grid) {
	c := f32Cells(&g)
	special := []float32{float32(math.NaN()), float32(math.Inf(1)), float32(math.Inf(-1)), 0,
		float32(math.Copysign(0, -1)), math.SmallestNonzeroFloat32, math.MaxFloat32, -math.MaxFloat32, 1 << 24, -(1 << 24) - 2}
	for i := range c {
		switch r.Intn(4) {
		case 0:
			c[i] = math.Float32frombits(r.Uint32())
		case 1:
			c[i] = special[r.Intn(len(special))]
		default:
			c[i] = float32(r.Intn(2000) - 1000)
		}
	}
	return
}

func randBool(r *rand.Rand, density int) (g BoolGrid) {
	c := cellBytes(&g)
	for i := range c {
		c[i] = b2u8(r.Intn(density) == 0)
	}
	return
}

func sameF32(a, b *F32Grid) bool {
	x, y := f32Cells(a), f32Cells(b)
	for i := range x {
		if math.Float32bits(x[i]) != math.Float32bits(y[i]) {
			return false
		}
	}
	return true
}

func TestVecRanges(t *testing.T) {
	r := rand.New(rand.NewSource(1))
	rr := func() Range16 {
		switch r.Intn(4) {
		case 0:
			return NoRange16()
		case 1:
			lo := int16(r.Intn(1<<16) - 1<<15)
			return Range16{lo, lo + int16(r.Intn(100))}
		default:
			lo := int16(r.Intn(30) - 5)
			return Range16{lo, lo + int16(r.Intn(8))}
		}
	}
	for it := 0; it < 600; it++ {
		g := randI16(r)
		rs := make([]Range16, r.Intn(10))
		for i := range rs {
			rs[i] = rr()
		}
		if it%50 == 0 && len(rs) > 0 {
			rs[0] = Range16{math.MinInt16, 3} // not for the vector code: the plain loop runs
		}
		var want BoolGrid
		inRangesPlain(&g, rs, &want)
		if got := InRanges(&g, rs); got != want {
			t.Fatalf("InRanges %v differs", rs)
		}
		if got := AnyInRanges(&g, rs); got != anyInRangesPlain(&g, rs) {
			t.Fatalf("AnyInRanges %v: %v", rs, got)
		}
		// one match in the tail or elsewhere, or none at all
		var h I16Grid
		c := i16Cells(&h)
		for i := range c {
			c[i] = 1000
		}
		if it%2 == 0 {
			c[cells-1-r.Intn(cells)] = 7
		}
		n := 1 + r.Intn(6)
		one := make([]Range16, n)
		for i := range one {
			one[i] = NoRange16()
		}
		one[r.Intn(n)] = Range16{7, 7}
		if AnyInRanges(&h, one) != (it%2 == 0) {
			t.Fatalf("AnyInRanges on a single cell")
		}
	}
}

func TestVecSelect(t *testing.T) {
	r := rand.New(rand.NewSource(7))
	for it := 0; it < 300; it++ {
		g, src, m := randI16(r), randI16(r), randBool(r, 1+r.Intn(20))
		v := []int16{-1, 0, math.MinInt16, math.MaxInt16, int16(r.Intn(1 << 16))}[r.Intn(5)]
		a, b := g, g
		a.SetWhere(&m, v)
		selectI16Plain(&b, &src, &m, v, true)
		if a != b {
			t.Fatalf("I16 SetWhere differs")
		}
		a, b = g, g
		a.CopyWhere(&src, &m)
		selectI16Plain(&b, &src, &m, 0, false)
		if a != b {
			t.Fatalf("I16 CopyWhere differs")
		}
		var want BoolGrid
		inRangesPlain(&g, []Range16{{v, v}}, &want)
		if got := g.Eq(v); got != want {
			t.Fatalf("I16 Eq %d differs", v)
		}
		gi := randI32(r)
		ai, bi := gi, gi
		ai.SetWhere(&m, int32(v))
		setWhereI32Plain(&bi, &m, int32(v))
		if ai != bi {
			t.Fatalf("I32 SetWhere differs")
		}
	}
}

func TestVecI32(t *testing.T) {
	r := rand.New(rand.NewSource(2))
	for it := 0; it < 300; it++ {
		g, o := randI32(r), randI32(r)
		v := []int32{0, -1, 1, 3, math.MinInt32, math.MaxInt32, int32(r.Uint32())}[r.Intn(7)]
		for op := cmpEq; op <= cmpLt; op++ {
			for _, not := range []bool{false, true} {
				var want BoolGrid
				cmpI32Plain(&g, v, op, not, &want)
				if got := cmpI32(&g, v, op, not); got != want {
					t.Fatalf("cmp %d %v %d differs", op, not, v)
				}
			}
		}
		var want I32Grid
		mulI32Plain(&g, &o, &want)
		if got := g.Mul(&o); got != want {
			t.Fatalf("Mul differs")
		}
		p := g.AppendPositive([]int32{42})
		q := appendPositivePlain(&g, []int32{42})
		if len(p) != len(q) {
			t.Fatalf("AppendPositive: %d values, want %d", len(p), len(q))
		}
		for i := range p {
			if p[i] != q[i] {
				t.Fatalf("AppendPositive differs at %d", i)
			}
		}
	}
}

func TestVecF32(t *testing.T) {
	r := rand.New(rand.NewSource(3))
	limits := []float64{0, -1, 0.1, -0.1, 1e40, -1e40, math.NaN(), math.Inf(1), math.Inf(-1), 1<<24 + 1, -(1<<24 + 1),
		math.MaxFloat32, -math.MaxFloat32, 5e-46}
	for it := 0; it < 300; it++ {
		g, s, m := randF32(r), randI32(r), randBool(r, 1+r.Intn(4))
		c := []float32{250, -1, 0.1, float32(math.Inf(-1)), float32(math.NaN()), 1e-40}[r.Intn(6)]

		a, b := g, g
		a.SubI32(&s)
		subI32Plain(&b, &s)
		if !sameF32(&a, &b) {
			t.Fatalf("SubI32 differs")
		}
		a, b = g, g
		a.AddWhere(&m, c)
		addWherePlain(&b, &m, c)
		if !sameF32(&a, &b) {
			t.Fatalf("AddWhere %v differs", c)
		}
		a, b = g, g
		a.SetWhere(&m, c)
		setWherePlain(&b, &m, c)
		if !sameF32(&a, &b) {
			t.Fatalf("SetWhere %v differs", c)
		}
		for _, v := range limits {
			var want BoolGrid
			geF32Plain(&g, v, &want)
			if got := g.GreaterEqual(v); got != want {
				t.Fatalf("GreaterEqual %v differs", v)
			}
			ltF32Plain(&g, v, &want)
			if got := g.Less(v); got != want {
				t.Fatalf("Less %v differs", v)
			}
			eqF32Plain(&g, float32(v), &want)
			if got := g.Eq(float32(v)); got != want {
				t.Fatalf("Eq %v differs", v)
			}
		}
		var want BoolGrid
		isFiniteF32Plain(&g, &want)
		if got := g.IsFinite(); got != want {
			t.Fatalf("IsFinite differs")
		}
	}
	// the plain loops are numpy's float64 arithmetic rounded to float32
	for it := 0; it < 100000; it++ {
		x, c := math.Float32frombits(r.Uint32()), math.Float32frombits(r.Uint32())
		if s := x + c; math.Float32bits(s) != math.Float32bits(float32(float64(x)+float64(c))) && s == s {
			t.Fatalf("%v + %v", x, c)
		}
	}
}

func BenchmarkVecInRanges2(b *testing.B) {
	g := randI16(rand.New(rand.NewSource(4)))
	for i := 0; i < b.N; i++ {
		InRanges(&g, []Range16{{0, 761}})
	}
}

func BenchmarkVecToSearch(b *testing.B) {
	r := rand.New(rand.NewSource(5))
	sc, dis := randI32(r), randI32(r)
	m1, m2 := randBool(r, 8), randBool(r, 3)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		p := FullF32(-1)
		sq := sc.Mul(&sc)
		p.SubI32(&sq)
		p.AddWhere(&m1, 250)
		p.AddWhere(&m2, 250)
		u := dis.Eq(-1)
		p.SetWhere(&u, float32(math.Inf(-1)))
		_ = p.GreaterEqual(0)
	}
}

func BenchmarkVecOps(b *testing.B) {
	r := rand.New(rand.NewSource(5))
	sc, dis := randI32(r), randI32(r)
	m1 := randBool(r, 8)
	p := FullF32(-1)
	b.Run("Full", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			p = FullF32(-1)
		}
	})
	b.Run("Mul", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			sc = sc.Mul(&dis)
		}
	})
	b.Run("SubI32", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			p.SubI32(&sc)
		}
	})
	b.Run("AddWhere", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			p.AddWhere(&m1, 250)
		}
	})
	b.Run("Eq", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			m1 = dis.Eq(-1)
		}
	})
	b.Run("GreaterEqual", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			m1 = p.GreaterEqual(0)
		}
	})
}

func TestVecMulScalarFull(t *testing.T) {
	r := rand.New(rand.NewSource(6))
	for it := 0; it < 200; it++ {
		g := randI32(r)
		c := []int32{2, -1, 0, math.MaxInt32, math.MinInt32, int32(r.Uint32())}[r.Intn(6)]
		var want I32Grid
		mulScalarI32Plain(&g, c, &want)
		if got := g.MulScalar(c); got != want {
			t.Fatalf("MulScalar %d differs", c)
		}
		v := math.Float32frombits(r.Uint32())
		var w F32Grid
		fullF32Plain(v, &w)
		if got := FullF32(v); !sameF32(&got, &w) {
			t.Fatalf("FullF32 %v differs", v)
		}
	}
}

func BenchmarkVecKernels(b *testing.B) {
	r := rand.New(rand.NewSource(8))
	g, dis := randI16(r), randI32(r)
	var out BoolGrid
	b.Run("InRanges1", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			out = InRanges(&g, []Range16{{0, 761}})
		}
	})
	b.Run("InRanges5", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			out = InRanges(&g, []Range16{{0, 761}, {1144, 1524}, {1906, 2352}, {2355, 2358}, {5595, 5975}})
		}
	})
	b.Run("I32Ne", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			out = dis.Ne(-1)
		}
	})
	_ = out
}

func BenchmarkVecBoolOps(b *testing.B) {
	r := rand.New(rand.NewSource(9))
	x, y := randBool(r, 3), randBool(r, 2)
	for i := 0; i < b.N; i++ {
		x = x.And(y).Or(y).AndNot(x).Not()
	}
}
