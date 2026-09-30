package sense

import (
	"strings"

	. "aago/grid"
	. "aago/internal/pyrt"
	. "aago/nle"
	"aago/world"
)

// ---- monster tracker: ';' list of monsters on the map

// takeAllMonstersF returns []seenMonster (nil when swallowed or when there are no monsters).
type takeAllMonstersF struct{ PC int }

func (s *takeAllMonstersF) Resume(c *Ctx) Cmd {
	a := c.W
	if s.PC == 0 {
		if AnyIn(a.Glyphs(), G_SWALLOW) {
			return ret([]world.SeenMonster(nil))
		}
		s.PC = 1
		return call(Atom(&whatisF{}))
	}
	r, _ := c.Ret.(*OMap[Pos, string])
	if r == nil {
		return ret([]world.SeenMonster(nil))
	}
	var out []world.SeenMonster
	for _, p := range r.Keys() {
		out = append(out, world.SeenMonster{p, r.MustGet(p)})
	}
	return ret(out)
}

type whatisF struct{ PC int }

func (s *whatisF) Resume(c *Ctx) Cmd {
	a := c.W
	if s.PC == 0 {
		s.PC = 1
		return call(a.Term.Keys(A_Command_WHATIS, world.IterActs(world.Ch('M'))))
	}
	if strings.Contains(a.Message, "No monsters are currently shown on the map.") {
		return ret((*OMap[Pos, string])(nil))
	}
	index := Index(a.Popup, "All monsters currently shown on the map:")
	r := NewOMap[Pos, string]()
	for _, line := range a.Popup[index+1:] {
		m := Search(world.WhatisRe, line)
		Assert(m != nil, line)
		x, y := PyInt(m[1]), PyInt(m[2])
		x--
		r.Set(Pos{y, x}, m[4])
	}
	return ret(r)
}

// monsterUpdateF is MonsterTracker.update: which monsters are peaceful, from their movement or from ';'.
type monsterUpdateF struct{ PC int }

func (s *monsterUpdateF) Resume(c *Ctx) Cmd {
	a := c.W
	m := a.MonsterTracker
	if s.PC == 0 {
		newMonsterMask := m.CurrentMonsterMask()
		var newPeaceful BoolGrid
		known := false // Python: new_peaceful is not None
		if m.LastGlyphs != nil {
			// peaceful = last_glyphs where peaceful, aggressive = where a monster and not peaceful, new = the
			// monsters now; -1 elsewhere
			pea, agr, nmon := *m.LastGlyphs, *m.LastGlyphs, *a.Glyphs()
			notPea := m.PeacefulMonsterMask.Not()
			pea.SetWhere(&notPea, -1)
			notAgr := m.MonsterMask.AndNot(m.PeacefulMonsterMask).Not()
			agr.SetWhere(&notAgr, -1)
			notNew := newMonsterMask.Not()
			nmon.SetWhere(&notNew, -1)
			newPeaceful, known = world.FigureOutMonsterMovement(&pea, &agr, &nmon, 2)
		}
		m.MonsterMask = newMonsterMask
		m.PeacefulMonsterMask = BoolGrid{}
		if !a.Character.Prop.Hallu() {
			if !known {
				s.PC = 1
				return call(&takeAllMonstersF{})
			}
			m.PeacefulMonsterMask = newPeaceful
		}
	} else {
		all := c.Ret.([]world.SeenMonster)
		var petMask BoolGrid
		m.MonsterMask, petMask = m.CurrentMasks()
		for _, sm := range all {
			y, x := PyIdx(sm.Pos.Y, SY), PyIdx(sm.Pos.X, SX)
			if !(m.MonsterMask[y][x] || petMask[y][x] || (sm.Pos.Y == a.Blstats.Y && sm.Pos.X == a.Blstats.X)) {
				Panic("monsters differs between list and glyphs")
			}
			if strings.Contains(sm.Name, "peaceful") && !petMask[y][x] {
				m.PeacefulMonsterMask[y][x] = true
			}
		}
	}
	onlyPeaceful := m.PeacefulMonsterMask.AndNot(m.MonsterMask)
	Assert(!onlyPeaceful.Any()) // no peaceful cell that is not a monster
	if m.LastGlyphs == nil {
		m.LastGlyphs = new(I16Grid)
	}
	*m.LastGlyphs = *a.Glyphs()
	return ret(nil)
}
