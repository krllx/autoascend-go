package world

import (
	"regexp"

	. "aago/grid"
	. "aago/nle"
)

// Port of autoascend/monster_tracker/*.

type MonsterTracker struct {
	agent               *Agent
	LastGlyphs          *I16Grid
	PeacefulMonsterMask BoolGrid
	MonsterMask         BoolGrid
}

func NewMonsterTracker(a *Agent) *MonsterTracker {
	m := &MonsterTracker{agent: a}
	m.OnPanic()
	return m
}

func (m *MonsterTracker) OnPanic() {
	m.LastGlyphs = nil
	m.PeacefulMonsterMask = BoolGrid{}
	m.MonsterMask = BoolGrid{}
}

// WhatisRe is a line of the ';' list of monsters. Not in the Python bot: its description allowed only letters and
// `-,' `, but a hiding monster names the object ("garter snake, hiding under a scroll labeled NR 9", seed 1069).
var WhatisRe = regexp.MustCompile(`^<(\d+),(\d+)>  ([\x00-\x7F])  (.+)$`)

type SeenMonster struct {
	Pos  Pos
	Name string
}

func (m *MonsterTracker) CurrentMasks() (BoolGrid, BoolGrid) {
	return m.CurrentMonsterMask(), Isin(m.agent.Glyphs(), G_PETS)
}

// CurrentMonsterMask is the first mask of CurrentMasks: monsters on the screen except the agent.
func (m *MonsterTracker) CurrentMonsterMask() BoolGrid {
	a := m.agent
	nm := Isin(a.Glyphs(), G_MONS, G_INVISIBLE_MON)
	nm[a.Blstats.Y][a.Blstats.X] = false
	return nm
}

// DisappearanceMask marks the monsters of oldMons (-1: none) with no monster of the same glyph within maxRadius
// in newMons.
func DisappearanceMask(oldMons, newMons *I16Grid, maxRadius int) (ret BoolGrid) {
	present := oldMons.Eq(-1).Not()
	for _, p := range present.Nonzero() {
		y, x := p.Y, p.X
		g := oldMons[y][x]
		all := true
		for yy := max(0, y-maxRadius); yy < min(y+maxRadius+1, SY) && all; yy++ {
			for xx := max(0, x-maxRadius); xx < min(x+maxRadius+1, SX); xx++ {
				if newMons[yy][xx] == g {
					all = false
					break
				}
			}
		}
		ret[y][x] = all
	}
	return
}

// FigureOutMonsterMovement marks the monsters of newMons (-1: none) that can only have come from a peaceful one
// of the last step (a glyph match within maxRadius); ok is false (Python: None) when some monster fits both or
// neither.
func FigureOutMonsterMovement(peaceful, aggressive, newMons *I16Grid, maxRadius int) (ret BoolGrid, ok bool) {
	present := newMons.Eq(-1).Not()
	for _, p := range present.Nonzero() {
		y, x := p.Y, p.X
		g := newMons[y][x]
		canP, canA := false, false
		for py := max(0, y-maxRadius); py < min(y+maxRadius+1, SY); py++ {
			for px := max(0, x-maxRadius); px < min(x+maxRadius+1, SX); px++ {
				if peaceful[py][px] == g {
					canP = true
				}
				if aggressive[py][px] == g {
					canA = true
				}
			}
		}
		if canP == canA {
			return BoolGrid{}, false
		}
		if canP {
			ret[y][x] = true
		}
	}
	return ret, true
}
