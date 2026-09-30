package nle

import (
	"math/bits"
	"sync"

	. "aago/grid"
	. "aago/internal/pyrt"
)

// ---- GSet: set of glyph ids (fast isin). Indexed by uint16(v): negative values map above MAX_GLYPH (never set).

type GSet struct {
	bits   [1024]uint64
	list   []int
	ranges []Range16 // the same set as sorted runs of consecutive ids (most sets are one or two runs)
}

func NewGSet(vals ...int) *GSet {
	s := &GSet{}
	for _, v := range vals {
		s.add(v)
	}
	s.findRanges()
	return s
}

// findRanges fills ranges from bits.
func (s *GSet) findRanges() {
	s.ranges = nil
	for i, w := range s.bits {
		for ; w != 0; w &= w - 1 {
			v := int16(i<<6 + bits.TrailingZeros64(w))
			if n := len(s.ranges); n > 0 && s.ranges[n-1].Hi == v-1 {
				s.ranges[n-1].Hi = v
			} else {
				s.ranges = append(s.ranges, Range16{v, v})
			}
		}
	}
}

func (s *GSet) add(v int) {
	if v < 0 || v >= NH_MAX_GLYPH+64 {
		panic("glyph out of range")
	}
	if !s.Has(v) {
		s.list = append(s.list, v)
	}
	u := uint16(v)
	s.bits[u>>6] |= 1 << (u & 63)
}

func (s *GSet) Has(v int) bool {
	if v < -32768 || v > 32767 {
		return false
	}
	u := uint16(int16(v))
	return s.bits[u>>6]&(1<<(u&63)) != 0
}

func (s *GSet) Union(o ...*GSet) *GSet {
	r := &GSet{bits: s.bits, list: append([]int(nil), s.list...)}
	for _, x := range o {
		for _, v := range x.list {
			r.add(v)
		}
	}
	r.findRanges()
	return r
}

func (s *GSet) List() []int { return append([]int(nil), s.list...) }

type gsetKey [10]*GSet

// unionCache memoizes unions of static glyph sets (a pure function; safe for several bots in one process).
var unionCache sync.Map // gsetKey -> *GSet

func UnionOf(sets []*GSet) *GSet {
	if len(sets) == 1 {
		return sets[0]
	}
	if len(sets) > len(gsetKey{}) {
		return sets[0].Union(sets[1:]...)
	}
	var k gsetKey
	copy(k[:], sets)
	if u, ok := unionCache.Load(k); ok {
		return u.(*GSet)
	}
	u := &GSet{}
	for _, s := range sets {
		for i := range u.bits {
			u.bits[i] |= s.bits[i]
		}
	}
	u.findRanges()
	v, _ := unionCache.LoadOrStore(k, u)
	return v.(*GSet)
}

// Isin mirrors utils.isin(array, *elems) for an int16 grid.
func Isin(g *I16Grid, sets ...*GSet) (r BoolGrid) {
	if len(sets) == 0 {
		Raise("ValueError", "zero-size array to reduction operation")
	}
	u := UnionOf(sets)
	if len(u.ranges) <= maxRanges {
		return InRanges(g, u.ranges)
	}
	for y := 0; y < SY; y++ {
		row := &g[y]
		out := &r[y]
		for x := 0; x < SX; x++ {
			v := uint16(row[x])
			out[x] = u.bits[v>>6]&(1<<(v&63)) != 0
		}
	}
	return
}

func AnyIn(g *I16Grid, sets ...*GSet) bool {
	u := UnionOf(sets)
	if len(u.ranges) <= maxRanges {
		return AnyInRanges(g, u.ranges)
	}
	for y := 0; y < SY; y++ {
		row := &g[y]
		for x := 0; x < SX; x++ {
			v := uint16(row[x])
			if u.bits[v>>6]&(1<<(v&63)) != 0 {
				return true
			}
		}
	}
	return false
}

// maxRanges: a set of up to this many runs of ids is tested by range compares (InRanges), a bigger one by its
// bits. Glyph ids are 0..MAX_GLYPH, so a grid value outside them (-1 for "unknown") is in no range, as with bits.
const maxRanges = 8
