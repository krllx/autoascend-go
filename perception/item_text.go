package perception

import (
	"regexp"
	"strings"
	"sync"

	. "aago/internal/pyrt"
	. "aago/nle"
)

// Port of ItemManager.parse_text / ItemManager.parse_name (lru_cached pure functions; results copied).

type ParsedText struct {
	Objs       []*Obj
	Glyphs     []int
	Count      int
	Status     int
	Modifier   *int
	Equipped   bool
	AtReady    bool
	MonsterID  int // None -> -1
	ShopStatus int
	ShopPrice  int
	DmgBonus   *float64
	ToHitBonus *float64
	Naming     string
	Comment    string
	Uses       string
}

// copy mirrors utils.copy_result (fresh lists; scalars and pointed-to values are immutable).
func (p ParsedText) copy() ParsedText {
	p.Objs = append([]*Obj(nil), p.Objs...)
	p.Glyphs = append([]int(nil), p.Glyphs...)
	if p.Modifier != nil {
		v := *p.Modifier
		p.Modifier = &v
	}
	if p.DmgBonus != nil {
		v := *p.DmgBonus
		p.DmgBonus = &v
	}
	if p.ToHitBonus != nil {
		v := *p.ToHitBonus
		p.ToHitBonus = &v
	}
	return p
}

// Python's `$` also matches before a trailing newline: `\n?$` reproduces that for findall groups.
var parseTextRe = regexp.MustCompile(`^(a|an|the|\d+)` +
	`( empty)?` +
	`( (cursed|uncursed|blessed))?` +
	`( (very |thoroughly )?(rustproof|poisoned|corroded|rusty|burnt|rotted|partly eaten|partly used|diluted|unlocked|locked|wet|greased))*` +
	`( ([+-]\d+))? ` +
	`([a-zA-z0-9-!'# ]+)` +
	`( \(([0-9]+:[0-9]+|no charge)\))?` +
	`( \(([a-zA-Z0-9; ]+(, flickering|, gleaming|, glimmering)?[a-zA-Z0-9; ]*)\))?` +
	`( \((for sale|unpaid), (\d+ aum, )?((\d+)[a-zA-Z- ]+|no charge)\))?` +
	`\n?$`)

type parseTextKey struct {
	text            string
	category, glyph int
}

var parseTextCache sync.Map

// parseTextKey -> parsedText (stored copies are never modified)

// ParseText mirrors ItemManager.parse_text(text, category, glyph); category/glyph < 0 mean None.
func ParseText(text string, category, glyph int) ParsedText {
	k := parseTextKey{text, category, glyph}
	if r, ok := parseTextCache.Load(k); ok {
		return r.(ParsedText).copy()
	}
	r := parseTextUncached(text, category, glyph)
	parseTextCache.Store(k, r.copy())
	return r
}

var petNames = []string{"Hachi", "Idefix", "Slasher", "Sirius"}

func pf(v float64) *float64 { return &v }

func parseTextUncached(text string, category, glyph int) ParsedText {
	Assert(glyph < 0 || GlyphIsNormalObject(glyph), glyph)

	if category < 0 && glyph >= 0 {
		category = ObjClassOf(GlyphToObj(glyph))
	}
	Assert(glyph < 0 || category < 0 || category == ObjClassOf(GlyphToObj(glyph)))

	Assert(category != NH_RANDOM_CLASS)

	matches := FindAll(parseTextRe, text)
	Assert(len(matches) <= 1, text)
	Assert(len(matches) > 0, text, len(text))

	g := matches[0]
	count := g[0]
	status := g[3]
	modifier := g[8]
	name := g[9]
	uses := g[11]
	info := g[13]
	shopStatus := g[16]
	shopPrice := g[19]
	// TODO: effects, uses

	var equipped, atReady bool
	if info == "being worn" || info == "being worn; slippery" || info == "wielded" || info == "chained to you" ||
		strings.HasPrefix(info, "weapon in ") ||
		strings.HasPrefix(info, "tethered weapon in ") {
		equipped = true
		atReady = false
	} else if info == "at the ready" || info == "in quiver" || info == "in quiver pouch" || info == "lit" {
		equipped = false
		atReady = true
	} else if info == "" || info == "alternate weapon; not wielded" || info == "alternate weapon; notwielded" {
		equipped = false
		atReady = false
	} else {
		Assert(false, info)
	}

	var shopPriceI int
	if shopPrice == "" {
		shopPriceI = 0
	} else {
		shopPriceI = PyInt(shopPrice)
	}

	var countI int
	switch count {
	case "a", "an", "the":
		countI = 1
	default:
		countI = PyInt(count)
	}
	var statusI int
	switch status {
	case "":
		statusI = Item_UNKNOWN
	case "cursed":
		statusI = Item_CURSED
	case "uncursed":
		statusI = Item_UNCURSED
	case "blessed":
		statusI = Item_BLESSED
	default:
		Raise("KeyError", "%q", status)
	}
	// TODO: should be uses -- but the score is better this way
	if uses != "" && statusI == Item_UNKNOWN {
		statusI = Item_UNCURSED
	}
	var modifierP *int
	if modifier != "" {
		sign := 0
		switch modifier[0] {
		case '+':
			sign = 1
		case '-':
			sign = -1
		default:
			Raise("KeyError", "%q", modifier[:1])
		}
		v := sign * PyInt(modifier[1:])
		modifierP = &v
	}
	monsterID := -1

	if strings.Contains(name, " containing ") {
		// TODO: use number of items for verification
		name = name[:strings.Index(name, " containing ")]
	}

	comment := ""
	naming := ""
	if strings.Contains(name, " named ") {
		// TODO: many of these are artifacts
		naming = name[strings.Index(name, " named ")+len(" named "):]
		name = name[:strings.Index(name, " named ")]
		if strings.Contains(naming, "#") {
			// all given names by the bot, starts with #
			pos := strings.Index(naming, "#")
			comment = naming[pos+1:]
			naming = naming[:pos]
		} else {
			comment = ""
		}
	}

	var shopStatusI int
	switch shopStatus {
	case "":
		shopStatusI = Item_NOT_SHOP
	case "for sale":
		shopStatusI = Item_FOR_SALE
	case "unpaid":
		shopStatusI = Item_UNPAID
	default:
		Assert(false, shopStatus)
	}

	if name == "potion of holy water" || name == "potions of holy water" {
		name = "potion of water"
		statusI = Item_BLESSED
	} else if name == "potion of unholy water" || name == "potions of unholy water" {
		name = "potion of water"
		statusI = Item_CURSED
	} else if name == "gold piece" || name == "gold pieces" {
		statusI = Item_UNCURSED
	}

	stripArticle := func(monName string) string {
		if strings.HasPrefix(monName, "a ") {
			monName = monName[2:]
		}
		if strings.HasPrefix(monName, "an ") {
			monName = monName[3:]
		}
		return monName
	}

	// TODO: pass to Item class instance
	if strings.HasPrefix(name, "tin of ") || strings.HasPrefix(name, "tins of ") {
		monName := PyStrip(name[len("tin of "):])
		if strings.HasSuffix(monName, " meat") {
			monName = monName[:len(monName)-len(" meat")]
		}
		monName = stripArticle(monName)
		if monName == "spinach" {
			monsterID = -1
		} else {
			monsterID = GlyphToMon(MON_FromName(monName))
		}
		name = "tin"
	} else if strings.HasSuffix(name, " corpse") || strings.HasSuffix(name, " corpses") {
		monName := PyStrip(name[:strings.Index(name, "corpse")])
		monName = stripArticle(monName)
		monsterID = GlyphToMon(MON_FromName(monName))
		name = "corpse"
	} else if strings.HasPrefix(name, "statue of ") || strings.HasPrefix(name, "statues of ") ||
		strings.HasPrefix(name, "historic statue of ") || strings.HasPrefix(name, "historic statues of ") {
		var monName string
		if strings.HasPrefix(name, "historic") {
			monName = PyStrip(name[len("historic statue of "):])
		} else {
			monName = PyStrip(name[len("statue of "):])
		}
		monName = stripArticle(monName)
		monsterID = GlyphToMon(MON_FromName(monName))
		name = "statue"
	} else if strings.HasPrefix(name, "figurine of ") || strings.HasPrefix(name, "figurines of ") {
		monName := PyStrip(name[len("figurine of "):])
		monName = stripArticle(monName)
		monsterID = GlyphToMon(MON_FromName(monName))
		name = "figurine"
	} else if name == "novel" || name == "paperback" || name == "paperback book" {
		name = "spellbook of novel"
	} else if strings.HasSuffix(name, " egg") || strings.HasSuffix(name, " eggs") {
		monsterID = GlyphToMon(MON_FromName(PyStrip(name[:len(name)-len(" egg")])))
		name = "egg"
	} else if name == "worm teeth" {
		name = "worm tooth"
	}

	var dmgBonus, toHitBonus *float64

	if naming != "" {
		if Contains(petNames, naming) { // pet names
		} else if name == "corpse" {
		} else if name == "spellbook of novel" {
		} else {
			name = naming
		}
	}

	switch name {
	case "Excalibur":
		name = "long sword"
		dmgBonus, toHitBonus = pf(5.5), pf(3)
	case "Mjollnir":
		name = "war hammer"
		dmgBonus, toHitBonus = pf(12.5), pf(3)
	case "Cleaver":
		name = "battle-axe"
		dmgBonus, toHitBonus = pf(3.5), pf(2)
	case "Sting":
		name = "elven dagger"
		dmgBonus, toHitBonus = pf(2.5), pf(3)
	case "Grimtooth":
		name = "orcish dagger"
		dmgBonus, toHitBonus = pf(3.5), pf(1.5)
	case "Sunsword", "Frost Brand", "Fire Brand", "Demonbane", "Giantslayer":
		name = "long sword"
		dmgBonus, toHitBonus = pf(5.5), pf(3)
	case "Vorpal Blade":
		name = "long sword"
		dmgBonus, toHitBonus = pf(1), pf(3)
	case "Orcrist":
		name = "elven broadsword"
		dmgBonus, toHitBonus = pf(6), pf(3)
	case "Magicbane":
		name = "athame"
		dmgBonus, toHitBonus = pf(7.25), pf(2)
	case "Grayswandir", "Werebane":
		name = "silver saber"
		dmgBonus, toHitBonus = pf(15), pf(3)
	case "Stormbringer":
		name = "runesword"
		dmgBonus, toHitBonus = pf(6), pf(3)
	case "Snickersnee":
		name = "katana"
		dmgBonus, toHitBonus = pf(4.5), pf(1)
	case "Trollsbane":
		name = "morning star"
		dmgBonus, toHitBonus = pf(5.25), pf(3)
	case "Ogresmasher":
		name = "war hammer"
		dmgBonus, toHitBonus = pf(3), pf(3)
	case "Dragonbane":
		name = "broadsword"
		dmgBonus, toHitBonus = pf(4.75), pf(3)
	}

	objs, retGlyphs := ParseName(name)
	Assert(category < 0 || category == O_GetCategory(objs[0]), text, category, O_GetCategory(objs[0]))

	if glyph >= 0 {
		Assert(Contains(retGlyphs, glyph))
		pos := O_PossibilitiesFromGlyph(glyph)
		if n := objs[0].Name; n != "elven broadsword" && n != "runed broadsword" {
			for _, o := range objs {
				Assert(ContainsObj(pos, o), objs, pos)
			}
		}
		retGlyphs = []int{glyph}
		pos2 := O_PossibilitiesFromGlyph(glyph)
		seen := map[*Obj]bool{}
		inter := []*Obj{}
		for _, o := range objs {
			if !seen[o] && ContainsObj(pos2, o) {
				seen[o] = true
				inter = append(inter, o)
			}
		}
		objs = SortObjs(inter)
	}

	return ParsedText{
		Objs: objs, Glyphs: retGlyphs, Count: countI, Status: statusI, Modifier: modifierP,
		Equipped: equipped, AtReady: atReady, MonsterID: monsterID, ShopStatus: shopStatusI,
		ShopPrice: shopPriceI, DmgBonus: dmgBonus, ToHitBonus: toHitBonus, Naming: naming,
		Comment: comment, Uses: uses,
	}
}
