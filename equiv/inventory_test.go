package equiv

import (
	"testing"

	"aago/brain"
	"aago/internal/corpus"
	"aago/nle"
	"aago/snapshot"
)

// TestStateInventory lists what in the live agent state is not plain data (diagnostic, not a gate).
func TestStateInventory(t *testing.T) {
	dir := corpus.Dir()
	r, err := corpus.Open(corpus.Path(dir, 0))
	if err != nil {
		t.Skip(err)
	}
	defer r.Close()
	d := brain.NewDriver()
	defer d.Close()
	for i := 0; i < 3000; i++ {
		b, _, ok, _ := r.Next()
		if !ok {
			break
		}
		if i == 0 {
			d.Reset()
		}
		d.Act(nle.ObsFromWire(b))
	}
	_, err = snapshot.Save(d.Agent)
	t.Log(err)
}
