package world

import (
	"math"
	"regexp"

	. "aago/grid"
	. "aago/internal/pyrt"
	. "aago/nle"
)

// Port of exploration_logic.py: explore1, patrol, traps, altars.

var ExplAltarRe = regexp.MustCompile(`There is an altar to [a-zA-Z\- ]+ \(([a-z]+)\) here.`)

// explNameToAlignment mirrors Character.name_to_alignment.
var ExplNameToAlignment = map[string]int{
	"chaotic":   Character_CHAOTIC,
	"neutral":   Character_NEUTRAL,
	"lawful":    Character_LAWFUL,
	"unaligned": Character_UNALIGNED,
}

// npQuantileLinear mirrors np.quantile(int_array, q) with the default 'linear' method (numpy 2.x).
func NpQuantileLinear(vals []int32, q float64) float64 {
	n := len(vals)
	var buf [SY * SX]int32 // the usual input (search counts of a level) fits: no heap copy
	s := append(buf[:0], vals...)
	vi := float64(float64(n-1) * q) // explicit conversions everywhere: Go may fuse a*b+c (FMA) across statements
	prev := math.Floor(vi)
	var pi, ni int
	if vi >= float64(n-1) {
		pi, ni = n-1, n-1
		prev = -1
	} else {
		pi = int(prev)
		ni = pi + 1
	}
	gamma := vi - prev
	// only the pi-th and ni-th smallest values matter: select them instead of sorting everything
	selectKth(s, pi)
	av, bv := s[pi], s[pi]
	if ni != pi {
		bv = s[ni]
		for _, v := range s[ni+1:] {
			bv = min(bv, v)
		}
	}
	diff := float64(bv - av)
	res := float64(av) + float64(diff*gamma) // explicit conversion: no FMA
	if gamma >= 0.5 {
		res = float64(bv) - float64(diff*(1-gamma))
	}
	return res
}

// selectKth reorders s so that s[k] is the k-th smallest value, everything before it is <= s[k] and everything
// after it is >= s[k] (quickselect, median-of-three pivot; the same values as sorting would put there).
func selectKth(s []int32, k int) {
	lo, hi := 0, len(s)-1
	for lo < hi {
		mid := lo + (hi-lo)/2
		if s[mid] < s[lo] {
			s[mid], s[lo] = s[lo], s[mid]
		}
		if s[hi] < s[lo] {
			s[hi], s[lo] = s[lo], s[hi]
		}
		if s[hi] < s[mid] {
			s[hi], s[mid] = s[mid], s[hi]
		}
		pivot := s[mid]
		i, j := lo, hi
		for i <= j {
			for s[i] < pivot {
				i++
			}
			for s[j] > pivot {
				j--
			}
			if i <= j {
				s[i], s[j] = s[j], s[i]
				i++
				j--
			}
		}
		switch {
		case k <= j:
			hi = j
		case k >= i:
			lo = i
		default:
			return
		}
	}
}

// WorthUntrapping mirrors worth_untrapping.
func (e *ExplorationLogic) WorthUntrapping(trapY, trapX int) bool {
	walkable := &e.agent.CurrentLevel().Walkable
	lastWalkableSet := false
	lastWalkable := false
	walkableChanges := 0
	for _, d := range [][2]int{{-1, -1}, {-1, 0}, {-1, 1}, {0, 1}, {1, 1}, {1, 0}, {1, -1}, {0, -1}, {-1, -1}} {
		y, x := trapY-d[0], trapX-d[1]
		var w bool
		if !(0 <= y && y < SY) || !(0 <= x && x < SX) {
			w = false
		} else {
			w = walkable[y][x]
		}
		if lastWalkableSet && lastWalkable != w {
			walkableChanges++
		}
		lastWalkable = w
		lastWalkableSet = true
	}
	Assert(walkableChanges%2 == 0)
	return walkableChanges >= 4
}

// Port of exploration_logic.py: level graph search and level-to-level strategies.

// LevelPathStep is one (k, t, dir) element of the _level_dfs path.
type LevelPathStep struct {
	Pos  Pos
	Dest StairDest
	Dir  string
}

func LevelKeyOf(k SetKey) LevelKey { return LevelKey{k.A, k.B} }

func LevelSetKey(l LevelKey) SetKey { return PairKey(l.Dungeon, l.Level) }

// levelDfs mirrors _level_dfs; ok == false means None.
func (e *ExplorationLogic) levelDfs(start, end LevelKey, path *[]LevelPathStep, vis *PySet) ([]LevelPathStep, bool) {
	a := e.agent
	if vis.Contains(LevelSetKey(start)) {
		return nil, false
	}

	if start == end {
		return *path, true
	}

	vis.Add(LevelSetKey(start))
	var stairs []StairEntry
	if a.Levels.Has(start) {
		stairs = a.Levels.MustGet(start).GetStairs(false, false, false, true)
	}
	for _, st := range stairs {
		if st.Dest == nil {
			continue
		}
		k, t := st.Pos, *st.Dest
		glyph := int(a.Levels.MustGet(start).Objects[k.Y][k.X])
		dir := ""
		if G_STAIR_DOWN.Has(glyph) {
			dir = ">"
		} else if G_STAIR_UP.Has(glyph) {
			dir = "<"
		}
		Assert(dir != "", glyph) // TODO: portals

		*path = append(*path, LevelPathStep{Pos: k, Dest: t, Dir: dir})
		r, ok := e.levelDfs(t.Level, end, path, vis)
		if ok {
			return r, true
		}
		*path = (*path)[:len(*path)-1]
	}
	return nil, false
}

// GetPathToLevel mirrors get_path_to_level (ok == false means None).
func (e *ExplorationLogic) GetPathToLevel(dungeonNumber, levelNumber int) ([]LevelPathStep, bool) {
	path := []LevelPathStep{}
	return e.levelDfs(e.agent.CurrentLevel().Key(), LevelKey{dungeonNumber, levelNumber}, &path, NewPySet())
}

// GetAchievableLevels mirrors get_achievable_levels() (no arguments).
func (e *ExplorationLogic) GetAchievableLevels() *PySet {
	k := e.agent.CurrentLevel().Key()
	vis := NewPySet()
	path := []LevelPathStep{}
	e.levelDfs(k, LevelKey{-1, -1}, &path, vis)
	return vis
}

// LevelsToExploreToGetTo mirrors levels_to_explore_to_get_to (achievableLevels nil means None).
func (e *ExplorationLogic) LevelsToExploreToGetTo(dungeonNumber, levelNumber int, achievableLevels *PySet) *PySet {
	if achievableLevels == nil {
		achievableLevels = e.GetAchievableLevels()
	}

	if achievableLevels.Len() == 1 {
		return achievableLevels
	}

	if achievableLevels.Contains(PairKey(dungeonNumber, levelNumber)) {
		return NewPySet()
	}

	items := achievableLevels.Items()
	anyDun := false
	for _, k := range items {
		if k.A == dungeonNumber {
			anyDun = true
			break
		}
	}
	if anyDun {
		first := true
		closest, closestKey := 0, 0
		for _, k := range items {
			if k.A == dungeonNumber {
				kv := Abs(levelNumber - k.B)
				if first || kv < closestKey {
					closest, closestKey = k.B, kv
					first = false
				}
			}
		}
		return NewPySet(PairKey(dungeonNumber, closest))
	}

	unionRange := func(lo, hi int) *PySet {
		parts := []*PySet{}
		for i := lo; i < hi; i++ {
			parts = append(parts, e.LevelsToExploreToGetTo(Level_DUNGEONS_OF_DOOM, i, achievableLevels))
		}
		lit := NewPySet()
		for i := lo; i < hi; i++ {
			if achievableLevels.Contains(PairKey(Level_DUNGEONS_OF_DOOM, i)) {
				lit.Add(PairKey(Level_DUNGEONS_OF_DOOM, i))
			}
		}
		return parts[0].Union(append(parts[1:], lit)...)
	}

	if dungeonNumber == Level_GNOMISH_MINES {
		return unionRange(2, 5)
	}

	if dungeonNumber == Level_SOKOBAN {
		// TODO: one level below oracle
		return unionRange(6, 11)
	}

	allMines := true
	for _, k := range items {
		if k.A != Level_GNOMISH_MINES {
			allMines = false
			break
		}
	}
	if allMines {
		r := e.LevelsToExploreToGetTo(Level_GNOMISH_MINES, 1, nil)
		var other *PySet
		if achievableLevels.Contains(PairKey(Level_GNOMISH_MINES, 1)) {
			other = NewPySet(PairKey(Level_GNOMISH_MINES, 1))
		} else {
			other = NewPySet()
		}
		return r.Union(other)
	}

	// TODO: more dungeons

	Assert(false, dungeonNumber, levelNumber)
	return nil
}

// GetUnexploredStairs mirrors get_unexplored_stairs(dungeon_number, level_number, **kwargs);
// key nil means the current level.
func (e *ExplorationLogic) GetUnexploredStairs(key *LevelKey, down, up, portal, all bool) []Pos {
	var k LevelKey
	if key == nil {
		k = e.agent.CurrentLevel().Key()
	} else {
		k = *key
	}
	stairs := e.agent.Levels.MustGet(k).GetStairs(down, up, portal, all)
	var r []Pos
	for _, s := range stairs {
		if s.Dest == nil {
			r = append(r, s.Pos)
		}
	}
	return r
}

type ExplorationLogic struct{ agent *Agent }

// ExploreOpts carries explore1's keyword arguments; use DefaultExploreOpts() for Python defaults.
type ExploreOpts struct {
	SearchPrioLimit     *int // None -> nil
	DoorOpenCount       int
	KickDoors           bool
	TrapSearchOffset    int
	CheckAltarAlignment bool
	FastGoTo            bool
}

func DefaultExploreOpts() ExploreOpts {
	zero := 0
	return ExploreOpts{SearchPrioLimit: &zero, DoorOpenCount: 4, KickDoors: true, TrapSearchOffset: 0,
		CheckAltarAlignment: true, FastGoTo: false}
}
