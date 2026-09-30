package brain

import (
	"strings"

	"aago/flow"
	. "aago/grid"
	. "aago/internal/pyrt"
	. "aago/nle"
	"aago/sense"
	"aago/strategy"
	"aago/world"
)

// mainF is Agent.main(): the initial checks, then forever: ESC, ESC, reset the knowledge that a panic may
// have left inconsistent (on_panic), and run the global strategy; an AgentPanic starts the loop over.
type mainF struct {
	InitFinished         bool
	LastStep, Inactivity int
	PC                   int
}

const (
	mainInit      = iota // start the init (inside try)
	mainInitTry          // the init runs
	mainAfterInit        // after the init's try
	mainLoop             // loop head
	mainEsc2             // first ESC done (inside try and the inner block with `finally: last_step = ...`)
	mainOnPanic          // second ESC done
	mainStrategy         // on_panic done
	mainRunning          // the global strategy runs (inside try)
)

func (s *mainF) Resume(c *Ctx) Cmd {
	a := c.W
	for {
		switch s.PC {
		case mainInit:
			s.PC = mainInitTry
			return call(sense.Atom(&mainInitF{M: s}))
		case mainInitTry:
			s.PC = mainAfterInit
		case mainAfterInit:
			Assert(s.InitFinished)
			s.LastStep = a.StepCount
			s.Inactivity = 0
			s.PC = mainLoop
		case mainLoop:
			s.Inactivity++
			if a.StepCount != s.LastStep {
				s.Inactivity = 0
			}
			if s.Inactivity >= 5 {
				Raise("RuntimeError", "Cyclic Panic")
			}
			s.PC = mainEsc2
			return call(a.Term.Key(A_Command_ESC))
		case mainEsc2:
			s.PC = mainOnPanic
			return call(a.Term.Key(A_Command_ESC))
		case mainOnPanic:
			s.PC = mainStrategy
			return call(&onPanicF{})
		case mainStrategy:
			s.LastStep = a.StepCount
			s.PC = mainRunning
			return call(flow.RunNode(strategy.GlobalStrategy(a.GlobalLogic)))
		case mainRunning:
			Assert(false)
		}
	}
}

// Catch is the `except` of both try blocks (handle_exception: AgentPanic is swallowed).
func (s *mainF) Catch(c *Ctx, exc any) bool {
	if s.PC < mainInitTry || s.PC == mainAfterInit || s.PC == mainLoop {
		return false
	}
	if s.PC >= mainEsc2 && s.PC <= mainStrategy {
		s.LastStep = c.W.StepCount // the inner block's finally
	}
	if _, ok := exc.(*AgentPanic); !ok {
		return false
	}
	c.W.AllPanics = append(c.W.AllPanics, exc)
	if s.PC == mainInitTry {
		s.PC = mainAfterInit
	} else {
		s.PC = mainLoop
	}
	return true
}

// mainInitF is the atom operation at the start of main.
type mainInitF struct {
	M  *mainF
	PC int
}

func (s *mainInitF) Resume(c *Ctx) Cmd {
	a := c.W
	switch s.PC {
	case 0:
		s.PC = 1
		return call(a.Term.Key(A_Command_ESC))
	case 1:
		s.PC = 2
		return call(a.Term.Key(A_Command_ESC))
	case 2:
		a.CurrentLevel().StairDestination.Set(Pos{a.Blstats.Y, a.Blstats.X},
			world.StairDest{Level: world.LevelKey{world.Level_PLANE, 1}, PosNone: true})
		s.PC = 3
		return call(sense.Atom(sense.CheckAttributes()))
	case 3:
		s.PC = 4
		return call(sense.ParseEnhanceView())
	case 4:
		s.PC = 5
		return call(a.Term.Key(A_Command_AUTOPICKUP))
	case 5:
		s.PC = 6
		if strings.Contains(a.Message, "Autopickup: ON") {
			return call(a.Term.Key(A_Command_AUTOPICKUP))
		}
	}
	s.M.InitFinished = true
	return ret(nil)
}

// onPanicF is Agent.on_panic.
type onPanicF struct{ PC int }

func (s *onPanicF) Resume(c *Ctx) Cmd {
	a := c.W
	switch s.PC {
	case 0:
		s.PC = 1
		return call(sense.CheckTerrain(true))
	case 1:
		inv := a.Inventory
		inv.ItemsBelowMe, inv.ItemsBelowMeNone = nil, true
		inv.LettersBelowMe, inv.LettersBelowMeNone = nil, true
		inv.EngravingBelowMe, inv.EngravingBelowMeNone = "", true
		inv.PreviousBlstatsNone = true
		s.PC = 2
		return call(sense.UpdateObjectGlyphMapping()) // inventory.item_manager.on_panic()
	case 2:
		a.Inventory.Items.OnPanic()
		a.MonsterTracker.OnPanic()
		s.PC = 3
		return call(sense.UpdateState(true, true))
	}
	return ret(nil)
}
