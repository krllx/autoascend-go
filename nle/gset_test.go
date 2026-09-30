package nle

import (
	"math/rand"
	"testing"

	. "aago/grid"
)

// Isin/AnyIn through the range form of a set (InRanges2) against the membership bits, cell by cell.
func TestIsinRanges(t *testing.T) {
	r := rand.New(rand.NewSource(1))
	sets := [][]*GSet{{G_MONS, G_PETS}, {G_STAIR_UP, G_STAIR_DOWN}, {G_WALL, G_DOOR_CLOSED, G_FLOOR}, {NewGSet()}}
	for _, s := range namedGSets {
		sets = append(sets, []*GSet{s})
	}
	multi := 0
	for _, ss := range sets {
		u := UnionOf(ss)
		if len(u.ranges) > 2 {
			multi++
		}
		for it := 0; it < 20; it++ {
			var g I16Grid
			for y := 0; y < SY; y++ {
				for x := 0; x < SX; x++ {
					switch r.Intn(3) {
					case 0:
						g[y][x] = int16(r.Intn(1 << 16))
					case 1:
						g[y][x] = int16(r.Intn(NH_MAX_GLYPH+70) - 2)
					default:
						if l := u.List(); len(l) > 0 && r.Intn(4) == 0 {
							g[y][x] = int16(l[r.Intn(len(l))])
						} else if len(u.ranges) > 0 {
							rg := u.ranges[r.Intn(len(u.ranges))]
							g[y][x] = rg.Lo - 1 + int16(r.Intn(3))*(rg.Hi-rg.Lo+1)/2 // around the ends
						}
					}
				}
			}
			got := Isin(&g, ss...)
			any := false
			for y := 0; y < SY; y++ {
				for x := 0; x < SX; x++ {
					v := uint16(g[y][x])
					want := u.bits[v>>6]&(1<<(v&63)) != 0
					any = any || want
					if got[y][x] != want {
						t.Fatalf("set %v: cell %d,%d value %d: %v", u.ranges, y, x, g[y][x], got[y][x])
					}
				}
			}
			if AnyIn(&g, ss...) != any {
				t.Fatalf("AnyIn %v", u.ranges)
			}
		}
	}
	t.Logf("%d sets, %d of them more than two ranges", len(sets), multi)
}
