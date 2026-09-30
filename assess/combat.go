package assess

import (
	"math"
	"strings"

	. "aago/grid"
	. "aago/internal/pyrt"
	. "aago/nle"
	"aago/world"
)

// Port of combat/monster_utils.py, combat/utils.py and combat/movement_priority.py.

// heuristic monster types lists (ONLY_RANGED_SLOW_MONSTERS is in skel_logic.go)
var EXPLODING_MONSTERS = []string{"yellow light", "gas spore", "flaming sphere", "freezing sphere", "shocking sphere"}

var INSECTS = []string{"giant ant", "killer bee", "soldier ant", "fire ant", "giant beetle", "queen bee"}

var WEAK_MONSTERS = []string{"lichen", "newt", "shrieker", "grid bug"}

var WEIRD_MONSTERS = []string{"leprechaun", "nymph"}

// IsMonsterFaster mirrors is_monster_faster.
func IsMonsterFaster(a *world.Agent, m world.Monster) bool {
	n := m.Mon.Mname
	// TOOD: implement properly
	return strings.Contains(n, "bat") || strings.Contains(n, "dog") || strings.Contains(n, "cat") ||
		strings.Contains(n, "kitten") || strings.Contains(n, "pony") || strings.Contains(n, "horse") ||
		strings.Contains(n, "bee") || strings.Contains(n, "fox")
}

// ImminentDeathOnMelee mirrors imminent_death_on_melee.
func ImminentDeathOnMelee(a *world.Agent, m world.Monster) bool {
	if IsDangerousMonster(m) {
		return a.Blstats.Hitpoints <= 16
	}
	return a.Blstats.Hitpoints <= 8
}

// IsDangerousMonster mirrors is_dangerous_monster.
func IsDangerousMonster(m world.Monster) bool {
	n := m.Mon.Mname
	isPet := strings.Contains(n, "dog") || strings.Contains(n, "cat") || strings.Contains(n, "kitten") ||
		strings.Contains(n, "pony") || strings.Contains(n, "horse")
	return isPet || Contains(INSECTS, n)
}

// ConsiderMeleeOnlyRangedIfHpFull mirrors consider_melee_only_ranged_if_hp_full.
func ConsiderMeleeOnlyRangedIfHpFull(a *world.Agent, m world.Monster) bool {
	n := m.Mon.Mname
	return (n == "brown mold" || n == "blue jelly") && a.Blstats.Hitpoints == a.Blstats.MaxHitpoints
}

// ---- combat/utils.py

func wieldingRangedWeapon(a *world.Agent) bool {
	for _, item := range a.Inventory.Items.AllItems {
		if item.IsLauncher() && item.Equipped {
			return true
		}
	}
	return false
}

func lineDisFrom(a *world.Agent, y, x int) int {
	return max(Abs(a.Blstats.X-x), Abs(a.Blstats.Y-y))
}

func combatInside(y, x int) bool { return 0 <= y && y < SY && 0 <= x && x < SX }

// combatElbereth mirrors `agent.inventory.engraving_below_me.lower() == 'elbereth'`.
func combatElbereth(a *world.Agent) bool {
	if a.Inventory.EngravingBelowMeNone {
		Raise("AttributeError", "'NoneType' object has no attribute 'lower'")
	}
	return strings.ToLower(a.Inventory.EngravingBelowMe) == "elbereth"
}

func hasRangedCombinations(a *world.Agent) bool {
	return len(GetRangedCombinations(a.Inventory, nil, true, false, false, false, []*world.Item{})) > 0
}

// ---- combat/movement_priority.py

const (
	opAdd = 0
	opMax = 1
)

func drawAround(priority *F64Grid, y, x int, value float64, radius int, op int) {
	// TODO: optimize
	for y1 := y - radius; y1 < y+radius+1; y1++ {
		for x1 := x - radius; x1 < x+radius+1; x1++ {
			if max(Abs(y1-y), Abs(x1-x)) != radius {
				continue
			}
			if 0 <= y1 && y1 < SY && 0 <= x1 && x1 < SX {
				if op == opAdd {
					priority[y1][x1] += value
				} else {
					priority[y1][x1] = PyMaxF(priority[y1][x1], value)
				}
			}
		}
	}
}

func drawRanged(priority *F64Grid, y, x int, value float64, walkable *BoolGrid, radius int, op int) {
	// TODO: optimize
	for _, directionY := range []int{-1, 0, 1} {
		for _, directionX := range []int{-1, 0, 1} {
			if directionY != 0 || directionX != 0 {
				for i := 1; i < radius+1; i++ {
					y1 := y + directionY*i
					x1 := x + directionX*i
					if 0 <= y1 && y1 < SY && 0 <= x1 && x1 < SX {
						if !walkable[y1][x1] {
							break
						}
						if op == opAdd {
							priority[y1][x1] += value
						} else {
							priority[y1][x1] = PyMaxF(priority[y1][x1], value)
						}
					}
				}
			}
		}
	}
}

func drawMonsterPriorityPositive(a *world.Agent, m world.Monster, priority *F64Grid, walkable *BoolGrid) {
	y, x, mon := m.Y, m.X, m.Mon

	// don't move into the monster
	priority[y][x] = math.NaN()

	if Contains(WEAK_MONSTERS, mon.Mname) {
		// weak monster - freely engage in melee
		drawAround(priority, y, x, 2, 1, opMax)
		drawAround(priority, y, x, 1, 2, opMax)
	} else if strings.Contains(mon.Mname, "mold") && !Contains(ONLY_RANGED_SLOW_MONSTERS, mon.Mname) {
		if a.Blstats.Hitpoints >= 15 || a.Blstats.Hitpoints == a.Blstats.MaxHitpoints {
			// freely engage in melee
			drawAround(priority, y, x, 2, 1, opMax)
			drawAround(priority, y, x, 1, 2, opMax)
		}
		if hasRangedCombinations(a) {
			drawRanged(priority, y, x, 1, walkable, 7, opMax)
		}
	} else if Contains(ONLY_RANGED_SLOW_MONSTERS, mon.Mname) { // and agent.inventory.get_ranged_combinations():
		if ConsiderMeleeOnlyRangedIfHpFull(a, m) {
			drawAround(priority, y, x, 2, 1, opMax)
			drawAround(priority, y, x, 1, 2, opMax)
		}
		if hasRangedCombinations(a) {
			drawRanged(priority, y, x, 1, walkable, 7, opMax)
		}
	} else if strings.Contains(mon.Mname, "unicorn") {
		if a.Blstats.Hitpoints >= 15 || a.Blstats.Hitpoints == a.Blstats.MaxHitpoints {
			// freely engage in melee
			drawAround(priority, y, x, 2, 1, opMax)
			drawAround(priority, y, x, 1, 2, opMax)
		}
	} else {
		if !ImminentDeathOnMelee(a, m) && !wieldingRangedWeapon(a) {
			// engage, but ensure striking first if possible
			if mon.Mmove <= 12 {
				drawAround(priority, y, x, 3, 2, opMax)
			} else {
				drawAround(priority, y, x, 3, 3, opMax)
			}
		}
		if wieldingRangedWeapon(a) {
			drawRanged(priority, y, x, 4, walkable, 7, opMax)
		} else if hasRangedCombinations(a) {
			drawRanged(priority, y, x, 1, walkable, 7, opMax)
		}
	}
}

func drawMonsterPriorityNegative(a *world.Agent, m world.Monster, priority *F64Grid, walkable *BoolGrid) {
	y, x, mon := m.Y, m.X, m.Mon

	if ImminentDeathOnMelee(a, m) && !Contains(WEAK_MONSTERS, mon.Mname) &&
		!Contains(ONLY_RANGED_SLOW_MONSTERS, mon.Mname) {
		if mon.Mmove <= 12 {
			drawAround(priority, y, x, -10, 1, opAdd)
		} else {
			if Adjacent(Pos{a.Blstats.Y, a.Blstats.X}, Pos{y, x}) {
				// no point in running -- monster is fast
			} else {
				drawAround(priority, y, x, -10, 2, opAdd)
				drawAround(priority, y, x, -5, 1, opAdd)
			}
		}

		if !hasRangedCombinations(a) {
			// prefer avoiding being in line of fire
			drawRanged(priority, y, x, -1, walkable, 7, opAdd)
		}
	}

	if Contains(EXPLODING_MONSTERS, mon.Mname) {
		drawAround(priority, y, x, -10, 1, opAdd)
		if !Contains(ONLY_RANGED_SLOW_MONSTERS, mon.Mname) {
			drawAround(priority, y, x, -5, 2, opAdd)
		}
		drawRanged(priority, y, x, 4, walkable, 7, opAdd)
	} else if strings.Contains(mon.Mname, "mold") && !Contains(ONLY_RANGED_SLOW_MONSTERS, mon.Mname) {
		// prioritize staying in ranged weapons line of fire
		if hasRangedCombinations(a) {
			drawRanged(priority, y, x, 2, walkable, 7, opAdd)
		}
	} else if Contains(WEIRD_MONSTERS, mon.Mname) {
		// stay away
		drawAround(priority, y, x, -10, 1, opAdd)
		// prioritize staying in ranged weapons line of fire
		if hasRangedCombinations(a) {
			drawRanged(priority, y, x, 6, walkable, 7, opAdd)
		}
	} else if Contains(ONLY_RANGED_SLOW_MONSTERS, mon.Mname) { // and agent.inventory.get_ranged_combinations():
		// ignore
	} else if strings.Contains(mon.Mname, "unicorn") {
	} else {
		if !Contains(WEAK_MONSTERS, mon.Mname) {
			// engage, but ensure striking first if possible
			drawAround(priority, y, x, -9, 1, opAdd)
			if !hasRangedCombinations(a) {
				drawRanged(priority, y, x, -1, walkable, 7, opAdd)
			}
		}
	}

	if mon.Mname == "purple worm" && hasRangedCombinations(a) {
		drawAround(priority, y, x, -10, 1, opAdd)
	}
}
