package perception

import (
	"testing"

	. "aago/nle"
)

// Artifacts are parsed as their base object with the damage and to-hit bonus AutoAscend gives them (the wizard
// is gifted Magicbane on an altar; the others come from bones, sacrifices or wishes).
func TestParseTextArtifacts(t *testing.T) {
	for _, c := range []struct {
		text, obj   string
		dmg, toHit  float64
		named, weap bool
	}{
		{"a +0 long sword named Excalibur", "long sword", 5.5, 3, true, true},
		{"a +2 war hammer named Mjollnir", "war hammer", 12.5, 3, true, true},
		{"a +0 battle-axe named Cleaver", "battle-axe", 3.5, 2, true, true},
		{"a +0 elven dagger named Sting", "elven dagger", 2.5, 3, true, true},
		{"a +0 orcish dagger named Grimtooth", "orcish dagger", 3.5, 1.5, true, true},
		{"a +0 long sword named Sunsword", "long sword", 5.5, 3, true, true},
		{"a +0 long sword named Frost Brand", "long sword", 5.5, 3, true, true},
		{"a +0 long sword named Fire Brand", "long sword", 5.5, 3, true, true},
		{"a +0 long sword named Demonbane", "long sword", 5.5, 3, true, true},
		{"a +0 long sword named Giantslayer", "long sword", 5.5, 3, true, true},
		{"a +0 long sword named Vorpal Blade", "long sword", 1, 3, true, true},
		{"a +0 elven broadsword named Orcrist", "elven broadsword", 6, 3, true, true},
		{"a +1 athame named Magicbane (weapon in hand)", "athame", 7.25, 2, true, true},
		{"a +0 silver saber named Grayswandir", "silver saber", 15, 3, true, true},
		{"a +0 silver saber named Werebane", "silver saber", 15, 3, true, true},
		{"a +0 runesword named Stormbringer", "runesword", 6, 3, true, true},
		{"a +0 katana named Snickersnee", "katana", 4.5, 1, true, true},
		{"a +0 morning star named Trollsbane", "morning star", 5.25, 3, true, true},
		{"a +0 war hammer named Ogresmasher", "war hammer", 3, 3, true, true},
		{"a +0 broadsword named Dragonbane", "broadsword", 4.75, 3, true, true},
	} {
		p := ParseText(c.text, NH_WEAPON_CLASS, -1)
		if len(p.Objs) != 1 || p.Objs[0] != O_FromName1(c.obj) {
			t.Errorf("%q: objects %v, want %s", c.text, p.Objs, c.obj)
			continue
		}
		if p.DmgBonus == nil || *p.DmgBonus != c.dmg || p.ToHitBonus == nil || *p.ToHitBonus != c.toHit {
			t.Errorf("%q: bonus %v/%v, want %v/%v", c.text, p.DmgBonus, p.ToHitBonus, c.dmg, c.toHit)
		}
	}
	if p := ParseText("a +1 athame named Magicbane (weapon in hand)", NH_WEAPON_CLASS, -1); !p.Equipped {
		t.Error("Magicbane in hand is not equipped")
	}
}

// Names AutoAscend rewrites before looking the object up: statues and figurines of a monster, eggs, novels,
// worm teeth.
func TestParseTextSpecialNames(t *testing.T) {
	for _, c := range []struct {
		text     string
		category int
		obj      string
		monster  string // "" when not a monster's item
	}{
		{"a statue of a newt", NH_ROCK_CLASS, "statue", "newt"},
		{"a figurine of a gnome lord", NH_TOOL_CLASS, "figurine", "gnome lord"},
		{"2 figurines of a jackal", NH_TOOL_CLASS, "figurine", "jackal"},
		{"a lizard egg", NH_FOOD_CLASS, "egg", "lizard"},
		{"3 cockatrice eggs", NH_FOOD_CLASS, "egg", "cockatrice"},
		{"a paperback book", NH_SPBOOK_CLASS, "novel", ""},
		{"a novel", NH_SPBOOK_CLASS, "novel", ""},
		{"2 worm teeth", NH_WEAPON_CLASS, "worm tooth", ""},
	} {
		p := ParseText(c.text, c.category, -1)
		if len(p.Objs) != 1 || p.Objs[0].Name != c.obj {
			t.Errorf("%q: objects %v, want %s", c.text, p.Objs, c.obj)
			continue
		}
		want := -1
		if c.monster != "" {
			want = GlyphToMon(MON_FromName(c.monster))
		}
		if p.MonsterID != want {
			t.Errorf("%q: monster %d, want %d (%s)", c.text, p.MonsterID, want, c.monster)
		}
	}
}
