package grid

import (
	"math/rand"
	"testing"
)

func BenchmarkNeighborCount8(b *testing.B) {
	rng := rand.New(rand.NewSource(1))
	var m BoolGrid
	for y := 0; y < SY; y++ {
		for x := 0; x < SX; x++ {
			m[y][x] = rng.Intn(3) == 0
		}
	}
	var out I32Grid
	for i := 0; i < b.N; i++ {
		NeighborCount8(&m, &out)
	}
}

func BenchmarkBFS(b *testing.B) {
	var w BoolGrid
	for y := 0; y < SY; y++ {
		for x := 0; x < SX; x++ {
			w[y][x] = (x+y)%7 != 0
		}
	}
	for i := 0; i < b.N; i++ {
		BFS(10, 40, &w, &w, true)
	}
}

func BenchmarkBoolOps(b *testing.B) {
	rng := rand.New(rand.NewSource(1))
	var m, o BoolGrid
	for y := 0; y < SY; y++ {
		for x := 0; x < SX; x++ {
			m[y][x] = rng.Intn(3) == 0
			o[y][x] = rng.Intn(2) == 0
		}
	}
	n := 0
	for i := 0; i < b.N; i++ {
		r := m.And(o).Or(m).AndNot(o)
		n += r.Count() + len(r.Nonzero())
		if r.Any() {
			n++
		}
	}
	_ = n
}
