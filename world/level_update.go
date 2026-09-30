package world

import (
	"regexp"
	"strings"

	. "aago/grid"
	. "aago/internal/pyrt"
	. "aago/nle"
	"aago/perception"
)

func (a *Agent) updateLevelItems() {
	level := a.CurrentLevel()
	by, bx := a.Blstats.Y, a.Blstats.X
	if a.Inventory.ItemsBelowMeNone {
		Raise("TypeError", "object of type 'NoneType' has no len()")
	}
	level.Items[by][bx] = a.Inventory.ItemsBelowMe
	level.ItemCount[by][bx] = int32(len(a.Inventory.ItemsBelowMe))

	ignore := Isin(a.Glyphs(), G_MONS, G_PETS) // TODO: effects, etc
	mask := level.ItemCount.Ne(0).AndNot(ignore)
	notMask := mask.Not()
	level.ItemDisagreementCounter.SetWhere(&notMask, 0)
	for _, p := range mask.Nonzero() {
		y, x := p.Y, p.X
		if (level.ItemCount[y][x] >= 2) == ((int(a.LastObservation.Specials[y][x]) & NH_MG_OBJPILE) > 0) {
			found := false
			g := int(a.Glyphs()[y][x])
		outer:
			for _, item := range level.Items[y][x] {
				for _, dg := range item.DisplayGlyphs() {
					if dg == g {
						found = true
						break outer
					}
				}
			}
			if found {
				level.ItemDisagreementCounter[y][x] = 0
				continue
			}
		}
		level.ItemDisagreementCounter[y][x]++
		if level.ItemDisagreementCounter[y][x] > 3 {
			level.ItemDisagreementCounter[y][x] = 0
			level.Items[y][x] = nil
			level.ItemCount[y][x] = 0
		}
	}
}

var shopWelcomeRe = regexp.MustCompile("Welcome( again)? to [a-zA-Z' ]*(" + strings.Join(SHOP_Names, "|") + ")!")

func (a *Agent) updateLevelShops() {
	level := a.CurrentLevel()
	shopType := -1
	if m := Search(shopWelcomeRe, a.Message); m != nil {
		id, ok := SHOP_Name2ID(m[2])
		Assert(ok, m[2])
		shopType = id
	}
	sk := Isin(a.Glyphs(), G_SHOPKEEPER).And(a.MonsterTracker.PeacefulMonsterMask)
	for _, p := range sk.Nonzero() {
		wall := Isin(&level.Objects, G_WALL)
		t1, t2, t3, t4 := TranslateB(&wall, 1, 0), TranslateB(&wall, -1, 0), TranslateB(&wall, 0, 1), TranslateB(&wall, 0, -1)
		a12 := t1.And(t2)
		a34 := t3.And(t4)
		entry := a12.Or(a34)
		entry = entry.And(level.Walkable)
		walkable := level.Walkable.AndNot(entry)
		dis := BFS(p.Y, p.X, &walkable, &walkable, false)
		var mask BoolGrid
		for y := 0; y < SY; y++ {
			for x := 0; x < SX; x++ {
				mask[y][x] = dis[y][x] != -1
			}
		}
		mask = Dilate(&mask, 1, true)
		de := Dilate(&entry, 1, false)
		for y := 0; y < SY; y++ {
			for x := 0; x < SX; x++ {
				if mask[y][x] {
					level.Shop[y][x] = true
				}
			}
		}
		if mask[a.Blstats.Y][a.Blstats.X] && shopType >= 0 {
			for y := 0; y < SY; y++ {
				for x := 0; x < SX; x++ {
					if mask[y][x] {
						level.ShopType[y][x] = int32(shopType)
					}
				}
			}
		}
		for y := 0; y < SY; y++ {
			for x := 0; x < SX; x++ {
				if mask[y][x] && !de[y][x] {
					level.ShopInterior[y][x] = true
				}
			}
		}
	}
}

// Kill messages. Not in the Python bot: its article group `(an?)` had no space after it, so "kill a newt!"
// gave the name " newt", and "Your scroll of destroy armor catches fire and burns!" gave "rmor catches fire and
// burns"; the monster lookup of such a name failed (seed 3135). Now the article takes its space, and a name
// that is no monster is ignored (before, both failed the same way).
var (
	corpseRe1 = regexp.MustCompile(`((kills?)|(destroys?)) ((an? )|(the) )?([a-zA-Z ]+)\!`)
	corpseRe2 = regexp.MustCompile(`((An? )|(The )( *))([a-zA-Z ]+) is ((killed)|(destroyed))\!`)
)

func (a *Agent) updateLevelCorpses() {
	var names []string
	if strings.Contains(a.Message, "!") { // both patterns end with '!'
		for _, g := range FindAll(corpseRe1, a.Message) {
			names = append(names, g[len(g)-1])
		}
		for _, g := range FindAll(corpseRe2, a.Message) {
			names = append(names, g[len(g)-4])
		}
	}
	var mnames []string
	for _, n := range names {
		if !strings.Contains(n, "invisible") && n != "it" && !strings.HasPrefix(n, "poor ") {
			if strings.HasPrefix(n, "saddled ") {
				n = n[len("saddled "):]
			}
			mnames = append(mnames, n)
		}
	}
	var mn2 []string
	for _, n := range mnames {
		if n == "" {
			Raise("IndexError", "string index out of range")
		}
		if strings.ToLower(n[:1]) == n[:1] && MON_IsName(n) {
			mn2 = append(mn2, n)
		}
	}
	mnames = mn2
	level := a.CurrentLevel()
	if !a.Character.Prop.Hallu() && len(mnames) > 0 {
		oldMons, newMons := a.PreviousObs.Glyphs, *a.Glyphs()
		notOld := Isin(&oldMons, G_MONS, G_INVISIBLE_MON).Not()
		oldMons.SetWhere(&notOld, -1)
		notNew := Isin(&newMons, G_MONS, G_INVISIBLE_MON).Not()
		newMons.SetWhere(&notNew, -1)
		mask := DisappearanceMask(&oldMons, &newMons, 1)
		mons := oldMons
		anyNZ := false
		for y := 0; y < SY; y++ {
			for x := 0; x < SX; x++ {
				if !mask[y][x] {
					mons[y][x] = -1
				}
				if mons[y][x] != 0 {
					anyNZ = true
				}
			}
		}
		Assert(anyNZ)
		for _, mname := range mnames {
			glyph := MON_FromName(mname)
			monsterID := glyph - NH_GLYPH_MON_OFF
			MON_BodyFromName(mname)
			for y := 0; y < SY; y++ {
				for x := 0; x < SX; x++ {
					if int(mons[y][x]) != glyph {
						continue
					}
					all := true
					for _, item := range level.Items[y][x] {
						if !(item.IsCorpse() && item.MonsterID != monsterID) {
							all = false
							break
						}
					}
					if all {
						level.CorpsesAt(Pos{y, x}).Set(monsterID, a.Blstats.Time)
					}
				}
			}
		}
	}
	bp := Pos{a.Blstats.Y, a.Blstats.X}
	// old = corpses_to_eat[bp].copy(); del corpses_to_eat[bp] (a missing entry would be inserted empty and deleted)
	old := NewOMap[int, int]()
	if level.CorpsesToEat.Has(bp) {
		old = level.CorpsesAt(bp).Copy()
		level.CorpsesToEat.Del(bp)
	}
	if a.Inventory.ItemsBelowMeNone {
		Raise("TypeError", "'NoneType' object is not iterable")
	}
	var seen []*Item
	counts := map[*Item]int{}
	for _, item := range a.Inventory.ItemsBelowMe {
		if item.IsCorpse() {
			if counts[item] == 0 {
				seen = append(seen, item)
			}
			counts[item]++
		}
	}
	for _, item := range seen {
		if counts[item] != 1 {
			continue
		}
		v := CorpseAge(old, item.MonsterID)
		level.CorpsesAt(bp).Set(item.MonsterID, v)
	}
}

func (a *Agent) UpdateLevel() {
	if AnyIn(a.Glyphs(), G_SWALLOW) {
		return
	}
	if AnyIn(a.Glyphs(), G_PETS) {
		a.lastPetSeen = a.Blstats.Time
	}
	level := a.CurrentLevel()
	g := a.Glyphs()

	mask := Isin(g, G_FLOOR, G_STAIR_UP, G_STAIR_DOWN, G_DOOR_OPENED, G_TRAPS, G_ALTAR, G_FOUNTAIN)
	level.Walkable = level.Walkable.Or(mask)
	level.Seen = level.Seen.Or(mask)
	level.Objects.CopyWhere(g, &mask)

	mask = Isin(g, G_MONS, G_PETS, G_BODIES, G_OBJECTS, G_STATUES)
	level.Seen = level.Seen.Or(mask)
	level.Walkable = level.Walkable.Or(mask.And(level.Objects.Eq(-1)))

	mask = Isin(g, G_WALL, G_DOOR_CLOSED, G_BARS)
	level.Seen = level.Seen.Or(mask)
	level.Objects.CopyWhere(g, &mask)
	level.Walkable = level.Walkable.AndNot(mask)

	// The agent stands on its cell, so the cell is walkable. Not in the Python bot: a dark cell next to the agent
	// shows as stone and gets marked so (see the neighbours below); once the agent steps on it, the cell stays
	// unwalkable, and BFS takes no diagonal step from an unwalkable cell. With the one orthogonal exit taken by a
	// peaceful monster, nothing is reachable and the exploration loop waits forever without a key (seed 1061).
	level.Walkable[a.Blstats.Y][a.Blstats.X] = true

	a.updateLevelItems()
	a.updateLevelShops()
	a.updateLevelCorpses()

	alt := Isin(&level.Objects, G_ALTAR)
	for _, p := range alt.Nonzero() {
		if !level.Altars.Has(p) {
			level.Altars.Set(p, Character_UNKNOWN)
		}
	}
	level.WasOn[a.Blstats.Y][a.Blstats.X] = true
	for _, p := range a.Neighbors(a.Blstats.Y, a.Blstats.X, false, true) {
		if G_STONE.Has(int(a.Glyphs()[p.Y][p.X])) {
			level.Seen[p.Y][p.X] = true
			level.Objects[p.Y][p.X] = a.Glyphs()[p.Y][p.X]
			level.Walkable[p.Y][p.X] = false
		}
	}
	if !a.Inventory.EngravingBelowMeNone && a.Inventory.EngravingBelowMe != "" &&
		perception.EditDistance(a.Inventory.EngravingBelowMe, "ad aerarium") <= 6 {
		for _, d := range []Pos{{0, 1}, {0, -1}, {1, 0}, {-1, 0}} {
			y, x := a.Blstats.Y+d.Y, a.Blstats.X+d.X
			if InBounds(y, x) && !level.Walkable[y][x] {
				level.Forbidden[y][x] = true
			}
		}
	}
}
