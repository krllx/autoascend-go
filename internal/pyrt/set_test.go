package pyrt

import (
	"encoding/json"
	"os"
	"testing"
)

func TestPySetOrder(t *testing.T) {
	b, err := os.ReadFile("/tmp/setcases.json")
	if err != nil {
		t.Skip("no cases")
	}
	var cases []struct {
		Ops   [][]any `json:"ops"`
		Order []any   `json:"order"`
	}
	json.Unmarshal(b, &cases)
	key := func(v any) SetKey {
		switch x := v.(type) {
		case float64:
			return IntKey(int(x))
		case []any:
			return PairKey(int(x[0].(float64)), int(x[1].(float64)))
		}
		panic("bad")
	}
	for ci, c := range cases {
		s := NewPySet()
		for _, op := range c.Ops {
			switch op[0].(string) {
			case "add", "addi":
				s.Add(key(op[1]))
			case "discard":
				s.Discard(key(op[1]))
			case "union":
				s = s.Union(NewPySet(key(op[1])))
			}
		}
		got := s.Items()
		if len(got) != len(c.Order) {
			t.Fatalf("case %d len %d vs %d", ci, len(got), len(c.Order))
		}
		for i, v := range c.Order {
			if got[i] != key(v) {
				t.Fatalf("case %d pos %d: %v vs %v", ci, i, got[i], key(v))
			}
		}
	}
}
