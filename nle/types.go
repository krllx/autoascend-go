package nle

import ()

// Action is an NLE action value (the int value of nle.nethack.actions members, e.g. ord('k')).
type Action int

type Permonst struct {
	Idx                         int
	Mname                       string
	Mlet                        byte
	Mlevel, Mmove, Ac, Mr, Geno int
	Cwt, Cnutrit, Msound, Msize int
	Mresists, Mconveys          int
	Mflags1, Mflags2, Mflags3   int
	Difficulty, Mcolor          int
}

// DummyPermonst mirrors agent.get_visible_monsters' dummy_permonst (mname 'unknown', mlet '0', mmove 12).
var DummyPermonst = &Permonst{Idx: -1, Mname: "unknown", Mlet: '0', Mmove: 12}

type ObjClass struct {
	OcClass     int
	OcName      string
	OcNameNone  bool
	OcDescr     string
	OcDescrNone bool
}

type ObjKind int

const (
	KWeapon ObjKind = iota + 1
	KArmor
	KRing
	KAmulet
	KTool
	KContainer
	KWepTool
	KFood
	KPotion
	KScroll
	KSpell
	KWand
	KCoin
	KGem
	KRock
	KMisc
	KQuestItem
)

// Obj is one AutoAscend object namedtuple (autoascend/objects/data.py). Objects are pairwise distinct
// by value, so pointer equality == Python namedtuple equality.
type Obj struct {
	Idx                      int
	Kind                     ObjKind
	Name                     string
	NameNone                 bool // Python None
	Desc                     string
	DescNone                 bool
	Wt                       float64
	WtNone                   bool
	DamageSmall, DamageLarge string

	Kn, Mg, Bi, Prob, Cost, Sdam, Ldam, Hitbon, Typ, Sub, Metal, Color, Mgc, Blk, Power int
	Delay, Ac, Can, C, Spec, Mohs, Chg, Mat, Mrg, Clr, Unk, Tin, Nutrition, Level, Dir  int
	Worth, Gval, Nutr, Glass                                                            int
}

// HasCost mirrors hasattr(obj, 'cost').
func (o *Obj) HasCost() bool {
	switch o.Kind {
	case KWeapon, KArmor, KRing, KContainer, KTool, KWepTool, KPotion, KScroll, KWand:
		return true
	}
	return false
}

// DescOrName mirrors (o.desc or o.name).
func (o *Obj) DescOrName() string {
	if o.Desc != "" {
		return o.Desc
	}
	return o.Name
}

// SokoMapData is one AutoAscend Sokoban map converted for the solver (Err set if the conversion failed).
type SokoMapData struct {
	Err        string
	Grid       [][]int8
	PosY, PosX int
	Answer     [][4]int // y, x, dy, dx
}
