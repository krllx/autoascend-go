package perception

import (
	"strings"
	"sync"

	. "aago/internal/pyrt"
	. "aago/nle"
)

type parsedName struct {
	objs   []*Obj
	glyphs []int
}

var parseNameCache sync.Map

// string -> parsedName (stored slices are never modified)

// ParseName mirrors ItemManager.parse_name(name) (fresh slices).
func ParseName(name string) ([]*Obj, []int) {
	var r parsedName
	if v, ok := parseNameCache.Load(name); ok {
		r = v.(parsedName)
	} else {
		o, g := parseNameUncached(name)
		r = parsedName{o, g}
		parseNameCache.Store(name, r)
	}
	return append([]*Obj(nil), r.objs...), append([]int(nil), r.glyphs...)
}

type prefixClass struct {
	s string
	c int
}

var parseNameIdPrefixes = []prefixClass{
	{"scroll of ", NH_SCROLL_CLASS},
	{"scrolls of ", NH_SCROLL_CLASS},
	{"spellbook of ", NH_SPBOOK_CLASS},
	{"spellbooks of ", NH_SPBOOK_CLASS},
	{"ring of ", NH_RING_CLASS},
	{"rings of ", NH_RING_CLASS},
	{"wand of ", NH_WAND_CLASS},
	{"wands of ", NH_WAND_CLASS},
	{"", NH_AMULET_CLASS},
	{"potion of ", NH_POTION_CLASS},
	{"potions of ", NH_POTION_CLASS},
	{"", NH_GEM_CLASS},
	{"", NH_ARMOR_CLASS},
	{"pair of ", NH_ARMOR_CLASS},
	{"", NH_WEAPON_CLASS},
	{"", NH_TOOL_CLASS},
	{"", NH_FOOD_CLASS},
	{"", NH_COIN_CLASS},
	{"", NH_ROCK_CLASS},
	// Not in the Python bot: the heavy iron ball and the iron chain have no description, so the
	// description prefix ('', BALL_CLASS) below never matches and they parsed as "unknown" (category
	// VENOM), which failed the category check of the "Iron balls" section in the pick up menu.
	{"", NH_BALL_CLASS},
	{"", NH_CHAIN_CLASS},
}

var parseNameIdSuffixes = []prefixClass{
	{"s", NH_GEM_CLASS},
	{"s", NH_WEAPON_CLASS},
	{"s", NH_TOOL_CLASS},
	{"s", NH_FOOD_CLASS},
	{"s", NH_COIN_CLASS},
	{"s", NH_BALL_CLASS},
	{"s", NH_CHAIN_CLASS},
}

var parseNameDescPrefixes = []prefixClass{
	{"scroll labeled ", NH_SCROLL_CLASS},
	{"scrolls labeled ", NH_SCROLL_CLASS},
	{"", NH_ARMOR_CLASS},
	{"pair of ", NH_ARMOR_CLASS},
	{"", NH_WEAPON_CLASS},
	{"", NH_TOOL_CLASS},
	{"", NH_FOOD_CLASS},
	{"", NH_BALL_CLASS},
}

var parseNameDescSuffixes = []prefixClass{
	{" amulet", NH_AMULET_CLASS},
	{" amulets", NH_AMULET_CLASS},
	{" gem", NH_GEM_CLASS},
	{" gems", NH_GEM_CLASS},
	{" stone", NH_GEM_CLASS},
	{" stones", NH_GEM_CLASS},
	{" potion", NH_POTION_CLASS},
	{" potions", NH_POTION_CLASS},
	{" spellbook", NH_SPBOOK_CLASS},
	{" spellbooks", NH_SPBOOK_CLASS},
	{" ring", NH_RING_CLASS},
	{" rings", NH_RING_CLASS},
	{" wand", NH_WAND_CLASS},
	{" wands", NH_WAND_CLASS},
	{"s", NH_ARMOR_CLASS},
	{"s", NH_WEAPON_CLASS},
	{"s", NH_TOOL_CLASS},
	{"s", NH_FOOD_CLASS},
}

func parseNameUncached(name string) ([]*Obj, []int) {
	switch {
	case name == "wakizashi":
		name = "short sword"
	case name == "ninja-to":
		name = "broadsword"
	case name == "nunchaku":
		name = "flail"
	case name == "shito":
		name = "knife"
	case name == "naginata":
		name = "glaive"
	case name == "gunyoki":
		name = "food ration"
	case name == "osaku":
		name = "lock pick"
	case name == "tanko":
		name = "plate mail"
	case name == "pair of yugake" || name == "yugake":
		name = "pair of leather gloves"
	case name == "kabuto":
		name = "helmet"
	case name == "flint stone" || name == "flint stones":
		name = "flint"
	case name == "unlabeled scroll" || name == "unlabeled scrolls" || name == "blank paper":
		name = "scroll of blank paper"
	case name == "eucalyptus leaves":
		name = "eucalyptus leaf"
	case name == "pair of lenses":
		name = "lenses"
	case strings.HasPrefix(name, "small glob"):
		name = name[len("small "):]
	case name == "knives":
		name = "knife"
	}

	// object identified (look on names)
	objIDs := NewPySet()
	for i := 0; i < NH_NUM_OBJECTS; i++ {
		oc := ObjClasses[i]
		for _, p := range parseNameIdPrefixes {
			if oc.OcClass == p.c {
				objName := oc.OcName
				if !oc.OcNameNone && objName != "" && name == p.s+objName {
					objIDs.Add(IntKey(i))
				}
			}
		}

		for _, s := range parseNameIdSuffixes {
			if oc.OcClass == s.c {
				objName := oc.OcName
				if !oc.OcNameNone && objName != "" {
					match := name == objName+s.s
					if !match && s.c == NH_FOOD_CLASS {
						parts := PySplit(objName)
						match = name == parts[0]+s.s+" "+strings.Join(parts[1:], " ")
					}
					if match {
						objIDs.Add(IntKey(i))
					}
				}
			}
		}
	}

	// object unidentified (look on descriptions)
	appearanceSet := NewPySet()
	for i := 0; i < NH_NUM_OBJECTS; i++ {
		oc := ObjClasses[i]
		for _, p := range parseNameDescPrefixes {
			if oc.OcClass == p.c {
				objDescr := oc.OcDescr
				if !oc.OcDescrNone && objDescr != "" && name == p.s+objDescr {
					appearanceSet.Add(IntKey(i))
				}
			}
		}

		for _, s := range parseNameDescSuffixes {
			if oc.OcClass == s.c {
				objDescr := oc.OcDescr
				if !oc.OcDescrNone && objDescr != "" && name == objDescr+s.s {
					appearanceSet.Add(IntKey(i))
				}
			}
		}
	}

	var appearanceIDs []int
	for _, k := range appearanceSet.Items() {
		appearanceIDs = append(appearanceIDs, k.A)
	}
	// assert len(appearance_ids) == 0 or len({classes}) -- always true

	if (objIDs.Len() > 0) == (len(appearanceIDs) > 0) {
		unk := O_FromName1("unknown")
		return []*Obj{unk}, O_PossibleGlyphsFromObject(unk)
	}

	var objs []*Obj
	var glyphs []int
	if objIDs.Len() > 0 {
		Assert(objIDs.Len() == 1, name)
		objID := objIDs.Items()[0].A
		objs = []*Obj{Objects[objID]}
		for i := NH_GLYPH_OBJ_OFF; i < NH_NUM_OBJECTS+NH_GLYPH_OBJ_OFF; i++ {
			if Objects[i-NH_GLYPH_OBJ_OFF] != nil && ContainsObj(O_PossibilitiesFromGlyph(i), objs[0]) {
				glyphs = append(glyphs, i)
			}
		}
	} else {
		for _, objID := range appearanceIDs {
			glyphs = append(glyphs, objID+NH_GLYPH_OBJ_OFF)
		}
		seen := map[*Obj]bool{}
		var union []*Obj
		for _, g := range glyphs {
			for _, o := range O_PossibilitiesFromGlyph(g) {
				if !seen[o] {
					seen[o] = true
					union = append(union, o)
				}
			}
		}
		objs = SortObjs(union)
		if name != "runed broadsword" {
			for _, i := range appearanceIDs {
				Assert(ObjsEqual(SortObjs(O_PossibilitiesFromGlyph(i+NH_GLYPH_OBJ_OFF)), objs), name)
			}
		}
	}

	return objs, glyphs
}
