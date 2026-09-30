package botsim

import (
	"testing"

	. "aago/nle"
	"aago/world"
)

// The glyphs the game shows are what the bot's glyph sets expect.
func TestGlyphs(t *testing.T) {
	for _, c := range []struct {
		name  string
		glyph int
		set   *GSet
	}{
		{"stone", GlyphStone, G_STONE}, {"wall", GlyphWall, G_WALL}, {"doorway", GlyphDoorway, G_FLOOR},
		{"closed door", GlyphClosedDoor, G_DOOR_CLOSED}, {"floor", GlyphFloor, G_VISIBLE_FLOOR},
		{"up stairs", GlyphStairsUp, G_STAIR_UP}, {"down stairs", GlyphStairsDown, G_STAIR_DOWN},
		{"pit", GlyphPit, G_TRAPS}, {"hole", GlyphHole, G_TRAPS}, {"boulder", GlyphBoulder, G_BOULDER},
		{"scroll", GlyphScroll, G_NORMAL_OBJECTS}, {"hero", GlyphHero, G_MONS},
	} {
		if !c.set.Has(c.glyph) {
			t.Errorf("%s glyph %d is not in its set", c.name, c.glyph)
		}
	}
	if G_DOORS.Has(GlyphDoorway) || G_PETS.Has(GlyphHero) {
		t.Error("a doorway is not a door, the hero is not a pet")
	}
}

// Moves and pushes follow the Sokoban rules.
func TestMovesAndPushes(t *testing.T) {
	g := Parse(5, 10,
		"-------",
		"|@0.^|",
		"|.0..|",
		"|0...|",
		"-------")
	g.Sokoban = true
	g.Play(world.DirActions["e"]) // push the boulder east
	if g.X != 12 || !g.Boulder[6][13] || g.Pushes != 1 {
		t.Fatalf("push east: hero (%d, %d), %s", g.Y, g.X, g)
	}
	g.Play(world.DirActions["e"]) // into the hole: plugged
	if g.T[6][14] != Floor || g.Filled != 1 || g.Boulder[6][14] {
		t.Fatalf("plug: %s", g)
	}
	g.Play(world.DirActions["sw"]) // (6,13) -> (7,12) has a boulder: no diagonal pushes in Sokoban
	if g.X != 13 || g.Y != 6 {
		t.Fatalf("diagonal push moved the hero: %s", g)
	}
	g = Parse(5, 10,
		"-----",
		"|@0.|",
		"|0..|",
		"-----")
	g.Sokoban = true
	g.Play(world.DirActions["se"]) // squeezing between two boulders
	if g.Y != 6 || g.X != 11 || g.Message != "You cannot pass that way." {
		t.Fatalf("squeezed: (%d, %d) %q", g.Y, g.X, g.Message)
	}
	g.Sokoban = false
	g.Play(world.DirActions["se"])
	if g.Y != 7 || g.X != 12 {
		t.Fatalf("no squeeze outside Sokoban: (%d, %d)", g.Y, g.X)
	}
}

// The travel command: the cursor starts on the hero (then on the last destination), '.' travels.
func TestTravel(t *testing.T) {
	g := Parse(2, 2,
		"------",
		"|@...|",
		"|.0^.|",
		"|....|",
		"------")
	a := g.NewAgent(0, 1)
	for _, k := range []Action{A_Command_TRAVEL, world.DirActions["se"], world.DirActions["s"],
		world.DirActions["e"], world.DirActions["e"]} {
		g.Play(k)
		g.Observe(a)
	}
	if a.CursorPos.Y != 5 || a.CursorPos.X != 6 || g.Y != 3 {
		t.Fatalf("cursor %v, hero (%d, %d)", a.CursorPos, g.Y, g.X)
	}
	g.Play(A_MiscDirection_WAIT)
	g.Observe(a)
	if g.Y != 5 || g.X != 6 || a.Blstats.Y != 5 || a.Blstats.X != 6 || g.Travelled != 1 || len(g.Fell) != 0 {
		t.Fatalf("hero (%d, %d), agent (%d, %d)", g.Y, g.X, a.Blstats.Y, a.Blstats.X)
	}
	g.Play(A_Command_TRAVEL)
	if *g.cursor != (struct{ Y, X int }{5, 6}) {
		t.Fatalf("the second travel starts at %v", *g.cursor)
	}
}
