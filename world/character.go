package world

import (
	"bytes"
	"regexp"
	"strings"

	. "aago/internal/pyrt"
	. "aago/nle"
)

// Port of autoascend/character.py.

type Property struct{ agent *Agent }

// statusHas: the bottom tty line contains s, or one of its short forms as a word after the time. When the
// line is too long, tty shortens the conditions (wintty.c: Conf -> Cnf -> Cf, Hallu -> Hal -> Hl, ...). Not in
// the Python bot, which looked for the full word only: with "Hungry Stun Cnf Hal" it missed the hallucination,
// sent 'M' to a ';' menu that has no such entry when hallucinating and failed (seed 3128).
func (p *Property) statusHas(s string, short ...string) bool {
	line := p.agent.LastObservation.TtyChars[23][:]
	if bytes.Contains(line, []byte(s)) {
		return true
	}
	i := bytes.Index(line, []byte("T:"))
	if i < 0 {
		return false
	}
	for _, f := range bytes.Fields(line[i:]) {
		for _, w := range short {
			if string(f) == w {
				return true
			}
		}
	}
	return false
}

func (p *Property) Confusion() bool { return p.statusHas("Conf", "Cnf", "Cf") }

func (p *Property) Stun() bool { return p.statusHas("Stun", "St") }

func (p *Property) Hallu() bool { return p.statusHas("Hallu", "Hal", "Hl") }

func (p *Property) Blind() bool { return p.statusHas("Blind", "Blnd", "Bl") }

func (p *Property) Polymorph() bool {
	a := p.agent
	g := int(a.Glyphs()[a.Blstats.Y][a.Blstats.X])
	if !GlyphIsMonster(g) {
		return false
	}
	return a.Character.SelfGlyph != g
}

var nameToRole = map[string]int{
	"Archeologist": Character_ARCHEOLOGIST, "Barbarian": Character_BARBARIAN, "Caveman": Character_CAVEMAN,
	"Cavewoman": Character_CAVEMAN, "Healer": Character_HEALER, "Knight": Character_KNIGHT, "Monk": Character_MONK,
	"Priest": Character_PRIEST, "Priestess": Character_PRIEST, "Ranger": Character_RANGER, "Rogue": Character_ROGUE,
	"Samurai": Character_SAMURAI, "Tourist": Character_TOURIST, "Valkyrie": Character_VALKYRIE, "Wizard": Character_WIZARD,
}

var nameToAlignment = map[string]int{"chaotic": Character_CHAOTIC, "neutral": Character_NEUTRAL, "lawful": Character_LAWFUL, "unaligned": Character_UNALIGNED}

var nameToRace = map[string]int{"human": Character_HUMAN, "dwarf": Character_DWARF, "dwarven": Character_DWARF, "elf": Character_ELF,
	"elven": Character_ELF, "gnome": Character_GNOME, "gnomish": Character_GNOME, "orc": Character_ORC, "orcish": Character_ORC}

var nameToGender = map[string]int{"male": Character_MALE, "female": Character_FEMALE}

// skill names in dict order (regex alternation order matters)
var skillTypeNames = []string{"dagger", "knife", "axe", "pick-axe", "short sword", "broadsword", "long sword",
	"two-handed sword", "scimitar", "saber", "club", "mace", "morning star", "flail", "hammer", "quarterstaff",
	"polearms", "spear", "trident", "lance", "bow", "sling", "crossbow", "dart", "shuriken", "boomerang", "whip",
	"unicorn horn", "attack spells", "healing spells", "divination spells", "enchantment spells", "clerical spells",
	"escape spells", "matter spells", "bare handed combat", "martial arts", "two weapon combat", "riding"}

var skillTypeIDs = []int{O_P_DAGGER, O_P_KNIFE, O_P_AXE, O_P_PICK_AXE, O_P_SHORT_SWORD, O_P_BROAD_SWORD, O_P_LONG_SWORD,
	O_P_TWO_HANDED_SWORD, O_P_SCIMITAR, O_P_SABER, O_P_CLUB, O_P_MACE, O_P_MORNING_STAR, O_P_FLAIL, O_P_HAMMER,
	O_P_QUARTERSTAFF, O_P_POLEARMS, O_P_SPEAR, O_P_TRIDENT, O_P_LANCE, O_P_BOW, O_P_SLING, O_P_CROSSBOW, O_P_DART,
	O_P_SHURIKEN, O_P_BOOMERANG, O_P_WHIP, O_P_UNICORN_HORN, O_P_ATTACK_SPELL, O_P_HEALING_SPELL, O_P_DIVINATION_SPELL,
	O_P_ENCHANTMENT_SPELL, O_P_CLERIC_SPELL, O_P_ESCAPE_SPELL, O_P_MATTER_SPELL, O_P_BARE_HANDED_COMBAT,
	O_P_BARE_HANDED_COMBAT, O_P_TWO_WEAPON_COMBAT, O_P_RIDING}

var possibleSkillTypes = []string{"Fighting Skills", "Weapon Skills", "Spellcasting Skills"}

var possibleSkillLevels = []string{"Unskilled", "Basic", "Skilled", "Expert", "Master", "Grand Master"}

var nameToSkillLevel = map[string]int{"Restricted": 0, "Unskilled": 1, "Basic": 2, "Skilled": 3, "Expert": 4, "Master": 5, "Grand Master": 6}

type bonusPair struct{ hit, dmg float64 }

var weaponBonus = map[int]bonusPair{0: {-4, 2}, 1: {-4, 2}, 2: {0, 0}, 3: {2, 1}, 4: {3, 2}}

var unarmedBonus = map[int]bonusPair{0: {1, 0}, 1: {1, 0}, 2: {1, 1}, 3: {2, 1}, 4: {2, 2}, 5: {3, 2}, 6: {3, 3}}

var martialBonus = map[int]bonusPair{0: {1, 0}, 1: {2, 1}, 2: {3, 3}, 3: {4, 4}, 4: {5, 6}, 5: {6, 7}, 6: {7, 9}}

var (
	parseRe1      = regexp.MustCompile(`You are a ([a-z]+) (([a-z]+) )?([a-z]+) ([A-Z][a-z]+).`)
	parseRe2      = regexp.MustCompile(`You are an? ([a-zA-Z ]+), a level (\d+) (([a-z]+) )?([a-z]+) ([A-Z][a-z]+). *You are ([a-z]+)`)
	enhanceLineRe = regexp.MustCompile(`^([a-zA-Z] -)?#?\*? *(` + strings.Join(skillTypeNames, "|") + `) *\[(` +
		strings.Join(possibleSkillLevels, "|") + `)\]`)
)

type Character struct {
	agent                         *Agent
	Prop                          *Property
	Role, Alignment, Race, Gender int // PyNone before parse
	SelfGlyph                     int // None -> -1
	SkillLevels                   []int
	UpgradableSkills              *OMap[int, string]
	IsLycanthrope                 bool
}

func NewCharacter(a *Agent) *Character {
	return &Character{agent: a, Prop: &Property{agent: a}, Role: PyNone, Alignment: PyNone, Race: PyNone,
		Gender: PyNone, SelfGlyph: -1, SkillLevels: make([]int, O_P_RIDING+1), UpgradableSkills: NewOMap[int, string]()}
}

func (c *Character) Update() {
	if strings.Contains(c.agent.Message, "You feel feverish.") {
		c.IsLycanthrope = true
	}
	if strings.Contains(c.agent.Message, "You feel purified.") {
		c.IsLycanthrope = false
	}
}

func (c *Character) CarryingCapacity() int {
	return min(1000, (c.agent.Blstats.StrengthPercentage+c.agent.Blstats.Constitution)*25+50)
}

func mustKey(m map[string]int, k string) int {
	v, ok := m[k]
	if !ok {
		Raise("KeyError", "%q", k)
	}
	return v
}

func (c *Character) Parse(text string) {
	var alignment, gender, race, role string
	if ms := FindAll(parseRe1, text); len(ms) == 1 {
		alignment, gender, race, role = ms[0][0], ms[0][2], ms[0][3], ms[0][4]
	} else {
		ms := FindAll(parseRe2, text)
		Assert(len(ms) == 1, text)
		gender, race, role, alignment = ms[0][3], ms[0][4], ms[0][5], ms[0][6]
	}
	if gender == "" {
		switch role {
		case "Priestess", "Cavewoman", "Valkyrie":
			gender = "female"
		case "Priest", "Caveman":
			gender = "male"
		default:
			Assert(false, text)
		}
	}
	c.Role = mustKey(nameToRole, role)
	c.Alignment = mustKey(nameToAlignment, alignment)
	c.Race = mustKey(nameToRace, race)
	c.Gender = mustKey(nameToGender, gender)
}

func (c *Character) ParseEnhanceView() {
	popup := c.agent.Popup
	if len(popup) == 0 {
		Raise("IndexError", "list index out of range")
	}
	if popup[0] != "Current skills:" && popup[0] != "Pick a skill to advance:" {
		Raise("ValueError", "Invalid ehance popup text format.")
	}
	c.UpgradableSkills = NewOMap[int, string]()
	for _, line := range popup[1:] {
		ls := PyStrip(line)
		if Contains(possibleSkillTypes, ls) ||
			ls == `(Skill flagged by "#" cannot be enhanced any further.)` ||
			ls == `(Skills flagged by "#" cannot be enhanced any further.)` ||
			ls == `(Skill flagged by "*" may be enhanced when you're more experienced.)` ||
			ls == `(Skills flagged by "*" may be enhanced when you're more experienced.)` {
			continue
		}
		ms := FindAll(enhanceLineRe, line)
		Assert(len(ms) == 1, line)
		letter, skillType, skillLevel := ms[0][0], ms[0][1], ms[0][2]
		tid := skillTypeIDs[Index(skillTypeNames, skillType)]
		if letter != "" {
			letter = letter[:1]
			Assert(!Contains(c.UpgradableSkills.Values(), letter))
			c.UpgradableSkills.Set(tid, letter)
		}
		c.SkillLevels[tid] = mustKey(nameToSkillLevel, skillLevel)
	}
}

func (c *Character) strDexToHitBonus() float64 {
	bonus := 0
	s, d := c.agent.Blstats.Strength, c.agent.Blstats.Dexterity
	switch {
	case s < 6:
		bonus -= 2
	case s < 8:
		bonus -= 1
	case s < 17:
	case s <= 18+50:
		bonus += 1
	case s < 18+100:
		bonus += 2
	default:
		bonus += 3
	}
	switch {
	case d < 4:
		bonus -= 3
	case d < 6:
		bonus -= 2
	case d < 8:
		bonus -= 1
	case d < 14:
	default:
		return float64(bonus + d - 14)
	}
	return float64(bonus)
}

func objHasSub(o *Obj) bool {
	switch o.Kind {
	case KWeapon, KArmor, KWepTool, KSpell:
		return true
	}
	return false
}

func (c *Character) weaponSkillBonus(item *Item) (float64, float64) {
	get := func(m map[int]bonusPair, lvl int) (float64, float64) {
		b, ok := m[lvl]
		if !ok {
			Raise("KeyError", "%d", lvl)
		}
		return b.hit, b.dmg
	}
	if item == nil {
		if c.Role == Character_MONK || c.Role == Character_SAMURAI {
			return get(martialBonus, c.SkillLevels[O_P_BARE_HANDED_COMBAT])
		}
		return get(unarmedBonus, c.SkillLevels[O_P_BARE_HANDED_COMBAT])
	}
	for _, o := range item.Objs {
		if !objHasSub(o) {
			Raise("AttributeError", "no attribute 'sub'")
		}
		Assert(item.Objs[0].Sub == o.Sub)
	}
	sub := Abs(item.Objs[0].Sub)
	if !(sub >= 0 && sub < len(c.SkillLevels)) {
		Raise("ValueError", "Invalid item sub")
	}
	return get(weaponBonus, c.SkillLevels[sub])
}

func (c *Character) GetRangedBonus(launcher, ammo *Item, largeMonster bool) (float64, float64) {
	if launcher != nil {
		Assert(launcher.IsLauncher())
		Assert(ammo.IsFiredProjectile(nil))
	} else {
		Assert(ammo.IsThrownProjectile())
	}
	roll, dmg := 0.0, 0.0
	if launcher != nil {
		var hit float64
		hit, dmg = c.weaponSkillBonus(launcher)
		roll += hit
		h, _ := launcher.GetWeaponBonus(largeMonster)
		roll += h
		_, d := launcher.GetWeaponBonus(largeMonster)
		dmg += d
	}
	h, _ := ammo.GetWeaponBonus(largeMonster)
	roll += h
	_, d := ammo.GetWeaponBonus(largeMonster)
	dmg += d
	return roll, PyMaxF(0, dmg)
}

func (c *Character) GetRange(launcher, ammo *Item) int { return 7 }

func (c *Character) GetMeleeBonus(item *Item, largeMonster bool) (float64, float64) {
	a := c.agent
	roll := 1 + c.strDexToHitBonus() + float64(a.Blstats.ExperienceLevel)
	if c.Role == Character_MONK {
		if a.Inventory.Items.Suit != nil {
			roll -= 20
		} else if item == nil && a.Inventory.Items.OffHand == nil {
			roll += float64(FloorDiv(a.Blstats.ExperienceLevel, 3) + 2)
		}
	}
	hit, dmg := c.weaponSkillBonus(item)
	roll += hit
	if item != nil {
		if item.IsLauncher() || item.IsFiredProjectile(nil) || item.Objs[0].Name == "dart" || item.Objs[0].Name == "shuriken" {
			dmg = 1.5
		} else {
			_, d := item.GetWeaponBonus(largeMonster)
			dmg += d
		}
		h, _ := item.GetWeaponBonus(largeMonster)
		roll += h
	} else {
		dmg += 1.5
	}
	return roll, PyMaxF(0, dmg)
}

// Port of character.py constants; see character.go, monster_tracker.go.

const PyNone = -1000

// None marker for Character.role/alignment/race/gender before parse()

const (
	Character_UNKNOWN = -1

	Character_ARCHEOLOGIST = 0
	Character_BARBARIAN    = 1
	Character_CAVEMAN      = 2
	Character_HEALER       = 3
	Character_KNIGHT       = 4
	Character_MONK         = 5
	Character_PRIEST       = 6
	Character_RANGER       = 7
	Character_ROGUE        = 8
	Character_SAMURAI      = 9
	Character_TOURIST      = 10
	Character_VALKYRIE     = 11
	Character_WIZARD       = 12

	Character_CHAOTIC   = 0
	Character_NEUTRAL   = 1
	Character_LAWFUL    = 2
	Character_UNALIGNED = 3

	Character_HUMAN = 0
	Character_DWARF = 1
	Character_ELF   = 2
	Character_GNOME = 3
	Character_ORC   = 4

	Character_MALE   = 0
	Character_FEMALE = 1
)
