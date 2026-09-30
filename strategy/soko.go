package strategy

import (
	"sort"
	"strings"

	"aago/flow"
	. "aago/grid"
	. "aago/internal/pyrt"
	. "aago/nle"
	"aago/sense"
	"aago/skills"
	"aago/tactics"
	"aago/world"
)

// solve_sokoban_strategy and push_bolder (global_logic.py) as a node and frames.

type solveSokobanN struct{}

func (solveSokobanN) Check(a *world.Agent) (bool, Frame) {
	// TODO: refactor
	if !AnyIn(&a.CurrentLevel().Objects, G_TRAPS) {
		return false, nil
	}
	return true, &solveSokobanF{}
}

// sokoClear is the go_to callback of the Sokoban solver: forget the tiles already seen; stop when the
// target is seen.
type sokoClear struct {
	Mask   *BoolGrid
	VY, VX int
}

func (cb sokoClear) Check(a *world.Agent) bool {
	cb.Mask[a.Blstats.Y][a.Blstats.X] = false
	vf := Isin(a.Glyphs(), G_VISIBLE_FLOOR)
	for yy := 0; yy < SY; yy++ {
		for xx := 0; xx < SX; xx++ {
			if vf[yy][xx] {
				cb.Mask[yy][xx] = false
			}
		}
	}
	return !cb.Mask[cb.VY][cb.VX]
}

// solveSokobanF plays the known solution of the current Sokoban level; it looks for mimics when the level
// does not match the solution, and gives up (next milestone, AgentPanic) when stuck.
type solveSokobanF struct {
	Map            *sokoMap
	Answer         [][4]int
	Off0, Off1     int
	PossibleMimics *PySet
	LastResort     *[4]int
	AI             int // index in Answer
	Y, X, DY, DX   int // the current answer move
	TY, TX         int
	Mask, Boulders npB2 // the boulders seen and the solution's boulders, at the start of this move
	ToVisit        *BoolGrid
	Mims           []SetKey
	MI, SearchI    int
	PC             int
}

const (
	sokoStart      = iota // find the map
	sokoMove              // next answer move
	sokoVisit             // visit the tiles that the push will cut off
	sokoPushed            // push_bolder returned
	sokoAfterMove         // the mimic bookkeeping after a move
	sokoMimics            // look for mimics
	sokoSearch            // search next to a possible mimic
	sokoLastResort        // the last resort push is done
)

func (s *solveSokobanF) Resume(c *Ctx) Cmd {
	a := c.W
	for {
		switch s.PC {
		case sokoStart:
			wallMap := Isin(&a.CurrentLevel().Objects, G_WALL)
			found := false
			for mi := range SokoMapsData {
				md := &SokoMapsData[mi]
				sm := sokoConvertMap(md)
				wnz := wallMap.Nonzero()
				if len(wnz) == 0 {
					Raise("ValueError", "min() arg is an empty sequence")
				}
				swalls := sm.eq(Soko_WALL)
				snz := swalls.nonzero()
				if len(snz) == 0 {
					Raise("ValueError", "min() arg is an empty sequence")
				}
				s.Off0, s.Off1 = wnz[0].Y-snz[0].Y, wnz[0].X-snz[0].X
				mask := npSliceGrid(&wallMap, s.Off0, s.Off0+sm.h, s.Off1, s.Off1+sm.w)
				if npBin(npBin(mask, swalls, npAnd), mask, npEq).all() {
					s.Map = sm
					s.Answer = md.Answer
					found = true
					break
				}
			}
			Assert(found, "sokomap not found")
			s.PossibleMimics = NewPySet()
			s.LastResort = nil
			s.AI = 0
			s.PC = sokoMove
		case sokoMove:
			if s.AI == len(s.Answer) {
				s.PC = sokoMimics
				s.Mims = s.PossibleMimics.Items()
				sort.SliceStable(s.Mims, func(i, j int) bool {
					if s.Mims[i].A != s.Mims[j].A {
						return s.Mims[i].A < s.Mims[j].A
					}
					return s.Mims[i].B < s.Mims[j].B
				})
				s.MI = 0
				continue
			}
			ans := s.Answer[s.AI]
			s.Y, s.X, s.DY, s.DX = ans[0], ans[1], ans[2], ans[3]
			sm := s.Map
			boulderMap := Isin(a.Glyphs(), G_BOULDER)
			s.Mask = npSliceGrid(&boulderMap, s.Off0, s.Off0+sm.h, s.Off1, s.Off1+sm.w)
			s.TY, s.TX = s.Off0+s.Y-s.DY, s.Off1+s.X-s.DX
			s.Boulders = sm.eq(Soko_BOULDER)
			d := a.Bfs()
			if pyAt32(&d, s.TY, s.TX) != -1 &&
				npBin(npBin(s.Boulders, s.Mask, npOr), s.Boulders, npEq).all() &&
				G_BOULDER.Has(pyAt16(a.Glyphs(), s.TY+s.DY, s.TX+s.DX)) {

				sokoDis1 := sm.bfs()
				sm.move(s.Y, s.X, s.DY, s.DX)
				sokoDis2 := sm.bfs()

				// see points that will no longer be accessible
				d2 := a.Bfs()
				s.ToVisit = new(BoolGrid)
				for yy := 0; yy < SY; yy++ {
					for xx := 0; xx < SX; xx++ {
						s.ToVisit[yy][xx] = d2[yy][xx] != -1
					}
				}
				// to_visit_mask[slice] &= (soko_dis1 != -1) & (soko_dis2 == -1)
				ylo, yhi := npSliceBounds(s.Off0, s.Off0+sm.h, SY)
				xlo, xhi := npSliceBounds(s.Off1, s.Off1+sm.w, SX)
				if yhi-ylo != sm.h || xhi-xlo != sm.w {
					Raise("ValueError", "non-broadcastable output operand")
				}
				for i := 0; i < sm.h; i++ {
					for j := 0; j < sm.w; j++ {
						k := i*sm.w + j
						s.ToVisit[ylo+i][xlo+j] = s.ToVisit[ylo+i][xlo+j] && (sokoDis1[k] != -1 && sokoDis2[k] == -1)
					}
				}
				s.PC = sokoVisit
				continue
			}
			s.Map.move(s.Y, s.X, s.DY, s.DX)
			s.PC = sokoAfterMove
		case sokoVisit:
			if s.ToVisit.Any() {
				v := s.ToVisit.Nonzero()[0]
				s.ToVisit[v.Y][v.X] = false
				return call(skills.GoTo(v.Y, v.X, skills.GoToOpts{Callback: sokoClear{Mask: s.ToVisit, VY: v.Y, VX: v.X}}))
			}
			s.PC = sokoPushed
			return call(&pushBolderF{TY: s.TY, TX: s.TX, DY: s.DY, DX: s.DX})
		case sokoPushed:
			if !c.Ret.(bool) {
				s.AI++
				s.PC = sokoMove
				continue
			}
			s.PossibleMimics = NewPySet()
			s.LastResort = nil
			if !AnyIn(&a.CurrentLevel().Objects, G_TRAPS) {
				return ret(nil)
			}
			s.PC = sokoAfterMove
		case sokoAfterMove:
			if npBin(s.Boulders.not(), s.Mask, npOr).all() {
				d3 := a.Bfs()
				if pyAt32(&d3, s.TY, s.TX) != -1 &&
					G_BOULDER.Has(pyAt16(a.Glyphs(), s.TY+s.DY, s.TX+s.DX)) &&
					s.LastResort == nil {
					s.LastResort = &[4]int{s.TY, s.TX, s.DY, s.DX}
				}
				if s.PossibleMimics.Len() == 0 {
					for _, p := range npBin(s.Boulders.not(), s.Mask, npAnd).nonzero() {
						s.PossibleMimics.Add(PairKey(p.Y+s.Off0, p.X+s.Off1))
					}
				}
			}
			s.AI++
			s.PC = sokoMove
		case sokoMimics: // sokoban configuration is not in the list. Check for mimics
			for s.MI < len(s.Mims) {
				mimY, mimX := s.Mims[s.MI].A, s.Mims[s.MI].B
				s.MI++
				d := a.Bfs()
				ylo, yhi := npSliceBounds(max(0, mimY-1), mimY+2, SY)
				xlo, xhi := npSliceBounds(max(0, mimX-1), mimX+2, SX)
				anyR := false
				for yy := ylo; yy < yhi; yy++ {
					for xx := xlo; xx < xhi; xx++ {
						if d[yy][xx] != -1 {
							anyR = true
						}
					}
				}
				if anyR {
					s.SearchI = 0
					s.PC = sokoSearch
					return call(skills.GoTo(mimY, mimX, skills.GoToOpts{StopOneBefore: true}))
				}
			}
			// last resort move (sometimes we can see a mimic diagonally but cannot reach it)
			if s.LastResort != nil {
				m := *s.LastResort
				s.PC = sokoLastResort
				return call(&pushBolderF{TY: m[0], TX: m[1], DY: m[2], DX: m[3]})
			}
			sokoGiveUp(a)
		case sokoSearch:
			if s.SearchI < 3 {
				s.SearchI++
				return call(skills.SearchTimes(1))
			}
			s.PC = sokoMimics
		case sokoLastResort:
			// Not in the Python bot: stop when this push plugged the last hole (Python checks only after the
			// pushes of the solution, so it went on to a push that could not work), and give the level up when
			// the push failed (it would be chosen again every round).
			if !AnyIn(&a.CurrentLevel().Objects, G_TRAPS) {
				return ret(nil)
			}
			if !c.Ret.(bool) {
				sokoGiveUp(a)
			}
			s.PC = sokoStart
		}
	}
}

// sokoGiveUp leaves the level unsolved: the next milestone, and AgentPanic.
func sokoGiveUp(a *world.Agent) {
	g := a.GlobalLogic
	a.StatsLogger.LogEvent("sokoban_dropped")
	g.Milestone = world.MilestoneNext(g.Milestone)
	Panic("sokomap unsolvable")
}

// A boulder that does not move because of a monster behind it: push_bolder explores sokoWaitTurns turns and
// pushes again, sokoPushTries pushes in all.
const (
	sokoPushTries = 3
	sokoWaitTurns = 50
)

type turnAtLeast struct{ T int }

func (p turnAtLeast) Eval(a *world.Agent) bool { return a.Blstats.Time >= p.T }

// pushBolderF is push_bolder(ty, tx, dy, dx): stand at (ty, tx) and push; returns whether it pushed.
type pushBolderF struct {
	TY, TX, DY, DX int
	Direction      string
	Message        string
	Pickaxe        *world.Item
	Fails          int // pushes that did not move the boulder
	PC             int // 0: loop head, 1: at the boulder, 2: pushed, 3: walked behind, 4: pick-axe, 5: explored
}

func (s *pushBolderF) Resume(c *Ctx) Cmd {
	a := c.W
	for {
		switch s.PC {
		case 0:
			d := a.Bfs()
			if pyAt32(&d, s.TY, s.TX) == -1 {
				return ret(false)
			}
			s.PC = 1
			return call(skills.GoTo(s.TY, s.TX, skills.GoToOpts{}))
		case 1:
			s.PC = 2
			return call(sense.Atom(&pushF{P: s}))
		case 2:
			if !(a.Blstats.Y == s.TY && a.Blstats.X == s.TX) {
				return ret(true)
			}
			// Not in the Python bot: a monster the hero sees ("There's a giant mimic on the other side.") is one
			// behind the boulder too; Python failed the assert on it.
			monster := strings.Contains(s.Message, "You hear a monster behind the boulder.") ||
				strings.Contains(s.Message, " on the other side.")
			Assert(monster || strings.Contains(s.Message, "You try to move the boulder, but in vain."), s.Message)
			d2 := a.Bfs()
			if pyAt32(&d2, s.TY+s.DY, s.TX+s.DX) != -1 {
				s.PC = 3
				return call(skills.GoTo(s.TY+s.DY, s.TX+s.DX, skills.GoToOpts{}))
			}
			s.Pickaxe = nil
			for item := range world.EachItem(a.Inventory.Items.AllItems) {
				if item.IsUnambiguous() {
					o := item.Object()
					if o == O_FromName1("pick-axe") || o == O_FromName1("dwarvish mattock") {
						s.Pickaxe = item
						break
					}
				}
			}
			if s.Pickaxe != nil {
				s.PC = 4
				return call(sense.Atom(&digF{P: s}))
			}
			// Not in the Python bot: give the push up when waiting cannot help. A boulder or a wall behind stays
			// there; for a monster the bot explores sokoWaitTurns turns and pushes again, sokoPushTries times.
			// Python explored without a limit and pushed again forever (a mimic behind never goes away).
			s.Fails++
			if !monster || s.Fails >= sokoPushTries {
				return ret(false)
			}
			// TODO: not sure what to do
			o := world.DefaultExploreOpts()
			o.SearchPrioLimit = nil
			s.PC = 5
			return call(flow.RunNode(flow.Until(tactics.Explore1(a, o), turnAtLeast{a.Blstats.Time + sokoWaitTurns})))
		case 3, 5:
			s.PC = 0
		case 4:
			return ret(true)
		}
	}
}

// pushF is the atom operation of the push: one step towards the boulder.
type pushF struct {
	P  *pushBolderF
	PC int
}

func (s *pushF) Resume(c *Ctx) Cmd {
	p := s.P
	if s.PC == 0 {
		p.Direction = world.CalcDirection(p.TY, p.TX, p.TY+p.DY, p.TX+p.DX, false)
		s.PC = 1
		return call(skills.Direction(p.Direction))
	}
	p.Message = c.W.Message
	return ret(nil)
}

// digF applies the pick-axe in the push direction.
type digF struct {
	P  *pushBolderF
	PC int
}

func (s *digF) Resume(c *Ctx) Cmd {
	a := c.W
	p := s.P
	switch s.PC {
	case 0:
		s.PC = 1
		return call(skills.MoveToInventory(p.Pickaxe))
	case 1:
		p.Pickaxe = c.Ret.(*world.Item)
		s.PC = 2
		return call(a.Term.Key(A_Command_APPLY))
	case 2:
		s.PC = 3
		return call(sense.TypeText(a.Inventory.Items.GetLetter(p.Pickaxe)))
	case 3:
		s.PC = 4
		return call(skills.Direction(p.Direction))
	}
	return ret(nil)
}
