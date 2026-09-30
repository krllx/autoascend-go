package world

import (
	. "aago/grid"
	. "aago/internal/pyrt"
	. "aago/nle"
)

// Port of autoascend/level.py. OWNER: lead.

const (
	Level_DUNGEONS_OF_DOOM = 0
	Level_GNOMISH_MINES    = 2
	Level_QUEST            = 3
	Level_SOKOBAN          = 4
	Level_PLANE            = 1000
)

var Level_DungeonNames = map[int]string{0: "DUNGEONS_OF_DOOM", 2: "GNOMISH_MINES", 3: "QUEST", 4: "SOKOBAN", 1000: "PLANE"}

// StairDest is ((dungeon, level), (y, x)); PosNone marks the (None, None) position.
type StairDest struct {
	Level   LevelKey
	Pos     Pos
	PosNone bool
}

type Level struct {
	DungeonNumber, LevelNumber int
	Walkable, Seen             BoolGrid
	Objects                    I16Grid
	WasOn                      BoolGrid
	Shop, ShopInterior         BoolGrid
	ShopType                   I32Grid
	SearchCount, DoorOpenCount I32Grid
	ItemDisagreementCounter    I32Grid
	Items                      [SY][SX][]*Item
	ItemCount                  I32Grid
	StairDestination           *OMap[Pos, StairDest]
	Altars                     *OMap[Pos, int]
	// CorpsesToEat: defaultdict(lambda: defaultdict(lambda: -10000)); use CorpsesAt for d[pos] access.
	CorpsesToEat *OMap[Pos, *OMap[int, int]]
	Forbidden    BoolGrid

	searchMasks searchMaskCache `snapshot:"-"` // derived from Objects and Walkable, see SearchMasks
}

// SearchMasks is what the exploration's to_search needs from the level's maps alone: which cells are doors,
// how many of the 8 neighbours are stone / wall objects, how many of the 4 orthogonal neighbours are walkable.
type SearchMasks struct {
	IsOnDoor                      BoolGrid
	Stones, Walls, WalkableAround I32Grid
}

// searchMaskCache keeps SearchMasks until Objects or Walkable change (it compares them with its copies, so it
// needs no bookkeeping where the maps are written; a restored snapshot starts with an empty cache).
type searchMaskCache struct {
	valid    bool
	objects  I16Grid
	walkable BoolGrid
	masks    SearchMasks
}

// SearchMasks returns the masks for the current Objects and Walkable (valid until the level changes).
func (l *Level) SearchMasks() *SearchMasks {
	c := &l.searchMasks
	if c.valid && c.objects == l.Objects && c.walkable == l.Walkable {
		return &c.masks
	}
	c.valid, c.objects, c.walkable = true, l.Objects, l.Walkable
	m := &c.masks
	m.IsOnDoor = Isin(&l.Objects, G_DOORS)
	sMask, wMask := Isin(&l.Objects, G_STONE), Isin(&l.Objects, G_WALL)
	// STONE/WALL objects among the 8 neighbours (== summing isin(translate(objects, dy, dx)) since translate
	// fills with glyph 0, which is in neither set); walkable 4-neighbours
	NeighborCount8(&sMask, &m.Stones)
	NeighborCount8(&wMask, &m.Walls)
	NeighborCount4(&l.Walkable, &m.WalkableAround)
	return m
}

func NewLevel(dungeon, level int) *Level {
	l := &Level{DungeonNumber: dungeon, LevelNumber: level,
		StairDestination: NewOMap[Pos, StairDest](), Altars: NewOMap[Pos, int](),
		CorpsesToEat: NewOMap[Pos, *OMap[int, int]]()}
	for y := 0; y < SY; y++ {
		for x := 0; x < SX; x++ {
			l.Objects[y][x] = -1
			l.ShopType[y][x] = SHOP_UNKNOWN
		}
	}
	return l
}

func (l *Level) Key() LevelKey { return LevelKey{l.DungeonNumber, l.LevelNumber} }

// CorpsesAt mirrors level.corpses_to_eat[pos] (inserts an empty inner dict on miss).
func (l *Level) CorpsesAt(p Pos) *OMap[int, int] {
	return l.CorpsesToEat.GetOrInsert(p, func() *OMap[int, int] { return NewOMap[int, int]() })
}

// CorpseAge mirrors corpses_to_eat[pos][monster_id] (inserts -10000 on miss).
func CorpseAge(m *OMap[int, int], monsterID int) int {
	return m.GetOrInsert(monsterID, func() int { return -10000 })
}

// StairEntry is one (pos -> destination or None) of get_stairs().
type StairEntry struct {
	Pos  Pos
	Dest *StairDest // None -> nil
}

// GetStairs mirrors get_stairs(down, up, portal, all) (row-major order of positions).
func (l *Level) GetStairs(down, up, portal, all bool) []StairEntry {
	if all {
		down, up, portal = true, true, true
	}
	Assert(down || up || portal)
	var sets []*GSet
	if down {
		sets = append(sets, G_STAIR_DOWN)
	}
	if up {
		sets = append(sets, G_STAIR_UP)
	}
	mask := Isin(&l.Objects, sets...)
	var r []StairEntry
	for _, p := range mask.Nonzero() {
		e := StairEntry{Pos: p}
		if d, ok := l.StairDestination.Get(p); ok {
			dd := d
			e.Dest = &dd
		}
		r = append(r, e)
	}
	return r
}

func (l *Level) IsLightLevel() bool {
	m := Isin(&l.Objects, NewGSet(SS_S_room, SS_S_litcorr))
	return m.Count() > 15
}
