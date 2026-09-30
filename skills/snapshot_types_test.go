package skills

import (
	"testing"

	"aago/internal/genreg"
)

// The snapshot type registry must list every type with methods (regenerate: go run ./tools/genreg skills).
func TestSnapshotTypesUpToDate(t *testing.T) {
	stale, err := genreg.Stale(".")
	if err != nil {
		t.Fatal(err)
	}
	if stale {
		t.Fatal("skills/" + genreg.FileName + " is stale: go run ./tools/genreg skills")
	}
}
