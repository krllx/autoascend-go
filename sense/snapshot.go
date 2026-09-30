package sense

import (
	. "aago/grid"
	. "aago/internal/pyrt"
	"aago/snapshot"
)

// takeAllMonstersF returns the monsters seen, by position.
func init() { snapshot.Register((*OMap[Pos, string])(nil)) }
