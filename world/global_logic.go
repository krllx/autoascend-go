package world

import (
	"math"

	. "aago/internal/pyrt"
	. "aago/nle"
)

// Port of global_logic.py GlobalLogic (sokoban solving is in logic_soko.go).
//
// GlobalStrategy is the bot's top-level plan: currentStrategy walks the milestones (explore the
// first level, reach the Gnomish Mines, Minetown, Sokoban, ...), and each Preempt layer above it can
// interrupt everything below when its entry check passes (emergency beats fighting beats eating beats
// exploring). Conventions used here and across the package:
//   - a Strategy's Gen is its entry check: `return Skip()` when it does not apply now, `return
//     Do(func() {...})` when it does (see strategy.go); the check itself must not act in the game;
//   - Panic(...) is AutoAscend's AgentPanic: the agent presses ESC and restarts GlobalStrategy;
//   - Assert/Raise failures and Go runtime errors (nil pointer, index out of range) are fatal: the
//     bot stops and sends only ESC until the game ends.

func negInf() float64 { return math.Inf(-1) }

// Milestone IntEnum values.
const (
	Milestone_BE_ON_FIRST_LEVEL  = 1
	Milestone_FIND_GNOMISH_MINES = 2
	Milestone_FIND_MINETOWN      = 3
	Milestone_FIND_SOKOBAN       = 4
	Milestone_SOLVE_SOKOBAN      = 5
	Milestone_FIND_MINES_END     = 6
	Milestone_GO_DOWN            = 7
)

// milestoneNext mirrors Milestone(int(self.milestone) + 1) (ValueError past the last member).
func MilestoneNext(m int) int {
	n := m + 1
	if n < Milestone_BE_ON_FIRST_LEVEL || n > Milestone_GO_DOWN {
		Raise("ValueError", "%d is not a valid Milestone", n)
	}
	return n
}

type globalLogicState struct {
	oracleLevel   *LevelKey // None -> nil
	MinetownLevel *LevelKey // None -> nil
	GotArtifact   bool
}

// NewGlobalLogic mirrors GlobalLogic.__init__.
func NewGlobalLogic(a *Agent) *GlobalLogic {
	g := &GlobalLogic{Agent: a, Milestone: Milestone_BE_ON_FIRST_LEVEL}
	g.ItemPriority = NewItemPriority(a)
	g.St = &globalLogicState{}
	return g
}

// Update mirrors GlobalLogic.update.
func (g *GlobalLogic) Update() {
	a := g.Agent
	if !a.Character.Prop.Hallu() {
		if AnyIn(a.Glyphs(), G_ORACLE) {
			if g.St.oracleLevel == nil {
				k := a.CurrentLevel().Key()
				g.St.oracleLevel = &k
			} else {
				Assert(*g.St.oracleLevel == a.CurrentLevel().Key())
			}
		}

		if a.CurrentLevel().DungeonNumber == Level_GNOMISH_MINES && AnyIn(a.Glyphs(), G_SHOPKEEPER) {
			if g.St.MinetownLevel == nil {
				k := a.CurrentLevel().Key()
				g.St.MinetownLevel = &k
			} else {
				Assert(*g.St.MinetownLevel == a.CurrentLevel().Key())
			}
		}
	}
}

// pySliceEnd mirrors the stop index normalization of lst[:n].
func PySliceEnd(n, length int) int {
	if n < 0 {
		n += length
		if n < 0 {
			n = 0
		}
	}
	if n > length {
		n = length
	}
	return n
}

// CanSacrify mirrors GlobalLogic.can_sacrify.
func (g *GlobalLogic) CanSacrify(item *Item) bool {
	a := g.Agent
	if !item.IsCorpse() || item.Comment == "old" {
		return false
	}

	if item.MonsterID == -1 {
		Raise("TypeError", "unsupported operand type(s) for +: 'NoneType' and 'int'")
	}
	mname := MON_Permonst(item.MonsterID + NH_GLYPH_MON_OFF).Mname
	role := a.Character.Role
	if (mname == "pony" && (role == Character_KNIGHT || role == Character_BARBARIAN)) ||
		(mname == "kitten" && false) || // role == [BARBARIAN, WIZARD] compares an int to a list
		(mname == "little dog" && item.Naming != "") { // little dogs are always named
		// sufficient condition for being an initial pet
		return false
	}

	if a.Character.Alignment != Character_CHAOTIC {
		var m int
		switch a.Character.Race {
		case Character_HUMAN:
			m = MON_M2_HUMAN | MON_M2_WERE
		case Character_DWARF:
			m = MON_M2_DWARF
		case Character_ELF:
			m = MON_M2_ELF
		case Character_GNOME:
			m = MON_M2_GNOME
		case Character_ORC:
			m = MON_M2_ORC
		default:
			Raise("KeyError", "%d", a.Character.Race)
		}
		f2 := MON_Permonst(item.MonsterID + NH_GLYPH_MON_OFF).Mflags2
		if (f2 & m) > 0 {
			return false
		}
	}

	return true
}

type GlobalLogic struct {
	Agent        *Agent
	Milestone    int
	ItemPriority *ItemPriority
	St           *globalLogicState
}
