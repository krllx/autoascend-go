package strategy

import (
	"sort"

	"aago/flow"
	. "aago/grid"
	. "aago/internal/pyrt"
	. "aago/nle"
	"aago/skills"
	"aago/world"
)

// Level-to-level strategies of exploration_logic.py as nodes.

// goToFactory builds the strategy that walks to (y, x) (the go_to_strategy argument of these strategies).
type goToFactory interface {
	make(a *world.Agent, y, x int) flow.Node
}

// globalGoTo is GlobalLogic's go_to_strategy.
type globalGoTo struct{}

func (globalGoTo) make(a *world.Agent, y, x int) flow.Node { return goToStrategy(a.GlobalLogic, y, x) }

// goToStrategyN is exploration go_to_strategy(y, x, **kwargs): walk there if it is reachable.
type goToStrategyN struct {
	Y, X int
	O    skills.GoToOpts
}

func (n goToStrategyN) Check(a *world.Agent) (bool, Frame) {
	d := a.Bfs()
	if pyAt32(&d, n.Y, n.X) == -1 || (a.Blstats.Y == n.Y && a.Blstats.X == n.X) {
		return false, nil
	}
	return true, flow.ThenNil(skills.GoTo(n.Y, n.X, n.O))
}

// waitForPetF is _wait_for_pet_and_move(dir): wait a turn at a time until the pet is adjacent, then move.
type waitForPetF struct {
	Dir string
	PC  int
}

func (s *waitForPetF) Resume(c *Ctx) Cmd {
	a := c.W
	if s.PC == 1 {
		return ret(nil)
	}
	if a.HasPet() {
		by, bx := a.Blstats.Y, a.Blstats.X
		found := false
		for y := max(by-1, 0); y < min(by+2, SY); y++ {
			for x := max(bx-1, 0); x < min(bx+2, SX); x++ {
				if G_PETS.Has(int(a.Glyphs()[y][x])) {
					found = true
				}
			}
		}
		if !found {
			return call(skills.Move("."))
		}
	}
	s.PC = 1
	return call(skills.Move(s.Dir))
}

// exploreStairsN is explore_stairs(go_to_strategy, all): take a random unexplored staircase.
type exploreStairsN struct {
	F   goToFactory
	All bool
}

func (n exploreStairsN) Check(a *world.Agent) (bool, Frame) {
	unexploredStairs := a.Exploration.GetUnexploredStairs(nil, false, false, false, n.All)
	if len(unexploredStairs) == 0 {
		return false, nil
	}
	return true, &exploreStairsF{Stairs: unexploredStairs, F: n.F}
}

type exploreStairsF struct {
	Stairs []Pos
	F      goToFactory
	Y, X   int
	Dir    string
	PC     int
}

func (s *exploreStairsF) Resume(c *Ctx) Cmd {
	a := c.W
	switch s.PC {
	case 0:
		p := s.Stairs[a.Rng.RandInt(0, len(s.Stairs))]
		s.Y, s.X = p.Y, p.X
		glyph := int(a.CurrentLevel().Objects[s.Y][s.X])
		if G_STAIR_DOWN.Has(glyph) {
			s.Dir = ">"
		} else if G_STAIR_UP.Has(glyph) {
			s.Dir = "<"
		}
		Assert(s.Dir != "", glyph) // TODO: portals
		s.PC = 1
		return call(flow.RunNode(s.F.make(a, s.Y, s.X)))
	case 1:
		Assert(a.Blstats.Y == s.Y && a.Blstats.X == s.X)
		s.PC = 2
		return call(&waitForPetF{Dir: s.Dir})
	}
	return ret(nil)
}

// followLevelPathN is follow_level_path_strategy(path, go_to_strategy): take the stairs along the path.
type followLevelPathN struct {
	Path []world.LevelPathStep
	F    goToFactory
}

func (n followLevelPathN) Check(a *world.Agent) (bool, Frame) {
	if len(n.Path) == 0 {
		return false, nil
	}
	return true, &followLevelPathF{Path: n.Path, F: n.F}
}

type followLevelPathF struct {
	Path []world.LevelPathStep
	F    goToFactory
	I    int
	PC   int // 0: go to the stairs of Path[I], 1: there, 2: stairs taken
}

func (s *followLevelPathF) Resume(c *Ctx) Cmd {
	a := c.W
	switch s.PC {
	case 1:
		st := s.Path[s.I]
		Assert(a.Blstats.Y == st.Pos.Y && a.Blstats.X == st.Pos.X)
		s.PC = 2
		return call(&waitForPetF{Dir: st.Dir})
	case 2:
		s.I++
	}
	if s.I == len(s.Path) {
		return ret(nil)
	}
	st := s.Path[s.I]
	s.PC = 1
	return call(flow.RunNode(s.F.make(a, st.Pos.Y, st.Pos.X)))
}

// goToLeastExploredLevelN is go_to_least_explored_level: go to the level on the way that is searched least.
type goToLeastExploredLevelN struct {
	DN, LN int
	F      goToFactory
}

func (n goToLeastExploredLevelN) Check(a *world.Agent) (bool, Frame) {
	e := a.Exploration
	levelsToSearch := e.LevelsToExploreToGetTo(n.DN, n.LN, nil)
	if levelsToSearch.Len() == 0 {
		return false, nil
	}
	explorationLevels := NewOMap[SetKey, int]()
	for _, level := range levelsToSearch.Items() {
		sc := &a.Levels.MustGet(world.LevelKeyOf(level)).SearchCount
		s := 0
		for y := 0; y < SY; y++ {
			for x := 0; x < SX; x++ {
				s += int(sc[y][x])
			}
		}
		explorationLevels.Set(level, s)
	}
	sorted := levelsToSearch.Items()
	sort.SliceStable(sorted, func(i, j int) bool {
		if sorted[i].A != sorted[j].A {
			return sorted[i].A < sorted[j].A
		}
		return sorted[i].B < sorted[j].B
	})
	for _, level := range sorted {
		lk := world.LevelKeyOf(level)
		if len(e.GetUnexploredStairs(&lk, false, false, false, true)) > 0 {
			explorationLevels.Set(level, explorationLevels.MustGet(level)-10000)
		}
	}
	vals := explorationLevels.Values()
	minExplorationLevel := vals[0]
	for _, v := range vals[1:] {
		if v < minExplorationLevel {
			minExplorationLevel = v
		}
	}
	cur := world.LevelSetKey(a.CurrentLevel().Key())
	if levelsToSearch.Contains(cur) &&
		explorationLevels.MustGet(world.LevelSetKey(a.CurrentLevel().Key())) < minExplorationLevel+150 {
		return false, nil
	}
	var targetLevel world.LevelKey
	found := false
	for _, level := range levelsToSearch.Items() {
		if explorationLevels.MustGet(level) == minExplorationLevel {
			targetLevel = world.LevelKeyOf(level)
			found = true
			break
		}
	}
	return true, &goToLeastExploredLevelF{Target: targetLevel, Found: found, F: n.F}
}

type goToLeastExploredLevelF struct {
	Target world.LevelKey
	Found  bool
	F      goToFactory
	PC     int
}

func (s *goToLeastExploredLevelF) Resume(c *Ctx) Cmd {
	a := c.W
	if s.PC == 0 {
		Assert(s.Found)
		path, ok := a.Exploration.GetPathToLevel(s.Target.Dungeon, s.Target.Level)
		Assert(ok)
		s.PC = 1
		return call(flow.RunNode(followLevelPathN{Path: path, F: s.F}))
	}
	Assert(a.CurrentLevel().Key() == s.Target)
	return ret(nil)
}

// levelsContainCurrent is the condition `current level in levels_to_search` (of one loop iteration).
type levelsContainCurrent struct{ Levels *PySet }

func (p levelsContainCurrent) Eval(a *world.Agent) bool {
	return p.Levels.Contains(world.LevelSetKey(a.CurrentLevel().Key()))
}

// goToLevelN is go_to_level_strategy(dungeon, level, go_to_strategy, explore_strategy): explore the levels
// on the way (taking unexplored stairs, moving to the least explored level), then follow the path.
type goToLevelN struct {
	DN, LN  int
	F       goToFactory
	Explore flow.Node
}

func (n *goToLevelN) Check(a *world.Agent) (bool, Frame) { return true, &goToLevelF{N: n} }

type goToLevelF struct {
	N      *goToLevelN
	Levels *PySet // levels_to_search of the current iteration
	PC     int    // 0: loop head, 1: moved to the least explored level, 2: explored, 3: path followed
}

func (s *goToLevelF) Resume(c *Ctx) Cmd {
	a := c.W
	n := s.N
	switch s.PC {
	case 1:
		Assert(s.Levels.Contains(world.LevelSetKey(a.CurrentLevel().Key())))
	case 3:
		Assert(a.CurrentLevel().Key() == world.LevelKey{n.DN, n.LN})
		return ret(nil)
	}
	s.Levels = a.Exploration.LevelsToExploreToGetTo(n.DN, n.LN, nil)
	if s.Levels.Len() == 0 {
		path, ok := a.Exploration.GetPathToLevel(n.DN, n.LN)
		Assert(ok, a.CurrentLevel().Key(), n.DN, n.LN)
		s.PC = 3
		return call(flow.RunNode(followLevelPathN{Path: path, F: n.F}))
	}
	if !s.Levels.Contains(world.LevelSetKey(a.CurrentLevel().Key())) {
		s.PC = 1
		return call(flow.RunNode(goToLeastExploredLevelN{DN: n.DN, LN: n.LN, F: n.F}))
	}
	s.PC = 2
	return call(flow.RunNode(flow.Preempt(n.Explore, []flow.Node{
		flow.If(levelsContainCurrent{s.Levels}, exploreStairsN{F: n.F, All: true}),
		goToLeastExploredLevelN{DN: n.DN, LN: n.LN, F: n.F},
	}, false)))
}
