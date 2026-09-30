package world

import (
	. "aago/grid"
	"aago/snapshot"
)

// Values that frames keep in `any` fields (routine results on their way up: c.Ret, atom results). Named types
// with methods are registered in snapshot_types_gen.go (go run ./tools/genreg <package dirs>).
func init() {
	snapshot.Register(false, 0, "", (*Item)(nil), []*Item(nil), Pos{}, []Pos(nil), []SeenMonster(nil))
}
