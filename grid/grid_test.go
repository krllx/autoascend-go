package grid

import (
	"math/rand"
	"testing"

	. "aago/internal/pyrt"
)

func TestNeighborCount8(t *testing.T) {
	rng := rand.New(rand.NewSource(2))
	for n := 0; n < 200; n++ {
		var m BoolGrid
		p := rng.Float64()
		for y := 0; y < SY; y++ {
			for x := 0; x < SX; x++ {
				m[y][x] = rng.Float64() < p
			}
		}
		var got, want I32Grid
		NeighborCount8(&m, &got)
		for dy := -1; dy <= 1; dy++ {
			for dx := -1; dx <= 1; dx++ {
				if dy == 0 && dx == 0 {
					continue
				}
				tr := TranslateB(&m, dy, dx)
				for y := 0; y < SY; y++ {
					for x := 0; x < SX; x++ {
						want[y][x] += int32(B2i(tr[y][x]))
					}
				}
			}
		}
		if got != want {
			t.Fatal("mismatch")
		}
	}
}

func TestBFSMatchesFlat(t *testing.T) {
	rng := rand.New(rand.NewSource(3))
	for n := 0; n < 500; n++ {
		var w, d BoolGrid
		p := rng.Float64()
		for y := 0; y < SY; y++ {
			for x := 0; x < SX; x++ {
				w[y][x] = rng.Float64() < p
				d[y][x] = rng.Float64() < 0.7
			}
		}
		y, x := rng.Intn(SY), rng.Intn(SX)
		sq := rng.Intn(2) == 0
		got := BFS(y, x, &w, &d, sq)
		want := BFSFlat(SY, SX, y, x, flatB(&w), flatB(&d), sq)
		for yy := 0; yy < SY; yy++ {
			for xx := 0; xx < SX; xx++ {
				if got[yy][xx] != want[yy*SX+xx] {
					t.Fatalf("bfs mismatch")
				}
			}
		}
	}
}

func TestNeighborCount4(t *testing.T) {
	rng := rand.New(rand.NewSource(4))
	for n := 0; n < 200; n++ {
		var m BoolGrid
		for y := 0; y < SY; y++ {
			for x := 0; x < SX; x++ {
				m[y][x] = rng.Float64() < 0.4
			}
		}
		var got, want I32Grid
		NeighborCount4(&m, &got)
		for _, d := range [][2]int{{1, 0}, {-1, 0}, {0, 1}, {0, -1}} {
			tr := TranslateB(&m, d[0], d[1])
			for y := 0; y < SY; y++ {
				for x := 0; x < SX; x++ {
					want[y][x] += int32(B2i(tr[y][x]))
				}
			}
		}
		if got != want {
			t.Fatal("mismatch")
		}
	}
}
