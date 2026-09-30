package world

import (
	"math"
	"math/rand"
	"sort"
	"testing"
)

// NpQuantileLinear selects instead of sorting: it must give what the sorted version gives.
func TestQuantileBySelection(t *testing.T) {
	rng := rand.New(rand.NewSource(1))
	sorted := func(vals []int32, q float64) float64 { // the version with a full sort
		n := len(vals)
		s := append([]int32(nil), vals...)
		sort.Slice(s, func(i, j int) bool { return s[i] < s[j] })
		vi := float64(float64(n-1) * q)
		prev := math.Floor(vi)
		var pi, ni int
		if vi >= float64(n-1) {
			pi, ni = n-1, n-1
			prev = -1
		} else {
			pi = int(prev)
			ni = pi + 1
		}
		gamma := vi - prev
		av, bv := s[pi], s[ni]
		diff := float64(bv - av)
		res := float64(av) + float64(diff*gamma)
		if gamma >= 0.5 {
			res = float64(bv) - float64(diff*(1-gamma))
		}
		return res
	}
	for n := 0; n < 20000; n++ {
		vals := make([]int32, 1+rng.Intn(60))
		span := 1 + rng.Intn(5)
		if rng.Intn(2) == 0 {
			span = 1 + rng.Intn(1000)
		}
		for i := range vals {
			vals[i] = int32(1 + rng.Intn(span))
		}
		for _, q := range []float64{0, 0.3, 0.5, 0.99, 1} {
			if got, want := NpQuantileLinear(vals, q), sorted(vals, q); got != want {
				t.Fatalf("%v q=%v: %v, want %v", vals, q, got, want)
			}
		}
		// selectKth itself
		k := rng.Intn(len(vals))
		s := append([]int32(nil), vals...)
		selectKth(s, k)
		ss := append([]int32(nil), vals...)
		sort.Slice(ss, func(i, j int) bool { return ss[i] < ss[j] })
		if s[k] != ss[k] {
			t.Fatalf("selectKth(%v, %d) = %d, want %d", vals, k, s[k], ss[k])
		}
		for i := range s {
			if (i < k && s[i] > s[k]) || (i > k && s[i] < s[k]) {
				t.Fatalf("selectKth(%v, %d): not partitioned: %v", vals, k, s)
			}
		}
	}
}
