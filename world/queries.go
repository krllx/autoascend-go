package world

import (
	"sort"
	"strings"

	. "aago/grid"
	. "aago/internal/pyrt"
	. "aago/nle"
)

func CalcDirection(fromY, fromX, toY, toX int, allowNonunitDistance bool) string {
	if allowNonunitDistance {
		Assert(fromY == toY || fromX == toX || Abs(fromY-toY) == Abs(fromX-toX))
		toY = fromY + Sign(toY-fromY)
		toX = fromX + Sign(toX-fromX)
	}
	Assert(Abs(fromY-toY) <= 1 && Abs(fromX-toX) <= 1)
	ret := ""
	if toY == fromY+1 {
		ret += "s"
	}
	if toY == fromY-1 {
		ret += "n"
	}
	if toX == fromX+1 {
		ret += "e"
	}
	if toX == fromX-1 {
		ret += "w"
	}
	if ret == "" {
		ret = "."
	}
	return ret
}

func (a *Agent) IsSafeToPray(limit int) bool {
	return (a.LastPrayerTurn < 0 && a.Blstats.Time > 300) ||
		(a.LastPrayerTurn >= 0 && a.Blstats.Time-a.LastPrayerTurn > limit)
}

func (a *Agent) Cast(spell string, dy, dx int) {
	// character.known_spells is only set by parse_spellcast_view, which AutoAscend never calls.
	Raise("AttributeError", "'Character' object has no attribute 'known_spells'")
}

var DirActions = map[string]Action{
	"n": A_CompassDirection_N, "s": A_CompassDirection_S, "e": A_CompassDirection_E, "w": A_CompassDirection_W,
	"ne": A_CompassDirection_NE, "se": A_CompassDirection_SE, "nw": A_CompassDirection_NW, "sw": A_CompassDirection_SW,
	">": A_MiscDirection_DOWN, "<": A_MiscDirection_UP, ".": A_MiscDirection_WAIT,
}

func (a *Agent) CanEngrave() bool {
	if a.Character.Prop.Polymorph() {
		return false
	}
	return Pos{a.Blstats.Y, a.Blstats.X} != a.ForbiddenEngravePosition
}

func (a *Agent) Neighbors(y, x int, shuffle, diagonal bool) []Pos {
	ret := make([]Pos, 0, 8)
	for dy := -1; dy <= 1; dy++ {
		for dx := -1; dx <= 1; dx++ {
			if dy == 0 && dx == 0 {
				continue
			}
			if !diagonal && Abs(dy)+Abs(dx) > 1 {
				continue
			}
			ny, nx := y+dy, x+dx
			if InBounds(ny, nx) {
				ret = append(ret, Pos{ny, nx})
			}
		}
	}
	if shuffle {
		a.Rng.ShuffleLen(len(ret), func(i, j int) { ret[i], ret[j] = ret[j], ret[i] })
	}
	return ret
}

func (a *Agent) Bfs() I32Grid { return a.BfsFrom(a.Blstats.Y, a.Blstats.X) }

// BfsPeek is Bfs() for a check the Python bot does not make: it uses the distances cached for this step but never
// caches its own, which the Python bot would compute later in the step, perhaps after the state changed.
func (a *Agent) BfsPeek() I32Grid {
	if a.lastBfsStep == a.StepCount {
		return a.lastBfsDis
	}
	return a.bfsCompute(a.Blstats.Y, a.Blstats.X)
}

func (a *Agent) BfsFrom(y, x int) I32Grid {
	if a.lastBfsStep == a.StepCount && y == a.Blstats.Y && x == a.Blstats.X {
		return a.lastBfsDis
	}
	dis := a.bfsCompute(y, x)
	if y == a.Blstats.Y && x == a.Blstats.X {
		a.lastBfsDis = dis
		a.lastBfsStep = a.StepCount
	}
	return dis
}

func (a *Agent) bfsCompute(y, x int) I32Grid {
	level := a.CurrentLevel()
	g := a.Glyphs()
	walkable := level.Walkable.AndNot(Isin(g, G_BOULDER)).AndNot(a.MonsterTracker.PeacefulMonsterMask).AndNot(level.Forbidden)
	if float64(a.LastTurn)-a.AllowWalkingThroughTrapsTurn > 50 {
		walkable = walkable.AndNot(Isin(&level.Objects, G_TRAPS))
	}
	walkable = walkable.AndNot(Isin(g, G_ONLY_RANGED_SLOW_MONS))
	diag := walkable.AndNot(Isin(&level.Objects, G_DOORS)).AndNot(level.Objects.Eq(-1))
	return BFS(y, x, &walkable, &diag, a.Inventory.Items.TotalWeight <= 600 && a.CurrentLevel().DungeonNumber != Level_SOKOBAN)
}

func (a *Agent) Path(fromY, fromX, toY, toX int, dis *I32Grid) []Pos {
	if fromY == toY && fromX == toX {
		return []Pos{{toY, toX}}
	}
	if dis == nil {
		d := a.BfsFrom(fromY, fromX)
		dis = &d
	}
	Assert(dis[toY][toX] != -1)
	cy, cx := toY, toX
	rev := []Pos{{cy, cx}}
	for cy != fromY || cx != fromX {
		found := false
		for _, p := range a.Neighbors(cy, cx, true, true) {
			if dis[p.Y][p.X] == dis[cy][cx]-1 && dis[p.Y][p.X] >= 0 {
				rev = append(rev, p)
				cy, cx = p.Y, p.X
				found = true
				break
			}
		}
		Assert(found)
	}
	Assert(dis[cy][cx] == 0 && fromY == cy && fromX == cx)
	path := make([]Pos, len(rev))
	for i := range rev {
		path[i] = rev[len(rev)-1-i]
	}
	return path
}

func (a *Agent) GetVisibleMonsters() []Monster {
	mask := a.MonsterTracker.MonsterMask.AndNot(a.MonsterTracker.PeacefulMonsterMask)
	if !mask.Any() {
		return nil
	}
	dis := a.Bfs()
	var ret []Monster
	for _, p := range mask.Nonzero() {
		y, x := p.Y, p.X
		reach := false
		for yy := max(y-1, 0); yy < min(y+2, SY); yy++ {
			for xx := max(x-1, 0); xx < min(x+2, SX); xx++ {
				if dis[yy][xx] != -1 {
					reach = true
				}
			}
		}
		if !reach {
			continue
		}
		g := int(a.Glyphs()[y][x])
		if g == NH_GLYPH_INVISIBLE || !MON_IsMonster(g) {
			if Adjacent(Pos{a.Blstats.Y, a.Blstats.X}, Pos{y, x}) {
				ret = append(ret, Monster{Dis: int(dis[y][x]), Y: y, X: x, Mon: DummyPermonst, Glyph: g})
			}
		} else {
			ret = append(ret, Monster{Dis: int(dis[y][x]), Y: y, X: x, Mon: MON_Permonst(g), Glyph: g})
		}
	}
	sort.SliceStable(ret, func(i, j int) bool {
		if ret[i].Dis != ret[j].Dis {
			return ret[i].Dis < ret[j].Dis
		}
		if ret[i].Y != ret[j].Y {
			return ret[i].Y < ret[j].Y
		}
		return ret[i].X < ret[j].X
	})
	return ret
}

// engravingLower mirrors inventory.engraving_below_me.lower().
func (a *Agent) EngravingLower() string {
	if a.Inventory.EngravingBelowMeNone {
		Raise("AttributeError", "'NoneType' object has no attribute 'lower'")
	}
	return strings.ToLower(a.Inventory.EngravingBelowMe)
}

var (
	idChameleon, idDoppel, idSandestin = MON_IdFromName("chameleon"), MON_IdFromName("doppelganger"), MON_IdFromName("sandestin")
	idDisenchanter, idAbbot            = MON_IdFromName("disenchanter"), MON_IdFromName("abbot")
	idVioletFungus, idYellowMold       = MON_IdFromName("violet fungus"), MON_IdFromName("yellow mold")
	idBat, idGiantBat                  = MON_IdFromName("bat"), MON_IdFromName("giant bat")
	idLizard, idLichen, idMedusa       = MON_IdFromName("lizard"), MON_IdFromName("lichen"), MON_IdFromName("Medusa")
	idPets                             = []int{MON_IdFromName("dog"), MON_IdFromName("little dog"), MON_IdFromName("large dog"),
		MON_IdFromName("kitten"), MON_IdFromName("housecat"), MON_IdFromName("large cat")}
)

func (a *Agent) IsCorpseEditable(monsterID, ageTurn int) bool {
	p := MON_Permonst(monsterID)
	if a.Character.Race != Character_ORC && p.Mflags1&MON_M1_POIS != 0 {
		return false
	}
	if p.Mflags1&MON_M1_ACID != 0 {
		return false
	}
	if p.Mflags2&MON_M2_WERE != 0 {
		return false
	}
	if monsterID == idChameleon || monsterID == idDoppel || monsterID == idSandestin {
		return false
	}
	if monsterID == idDisenchanter {
		return false
	}
	if monsterID == idAbbot || monsterID == idVioletFungus || monsterID == idYellowMold {
		return false
	}
	if monsterID == idBat || monsterID == idGiantBat {
		return false
	}
	if Contains(idPets, monsterID) {
		return false
	}
	if int(p.Mlet) == MON_S_COCKATRICE || monsterID == idMedusa {
		return false
	}
	if int(p.Mlet) == MON_S_MIMIC {
		return false
	}
	var raceFlag int
	switch a.Character.Race {
	case Character_HUMAN:
		raceFlag = MON_M2_HUMAN
	case Character_DWARF:
		raceFlag = MON_M2_DWARF
	case Character_ELF:
		raceFlag = MON_M2_ELF
	case Character_GNOME:
		raceFlag = MON_M2_GNOME
	case Character_ORC:
		raceFlag = 0
	default:
		Raise("KeyError", "%v", a.Character.Race)
	}
	if a.Character.Role == Character_CAVEMAN {
		raceFlag = 0
	}
	if p.Mflags2&raceFlag != 0 {
		return false
	}
	if a.Blstats.Time-ageTurn >= 50 && monsterID != idLizard && monsterID != idLichen {
		return false
	}
	return true
}

type CorpseTarget struct{ Y, X, MonsterID int }

func (a *Agent) HealingPotions(names ...string) []*Item {
	var r []*Item
	for it := range EachItem(a.Inventory.Items.AllItems) {
		if it.IsUnambiguous() && it.Category == NH_POTION_CLASS && Contains(names, it.Object().Name) {
			r = append(r, it)
		}
	}
	return r
}
