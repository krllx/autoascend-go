package tactics

import (
	"math"
	"strings"

	"aago/flow"
	. "aago/grid"
	. "aago/nle"
	"aago/skills"
	"aago/world"
)

// Exploration tactics of exploration_logic.py as nodes: explore1 and its parts, traps, altars, patrol.

// checkCondition is Strategy.check_condition(): run the check, drop the body.
func checkCondition(a *world.Agent, n flow.Node) bool {
	ok, _ := n.Check(a)
	return ok
}

// ---- explore1

// explore1 is explore1(search_prio_limit, door_open_count, kick_doors, trap_search_offset,
// check_altar_alignment, fast_go_to): open doors, visit unexplored tiles, search for hidden passages, with
// item gathering and trap handling preempting it.
func Explore1(a *world.Agent, o world.ExploreOpts) flow.Node {
	return flow.Preempt(
		flow.Preempt(flow.Loop(openVisitSearchStepper{O: o, Limit: o.SearchPrioLimit}),
			[]flow.Node{GatherItems(a.Inventory), flow.Every(10, untrapTrapsN{})}, true),
		[]flow.Node{searchNeighborsForTrapsN{Offset: o.TrapSearchOffset}}, true)
}

// openNeighborDoorsN opens (or kicks) a closed door next to me.
type openNeighborDoorsN struct {
	DoorOpenCount int
	KickDoors     bool
}

func (n openNeighborDoorsN) Check(a *world.Agent) (bool, Frame) {
	// TODO: polymorphed into a handless creature, too heavy load to kick, using lockpicks
	for _, p := range a.Neighbors(a.Blstats.Y, a.Blstats.X, true, false) {
		if (int(a.CurrentLevel().DoorOpenCount[p.Y][p.X]) < n.DoorOpenCount || n.KickDoors) &&
			G_DOOR_CLOSED.Has(int(a.Glyphs()[p.Y][p.X])) {
			return true, flow.PosGuard(&openDoorsF{Y: p.Y, X: p.X, KickDoors: n.KickDoors})
		}
	}
	return false, nil
}

type openDoorsF struct {
	Y, X      int
	KickDoors bool
	I         int
	PC        int // 0: first try, 1: first try done, 2: retries, 3: kicking
}

func (s *openDoorsF) Resume(c *Ctx) Cmd {
	a := c.W
	switch s.PC {
	case 0:
		s.PC = 1
		return call(skills.OpenDoor(s.Y, s.X))
	case 1:
		if c.Ret.(bool) {
			return ret(nil)
		}
		if strings.Contains(a.Message, "locked") {
			s.PC = 3
			return s.kick(a)
		}
		s.PC = 2
		s.I = 1
		return call(skills.OpenDoor(s.Y, s.X))
	case 2:
		if c.Ret.(bool) {
			return ret(nil)
		}
		if s.I < 6 {
			s.I++
			return call(skills.OpenDoor(s.Y, s.X))
		}
		s.PC = 3
	}
	return s.kick(a)
}

func (s *openDoorsF) kick(a *world.Agent) Cmd {
	if s.KickDoors && G_DOOR_CLOSED.Has(int(a.Glyphs()[s.Y][s.X])) {
		return call(skills.Kick(s.Y, s.X))
	}
	return ret(nil)
}

// openVisitSearchStepper is one iteration of explore1's open_visit_search loop.
type openVisitSearchStepper struct {
	O     world.ExploreOpts
	Limit *int
}

func (s openVisitSearchStepper) Step(a *world.Agent) Frame {
	if s.O.CheckAltarAlignment && checkCondition(a, checkAltarN{}) {
		return flow.RunNode(checkAltarN{})
	}
	doors := openNeighborDoorsN{DoorOpenCount: s.O.DoorOpenCount, KickDoors: s.O.KickDoors}
	if checkCondition(a, doors) {
		return flow.RunNode(doors)
	}

	toVisit := exploreToVisit(a, s.O.DoorOpenCount)
	lim := 0
	if s.Limit != nil {
		lim = *s.Limit
	}
	_, toSearch := exploreToSearch(a, lim)

	if s.O.CheckAltarAlignment {
		altars := a.CurrentLevel().Altars
		for _, p := range altars.Keys() {
			if altars.MustGet(p) == world.Character_UNKNOWN {
				toVisit[p.Y][p.X] = true
			}
		}
	}

	// consider exploring tile only when there is a path to it
	dis := a.Bfs()
	reachable := dis.Ne(-1)
	toExplore := toVisit.Or(toSearch).And(reachable)

	var nonzero []Pos
	dynamicSearchFallback := false
	if !toExplore.Any() {
		dynamicSearchFallback = true
	} else {
		// find all closest to_explore tiles
		first := true
		var minDis int32
		for y := 0; y < SY; y++ {
			for x := 0; x < SX; x++ {
				if toExplore[y][x] && (first || dis[y][x] < minDis) {
					minDis = dis[y][x]
					first = false
				}
			}
		}
		for y := 0; y < SY; y++ {
			for x := 0; x < SX; x++ {
				if dis[y][x] == minDis && toExplore[y][x] {
					nonzero = append(nonzero, Pos{y, x})
				}
			}
		}
		if len(nonzero) == 0 {
			dynamicSearchFallback = true
		}
	}

	if dynamicSearchFallback {
		if s.Limit != nil && *s.Limit >= 0 {
			return nil
		}

		searchPrio, _ := exploreToSearch(a, 0)
		ninf := float32(math.Inf(-1))
		if s.Limit != nil {
			// search_prio[search_prio < limit] = -inf; search_prio -= dis * isfinite(search_prio) * 100
			below := searchPrio.Less(float64(*s.Limit))
			searchPrio.SetWhere(&below, ninf)
			penalty := dis.MulScalar(100)
			infinite := searchPrio.IsFinite().Not()
			penalty.SetWhere(&infinite, 0)
			searchPrio.SubI32(&penalty)
		} else {
			// search_prio -= dis * 4
			penalty := dis.MulScalar(4)
			searchPrio.SubI32(&penalty)
		}

		toSearch = searchPrio.IsFinite()
		toExplore = toVisit.Or(toSearch).And(reachable)
		if !toExplore.Any() {
			return nil
		}
		// the tiles with search_prio == search_prio[to_explore].max()
		maxP, _ := searchPrio.MaxWhere(&toExplore)
		best := searchPrio.Eq(maxP).And(toExplore)
		nonzero = best.Nonzero()
	}

	return &exploreGoF{Nonzero: nonzero, ToSearch: toSearch, ToVisit: toVisit, Fast: s.O.FastGoTo}
}

// exploreGoF goes to a random one of Nonzero, then searches there if it is a tile to search.
type exploreGoF struct {
	Nonzero           []Pos
	ToSearch, ToVisit BoolGrid
	Fast              bool
	TY, TX            int
	PC                int
}

func (s *exploreGoF) Resume(c *Ctx) Cmd {
	a := c.W
	switch s.PC {
	case 0:
		// select random closest to_explore tile
		i := a.Rng.RandInt1(len(s.Nonzero))
		s.TY, s.TX = s.Nonzero[i].Y, s.Nonzero[i].X
		s.PC = 1
		return call(skills.GoTo(s.TY, s.TX, skills.GoToOpts{Fast: s.Fast}))
	case 1:
		if s.ToSearch[s.TY][s.TX] && !s.ToVisit[s.TY][s.TX] {
			s.PC = 2
			return call(skills.SearchTimes(5))
		}
	}
	return ret(nil)
}

func exploreToVisit(a *world.Agent, doorOpenCount int) BoolGrid {
	level := a.CurrentLevel()

	// stone = ~level.seen & isin(glyphs, G.STONE); doors = isin(glyphs, G.DOOR_CLOSED) & (door_open_count < limit)
	stone := Isin(a.Glyphs(), G_STONE).AndNot(level.Seen)
	doors := Isin(a.Glyphs(), G_DOOR_CLOSED).And(level.DoorOpenCount.Lt(int32(doorOpenCount)))
	if !stone.Any() && !doors.Any() {
		return stone
	}

	// OR of translated masks == "some 8-neighbour is stone or some 4-neighbour is a door".
	return NeighborAny8(&stone).Or(NeighborAny4(&doors))
}

// exploreToSearch is to_search_func(prio_limit, return_prio): returns (prio, prio >= prio_limit).
func exploreToSearch(a *world.Agent, prioLimit int) (F32Grid, BoolGrid) {
	level := a.CurrentLevel()
	dis := a.Bfs()

	var buf [SY * SX]int32
	counts := level.SearchCount.AppendPositive(buf[:0]) // search_count[search_count > 0]
	searchDiff := 0.0
	var maxCount int32
	if len(counts) > 0 {
		maxCount = counts[0]
		for _, c := range counts {
			if c > maxCount {
				maxCount = c
			}
		}
		searchDiff = float64(maxCount) - world.NpQuantileLinear(counts, 0.3)
		a.StatsLogger.LogMaxValue("search_diff", searchDiff)
	}

	if searchDiff > 400 && a.Blstats.Hitpoints == a.Blstats.MaxHitpoints &&
		level.SearchCount[a.Blstats.Y][a.Blstats.X] == maxCount {
		a.StatsLogger.LogEvent("allow_walk_traps")
		a.AllowWalkingThroughTrapsTurn = float64(a.LastTurn)
		if searchDiff > 500 {
			a.StatsLogger.LogEvent("allow_attack_all")
			a.AllowAttackAllTurn = float64(a.LastTurn)
		}
	}

	m := level.SearchMasks()
	ninf := float32(math.Inf(-1))

	// prio = -1; prio -= search_count ** 2 * 2
	prio := FullF32(-1)
	sq := level.SearchCount.Mul(&level.SearchCount)
	sq = sq.MulScalar(2)
	prio.SubI32(&sq)
	// prio += (is_on_door & (stones > 3)) * 250
	door := m.Stones.Gt(3)
	door = door.And(m.IsOnDoor)
	prio.AddWhere(&door, 250)
	// prio += (sum of the walkable 4-neighbours <= 1) * 250
	deadEnd := m.WalkableAround.Le(1)
	prio.AddWhere(&deadEnd, 250)
	// prio[(stones == 0) & (walls == 0)] = -inf
	noStones, noWalls := m.Stones.Eq(0), m.Walls.Eq(0)
	lone := noStones.And(noWalls)
	prio.SetWhere(&lone, ninf)
	// prio[~level.walkable | (dis == -1)] = -inf
	off := level.Walkable.Not().Or(dis.Eq(-1))
	prio.SetWhere(&off, ninf)
	return prio, prio.GreaterEqual(float64(prioLimit))
}
