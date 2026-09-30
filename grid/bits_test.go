package grid

import (
	"math/rand"
	"testing"
)

// The word-at-a-time ops against the cell-by-cell definitions.
func TestBoolGridWordOps(t *testing.T) {
	rng := rand.New(rand.NewSource(5))
	grid := func(p float64) (m BoolGrid) {
		for y := 0; y < SY; y++ {
			for x := 0; x < SX; x++ {
				m[y][x] = rng.Float64() < p
			}
		}
		return
	}
	for n := 0; n < 3000; n++ {
		ps := []float64{0, 1, 0.001, 0.01, 0.5, 0.99, rng.Float64()}
		a, b := grid(ps[rng.Intn(len(ps))]), grid(ps[rng.Intn(len(ps))])
		if rng.Intn(4) == 0 { // a single cell, often in the last few cells
			a = BoolGrid{}
			c := cells - 1 - rng.Intn(12)
			a[c/SX][c%SX] = true
		}
		var and, or, andNot, not BoolGrid
		any, all, count := false, true, 0
		var nz []Pos
		for y := 0; y < SY; y++ {
			for x := 0; x < SX; x++ {
				and[y][x] = a[y][x] && b[y][x]
				or[y][x] = a[y][x] || b[y][x]
				andNot[y][x] = a[y][x] && !b[y][x]
				not[y][x] = !a[y][x]
				any = any || a[y][x]
				all = all && a[y][x]
				if a[y][x] {
					count++
					nz = append(nz, Pos{y, x})
				}
			}
		}
		if a.And(b) != and || a.Or(b) != or || a.AndNot(b) != andNot || a.Not() != not {
			t.Fatal("and/or/andnot/not")
		}
		if a.Any() != any || a.All() != all || a.Count() != count {
			t.Fatalf("any/all/count: %v %v %v, want %v %v %v", a.Any(), a.All(), a.Count(), any, all, count)
		}
		got := a.Nonzero()
		if len(got) != len(nz) {
			t.Fatalf("nonzero: %d cells, want %d", len(got), len(nz))
		}
		for i := range nz {
			if got[i] != nz[i] {
				t.Fatalf("nonzero[%d] = %v, want %v", i, got[i], nz[i])
			}
		}
		if len(nz) > 0 && a.First() != nz[0] {
			t.Fatalf("first = %v, want %v", a.First(), nz[0])
		}
	}
}

func TestNeighborAny(t *testing.T) {
	rng := rand.New(rand.NewSource(6))
	for n := 0; n < 500; n++ {
		var m BoolGrid
		p := []float64{0.001, 0.05, 0.3, 0.9}[n%4]
		for y := 0; y < SY; y++ {
			for x := 0; x < SX; x++ {
				m[y][x] = rng.Float64() < p
			}
		}
		var c8, c4 I32Grid
		NeighborCount8(&m, &c8)
		NeighborCount4(&m, &c4)
		a8, a4 := NeighborAny8(&m), NeighborAny4(&m)
		for y := 0; y < SY; y++ {
			for x := 0; x < SX; x++ {
				if a8[y][x] != (c8[y][x] > 0) || a4[y][x] != (c4[y][x] > 0) {
					t.Fatalf("(%d,%d): any8 %v any4 %v, counts %d %d", y, x, a8[y][x], a4[y][x], c8[y][x], c4[y][x])
				}
			}
		}
	}
}
