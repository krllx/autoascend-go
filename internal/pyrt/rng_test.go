package pyrt

import (
	"encoding/json"
	"os"
	"testing"
)

func TestRandomState(t *testing.T) {
	b, err := os.ReadFile("/tmp/rngcases.json")
	if err != nil {
		t.Skip()
	}
	var cases [][]any
	json.Unmarshal(b, &cases)
	r := NewRandomState(0)
	shuf := func(n int) []int {
		l := make([]int, n)
		for i := range l {
			l[i] = i
		}
		r.ShuffleLen(n, func(i, j int) { l[i], l[j] = l[j], l[i] })
		return l
	}
	for i, c := range cases {
		switch c[0].(string) {
		case "randint1":
			if got := r.RandInt1(int(c[1].(float64))); got != int(c[2].(float64)) {
				t.Fatalf("%d randint %v got %d", i, c, got)
			}
		case "shuffle8", "shuffle3":
			n := 8
			if c[0] == "shuffle3" {
				n = 3
			}
			got := shuf(n)
			for k, v := range c[1].([]any) {
				if got[k] != int(v.(float64)) {
					t.Fatalf("%d shuffle %v got %v", i, c, got)
				}
			}
		case "choice":
			p := []float64{0, 3, 1, 0, 2, 5}
			for k := range p {
				p[k] /= 11
			}
			if got := r.ChoiceP(p); got != int(c[1].(float64)) {
				t.Fatalf("%d choice %v got %d", i, c, got)
			}
		}
	}
}
