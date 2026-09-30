package tactics

import (
	"strings"
	"testing"

	. "aago/grid"
	"aago/internal/botsim"
	. "aago/internal/pyrt"
	. "aago/nle"
	"aago/sense"
	"aago/skills"
	"aago/world"
)

// dipPrompts answers #dip like the game: which item, then (on a fountain) whether into the fountain.
func dipPrompts(g *botsim.Game, act Action) bool {
	switch {
	case act == A_Command_DIP:
		g.Message = "What do you want to dip? [a or ?*]"
	case act == Action('a') && strings.HasPrefix(g.Prompt, "What do you want to dip?"):
		g.Message = "What do you want to dip the long sword into? [a or ?*]"
		if g.T[g.Y][g.X] == botsim.Fountain {
			g.Message = "Dip the long sword into the fountain? [yn] (n)"
		}
	case act == Action('y') && strings.HasPrefix(g.Prompt, "Dip the long sword"):
		g.Message = "As you dip the sword into the fountain, a murky hand from the depths reaches up to bless " +
			"the sword.  As the hand retreats, the fountain disappears!"
		g.T[g.Y][g.X] = botsim.Floor
	default:
		return false
	}
	return true
}

// dip_for_excalibur: only a lawful character of level 5+ with a long sword (not Excalibur yet) and a reachable
// fountain goes there and dips the sword. A wizard is never lawful, so no game of the corpus reaches it.
func TestDipForExcalibur(t *testing.T) {
	g := botsim.Parse(2, 10,
		"--------",
		"|@....{|",
		"--------")
	a := g.NewAgent(world.Level_DUNGEONS_OF_DOOM, 3)
	check := func(want bool, why string) Frame {
		t.Helper()
		ok, body := DipForExcalibur{}.Check(a)
		if ok != want {
			t.Fatalf("%s: check %v", why, ok)
		}
		return body
	}
	a.Character.Alignment = world.Character_NEUTRAL
	a.Blstats.ExperienceLevel = 5
	check(false, "neutral")
	a.Character.Alignment = world.Character_LAWFUL
	check(false, "no long sword")
	sword := botsim.GiveItem(a, 'a', "a long sword", botsim.ObjectGlyph("long sword"))
	a.Blstats.ExperienceLevel = 4
	check(false, "level 4")
	a.Blstats.ExperienceLevel = 5
	body := check(true, "ready")

	g.Respond = dipPrompts
	if _, exc := g.Run(a, body, 50); exc != nil {
		t.Fatalf("%v; %s", exc, g.Report(a))
	}
	if g.Y != 3 || g.X != 16 {
		t.Fatalf("dipped at (%d, %d)", g.Y, g.X)
	}
	want := []Action{A_Command_DIP, Action('a'), Action('y')}
	if k := g.Keys[len(g.Keys)-3:]; k[0] != want[0] || k[1] != want[1] || k[2] != want[2] {
		t.Fatalf("keys %v", g.Keys)
	}
	g.Play(world.DirActions["w"]) // the level memory sees the tile once the hero is off it
	g.Observe(a)
	check(false, "the fountain is gone")

	// Excalibur (a named long sword with a damage bonus): nothing to do
	g.T[3][16] = botsim.Fountain
	g.Observe(a)
	check(true, "a fountain again")
	sword.DmgBonus = new(float64)
	check(false, "already Excalibur")
	sword.DmgBonus = nil

	// the dip itself where there is no fountain: the game asks what to dip into; AgentPanic
	g.T[3][16] = botsim.Floor
	g.Observe(a)
	_, exc := g.Run(a, sense.Atom(&dipF{Item: sword}), 10)
	if p, ok := exc.(*AgentPanic); !ok || p.Msg != "no fountain here" {
		t.Fatalf("got %v", exc)
	}
}

// check_altar learns the alignment of the altar the hero stands on with ':'.
func TestCheckAltar(t *testing.T) {
	g := botsim.Parse(2, 10,
		"-----",
		"|@A.|",
		"-----")
	a := g.NewAgent(world.Level_DUNGEONS_OF_DOOM, 3)
	altar := Pos{3, 12}
	if got := a.CurrentLevel().Altars.GetOr(altar, -9); got != world.Character_UNKNOWN {
		t.Fatalf("altar seen as %d", got)
	}
	if ok, _ := (checkAltarN{}).Check(a); ok {
		t.Fatal("check_altar away from the altar")
	}
	if _, exc := g.Run(a, skills.GoTo(altar.Y, altar.X, skills.GoToOpts{}), 10); exc != nil {
		t.Fatal(exc)
	}
	ok, body := checkAltarN{}.Check(a)
	if !ok {
		t.Fatal("no check_altar on the altar")
	}
	g.Respond = func(g *botsim.Game, act Action) bool {
		if act != A_Command_LOOK {
			return false
		}
		g.Message = "There is an altar to Thoth (neutral) here."
		return true
	}
	if _, exc := g.Run(a, body, 10); exc != nil {
		t.Fatal(exc)
	}
	if got := a.CurrentLevel().Altars.MustGet(altar); got != world.Character_NEUTRAL {
		t.Fatalf("altar alignment %d", got)
	}
	if ok, _ := (checkAltarN{}).Check(a); ok {
		t.Fatal("check_altar again on a known altar")
	}
	// identify_items_on_altar: nothing unknown to drop
	if ok, _ := (IdentifyOnAltar{}).Check(a); ok {
		t.Fatal("identify_items_on_altar with an empty inventory")
	}
}
