package nle

import (
	"strconv"
	"strings"

	. "aago/internal/pyrt"
)

var (
	gIsMonster      = bitsToBools(glyphBits_is_monster)
	gIsPet          = bitsToBools(glyphBits_is_pet)
	gIsBody         = bitsToBools(glyphBits_is_body)
	gIsStatue       = bitsToBools(glyphBits_is_statue)
	gIsObject       = bitsToBools(glyphBits_is_object)
	gIsNormalObject = bitsToBools(glyphBits_is_normal_object)
)

func bitsToBools(s string) []bool {
	r := make([]bool, len(s))
	for i := range s {
		r[i] = s[i] == '1'
	}
	return r
}

func glyphTab(t []bool, g int) bool { return g >= 0 && g < len(t) && t[g] }

func GlyphIsMonster(g int) bool      { return glyphTab(gIsMonster, g) }
func GlyphIsPet(g int) bool          { return glyphTab(gIsPet, g) }
func GlyphIsBody(g int) bool         { return glyphTab(gIsBody, g) }
func GlyphIsStatue(g int) bool       { return glyphTab(gIsStatue, g) }
func GlyphIsObject(g int) bool       { return glyphTab(gIsObject, g) }
func GlyphIsNormalObject(g int) bool { return glyphTab(gIsNormalObject, g) }
func GlyphToMon(g int) int           { return int(glyphToMon[g]) }
func GlyphToObj(g int) int           { return int(glyphToObj[g]) }

// Glyph sets (glyph/__init__.py class G).
func gset(name string) *GSet {
	l, ok := gSetLists[name]
	if !ok {
		panic("no glyph set " + name)
	}
	s := NewGSet(l...)
	namedGSets = append(namedGSets, s)
	return s
}

// namedGSets are the G_* sets, in definition order (registered as static tables for snapshots).
var namedGSets []*GSet

var (
	G_FLOOR          = gset("FLOOR")
	G_VISIBLE_FLOOR  = gset("VISIBLE_FLOOR")
	G_STONE          = gset("STONE")
	G_WALL           = gset("WALL")
	G_STAIR_UP       = gset("STAIR_UP")
	G_STAIR_DOWN     = gset("STAIR_DOWN")
	G_ALTAR          = gset("ALTAR")
	G_FOUNTAIN       = gset("FOUNTAIN")
	G_DOOR_CLOSED    = gset("DOOR_CLOSED")
	G_DOOR_OPENED    = gset("DOOR_OPENED")
	G_DOORS          = gset("DOORS")
	G_BARS           = gset("BARS")
	G_MONS           = gset("MONS")
	G_PETS           = gset("PETS")
	G_WARNING        = gset("WARNING")
	G_INVISIBLE_MON  = gset("INVISIBLE_MON")
	G_SHOPKEEPER     = gset("SHOPKEEPER")
	G_ORACLE         = gset("ORACLE")
	G_GUARD          = gset("GUARD")
	G_STATUES        = gset("STATUES")
	G_BODIES         = gset("BODIES")
	G_OBJECTS        = gset("OBJECTS")
	G_BOULDER        = gset("BOULDER")
	G_NORMAL_OBJECTS = gset("NORMAL_OBJECTS")
	G_FOOD_OBJECTS   = gset("FOOD_OBJECTS")
	G_TRAPS          = gset("TRAPS")
	G_SWALLOW        = gset("SWALLOW")
)

// ---- MON (glyph/monster.py)

func MON_IsMonster(g int) bool { return GlyphIsMonster(g) }
func MON_IsPet(g int) bool     { return GlyphIsPet(g) }

// MON_Permonst mirrors MON.permonst(glyph).
func MON_Permonst(g int) *Permonst {
	if GlyphIsMonster(g) {
		return &Permonsts[GlyphToMon(g)]
	} else if GlyphIsPet(g) {
		Raise("AttributeError", "module 'nle.nethack' has no attribute 'glyph_to_pet'")
	} else if GlyphIsBody(g) {
		return &Permonsts[g-NH_GLYPH_BODY_OFF]
	}
	Assert(false, g)
	return nil
}

var monIDByName = func() map[string]int {
	m := map[string]int{}
	for i := len(Permonsts) - 1; i >= 0; i-- {
		m[Permonsts[i].Mname] = i // first match wins
	}
	return m
}()

// MON_IdFromName mirrors MON.id_from_name (AssertionError if unknown).
func MON_IdFromName(name string) int {
	i, ok := monIDByName[name]
	Assert(ok, name)
	return i
}

// MON_IsName: name is a monster name (MON_IdFromName would not fail).
func MON_IsName(name string) bool {
	_, ok := monIDByName[name]
	return ok
}

func MON_FromName(name string) int     { return NH_GLYPH_MON_OFF + MON_IdFromName(name) }
func MON_BodyFromName(name string) int { return MON_IdFromName(name) + NH_GLYPH_BODY_OFF }

// ---- SHOP, Hunger, C

const SHOP_UNKNOWN = 0

// SHOP_Names is SHOP.name2id in dict order (index == id).
var SHOP_Names = []string{"UNKNOWN", "general store", "used armor dealership", "second-hand bookstore",
	"liquor emporium", "antique weapons outlet", "delicatessen", "jewelers", "quality apparel and accessories",
	"hardware store", "rare books", "health food store", "lighting store"}

func SHOP_Name2ID(name string) (int, bool) {
	for i, n := range SHOP_Names {
		if n == name {
			return i, true
		}
	}
	return 0, false
}

const (
	Hunger_SATIATED   = 0
	Hunger_NOT_HUNGRY = 1
	Hunger_HUNGRY     = 2
	Hunger_WEAK       = 3
	Hunger_FAINTING   = 4
)

// WEA_ExpectedDamage mirrors WEA.expected_damage.
func WEA_ExpectedDamage(s string) float64 {
	if strings.Contains(s, "-") {
		Raise("NotImplementedError", "")
	}
	ret := 0.0
	for rest, more := s, true; more; { // the words of s.split("+")
		var word string
		word, rest, more = strings.Cut(rest, "+")
		var sides, mult int
		if i := strings.Index(word, "d"); i >= 0 {
			sides = pyInt(word[i+1:])
			if word[:i] == "" {
				mult = 1
			} else {
				mult = pyInt(word[:i])
			}
		} else {
			sides = 1
			mult = pyInt(word)
		}
		ret += float64(mult) * float64(1+sides) / 2
	}
	return ret
}

// pyInt mirrors int(str) (ValueError on failure; surrounding whitespace allowed).
func pyInt(s string) int {
	v, err := strconv.Atoi(PyStrip(s))
	if err != nil {
		Raise("ValueError", "invalid literal for int(): %q", s)
	}
	return v
}

func PyInt(s string) int { return pyInt(s) }

// ONLY_RANGED_SLOW_MONSTERS mirrors combat/utils.py (here because the glyph flag table uses it).
var ONLY_RANGED_SLOW_MONSTERS = []string{"floating eye", "blue jelly", "brown mold", "gas spore", "acid blob"}

// G_ONLY_RANGED_SLOW_MONS are the glyphs of G.MONS whose monster is in ONLY_RANGED_SLOW_MONSTERS (agent.bfs keeps
// off them).
var G_ONLY_RANGED_SLOW_MONS = func() *GSet {
	var ids []int
	for _, g := range G_MONS.list {
		if Contains(ONLY_RANGED_SLOW_MONSTERS, MON_Permonst(g).Mname) {
			ids = append(ids, g)
		}
	}
	return NewGSet(ids...)
}()
