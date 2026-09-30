// Package botsim is a scripted game for unit tests of the bot's routines (frames) without NLE.
//
// A Game is a small NetHack-like map: walls, floor, doorways, doors (closed, locked, open), stairs, pits and
// holes, boulders, giant mimics that look like boulders, objects lying on the floor, and the hero on it. Its
// Terminal plays the keys a routine sends: the eight directions (walking, opening doors, pushing boulders by
// the Sokoban rules), kick, search and wait with a count, the travel command with its cursor, and the #terrain
// view. After every key it writes into the agent what the routine reads: the glyphs of the observation, the
// position and turn in blstats, the cursor, the message, and the level memory (Agent.UpdateLevel); then it runs
// the on_update watches like the end of the update phase: after a key outside atom operations, and when the
// outermost atom operation has ended (seen at the next key).
//
// The real update phase (inventory, monsters) is switched off: the agent's IsUpdatingState stays set, so
// sense.UpdateState returns at once. Other keys, dialogs and raw keys are not simulated (a Go panic names the
// key). Test support only: the bot never imports this package.
package botsim

import (
	"fmt"
	"reflect"
	"strings"

	"aago/engine"
	. "aago/grid"
	. "aago/internal/pyrt"
	. "aago/nle"
	"aago/world"
)

type (
	Frame = engine.Frame[*world.Agent]
	Ctx   = engine.Ctx[*world.Agent]
	Cmd   = engine.Cmd[*world.Agent]
)

// Terrain is what lies under a cell.
type Terrain uint8

const (
	Stone Terrain = iota
	Wall
	Floor
	Doorway    // no door: walkable, also diagonally (like a broken door)
	ClosedDoor // blocks like a wall; the hero opens it by walking into it, unless it is locked
	OpenDoor   // no diagonal moves into or out of it
	StairsUp
	StairsDown
	Pit
	Hole
	Fountain
	Altar
)

// Glyphs the game shows (NLE numbering; botsim_test checks them against the nle glyph sets).
const (
	GlyphStone      = 2359 // S_stone
	GlyphWall       = 2360 // S_vwall
	GlyphDoorway    = 2371 // S_ndoor
	GlyphClosedDoor = 2374 // S_vcdoor
	GlyphOpenDoor   = 2372 // S_vodoor
	GlyphFloor      = 2378 // S_room (lit)
	GlyphStairsUp   = 2382 // S_upstair
	GlyphStairsDown = 2383 // S_dnstair
	GlyphPit        = 2411 // S_pit
	GlyphHole       = 2413 // S_hole
	GlyphFountain   = 2390 // S_fountain
	GlyphAltar      = 2386 // S_altar
	GlyphBoulder    = 2353 // the boulder object
	GlyphScroll     = 2155 // an object on the floor (a scroll; any object glyph does)
)

// GlyphHero is the hero as the game shows it (a player monster).
var GlyphHero = MON_FromName("wizard")

// Game is the map and the hero. Fields are exported for the tests to set up and inspect.
type Game struct {
	T       [SY][SX]Terrain
	Boulder BoolGrid
	Locked  BoolGrid      // closed doors that do not open
	Object  [SY][SX]int16 // glyph of an object lying on the cell, 0: none
	Y, X    int           // the hero
	Turn    int
	Sokoban bool // pushing follows the Sokoban rules: no diagonal pushes, no squeezing between boulders
	Message string

	StopTravel int // > 0: the next travel is interrupted after that many steps
	// Mimic: giant mimics that look like boulders (the top Sokoban levels have two). One is revealed, and gone,
	// when the hero walks into it or searches next to it; a boulder pushed at it does not move.
	Mimic BoolGrid
	// OnReveal is raised in the routine (at its next key outside an atom operation) when a mimic is revealed,
	// the way the bot's preempting fight layer interrupts it; nil: nothing is raised.
	OnReveal any
	// Respond, when set, sees every key first; it returns true when it played the key itself (a test scripts
	// the prompts of a command this way: set g.Message, change the map).
	Respond func(g *Game, act Action) bool
	Prompt  string // the message the key answers (the one before it)

	Keys      []Action // every key played
	Pushes    int      // boulders moved
	Filled    int      // pits and holes plugged by boulders
	Fell      []Pos    // traps the hero walked into
	Travelled int      // travel commands that moved the hero
	Revealed  int      // mimics revealed

	pending  any    // OnReveal waiting for the next key
	atom     any    // the outermost atom operation frame of the last key, while its watches wait
	count    int    // a count typed before a command
	ext      string // an extended command being typed ("#te")
	kick     bool   // the kick command waits for its direction
	menu     bool   // the #terrain menu is up
	terrain  bool   // the #terrain view is shown
	cursor   *Pos   // the travel cursor while the command asks where to
	travelTo *Pos   // the last travel destination (NetHack starts the next travel cursor there)
}

// Parse builds a game from a map drawn in text, with its top-left character at (y0, x0) of the level:
// '-' and '|' walls, '.' floor, '+' closed door, '#' doorway, '<' '>' stairs, '^' hole, '_' pit, '0' a boulder
// on floor, '?' an object on floor, '@' the hero on floor, '{' fountain, 'A' altar, ' ' stone.
func Parse(y0, x0 int, rows ...string) *Game {
	g := &Game{Y: -1}
	for i, row := range rows {
		for j, ch := range row {
			y, x := y0+i, x0+j
			if !InBounds(y, x) {
				panic(fmt.Sprintf("botsim: (%d, %d) outside the level", y, x))
			}
			t := Floor
			switch ch {
			case '-', '|':
				t = Wall
			case ' ':
				t = Stone
			case '+':
				t = ClosedDoor
			case '#':
				t = Doorway
			case '<':
				t = StairsUp
			case '>':
				t = StairsDown
			case '^':
				t = Hole
			case '_':
				t = Pit
			case '{':
				t = Fountain
			case 'A':
				t = Altar
			case '0':
				g.Boulder[y][x] = true
			case '?':
				g.Object[y][x] = GlyphScroll
			case '@':
				g.Y, g.X = y, x
			case '.':
			default:
				panic(fmt.Sprintf("botsim: unknown map character %q", ch))
			}
			g.T[y][x] = t
		}
	}
	if g.Y < 0 {
		panic("botsim: no hero ('@') on the map")
	}
	return g
}

// Glyph is what the game shows at (y, x).
func (g *Game) Glyph(y, x int) int16 {
	switch {
	case g.terrain: // known terrain and traps only
	case y == g.Y && x == g.X:
		return int16(GlyphHero)
	case g.Boulder[y][x] || g.Mimic[y][x]:
		return GlyphBoulder
	case g.Object[y][x] != 0:
		return g.Object[y][x]
	}
	return [...]int16{Stone: GlyphStone, Wall: GlyphWall, Floor: GlyphFloor, Doorway: GlyphDoorway,
		ClosedDoor: GlyphClosedDoor, OpenDoor: GlyphOpenDoor, StairsUp: GlyphStairsUp,
		StairsDown: GlyphStairsDown, Pit: GlyphPit, Hole: GlyphHole, Fountain: GlyphFountain,
		Altar: GlyphAltar}[g.T[y][x]]
}

// Traps is the number of pits and holes left.
func (g *Game) Traps() int {
	n := 0
	for y := range SY {
		for x := range SX {
			if g.T[y][x] == Pit || g.T[y][x] == Hole {
				n++
			}
		}
	}
	return n
}

// solid: a wall, stone or closed door (the hero and boulders cannot enter it).
func (g *Game) solid(y, x int) bool {
	t := g.T[y][x]
	return t == Stone || t == Wall || t == ClosedDoor
}

// rock: what the hero cannot squeeze past diagonally in Sokoban (bad_rock).
func (g *Game) rock(y, x int) bool { return g.solid(y, x) || g.Boulder[y][x] }

// Play is the game's answer to one key.
func (g *Game) Play(act Action) {
	g.Keys = append(g.Keys, act)
	g.Prompt, g.Message = g.Message, ""
	if g.Respond != nil && g.Respond(g, act) {
		return
	}
	if g.cursor != nil { // the travel command asks where to
		if d, ok := dirOf(act); ok {
			if y, x := g.cursor.Y+d.Y, g.cursor.X+d.X; InBounds(y, x) {
				g.cursor.Y, g.cursor.X = y, x
			}
			return
		}
		switch act {
		case A_MiscDirection_WAIT, Action(','), Action(';'), Action(':'):
			to := *g.cursor
			g.cursor, g.travelTo = nil, &to
			if dis := g.travelDistances(); dis[to.Y][to.X] > 0 {
				n := int(dis[to.Y][to.X])
				for g.StopTravel > 0 && n > g.StopTravel { // interrupted: walk back from the end
					to = g.stepBack(&dis, to)
					n--
				}
				g.StopTravel = 0
				g.Y, g.X = to.Y, to.X
				g.Turn += n
				g.Travelled++
			}
		case A_Command_ESC:
			g.cursor = nil
		default:
			panic(fmt.Sprintf("botsim: key %d while choosing where to travel", act))
		}
		return
	}
	switch {
	case g.terrain:
		if act != A_Command_ESC {
			panic(fmt.Sprintf("botsim: key %d in the #terrain view", act))
		}
		g.terrain = false
		return
	case g.menu:
		if act != Action('b') {
			panic(fmt.Sprintf("botsim: key %d in the #terrain menu", act))
		}
		g.menu, g.terrain = false, true
		g.Message = "Showing known terrain and traps only...  (For instructions type a '?')"
		return
	case g.ext != "" && act == A_MiscAction_MORE:
		if g.ext != "#te" {
			panic("botsim: extended command " + g.ext)
		}
		g.ext, g.menu = "", true
		return
	case g.ext != "" || act == Action('#'):
		g.ext += string(rune(act))
		g.Message = g.ext
		return
	case act >= Action('0') && act <= Action('9'):
		g.count = g.count*10 + int(act-Action('0'))
		return
	}
	n := max(1, g.count)
	g.count = 0
	if d, ok := dirOf(act); ok {
		if g.kick {
			g.kick = false
			g.Turn++
			if y, x := g.Y+d.Y, g.X+d.X; InBounds(y, x) && g.T[y][x] == ClosedDoor {
				g.Message = "WHAMM!!"
			}
			return
		}
		g.move(d.Y, d.X)
		return
	}
	switch act {
	case A_Command_SEARCH:
		for range n {
			g.Turn++
			found := false
			for dy := -1; dy <= 1; dy++ {
				for dx := -1; dx <= 1; dx++ {
					if y, x := g.Y+dy, g.X+dx; InBounds(y, x) && g.Mimic[y][x] {
						g.reveal(y, x, "You find a giant mimic.")
						found = true
					}
				}
			}
			if found {
				break
			}
		}
	case A_MiscDirection_WAIT:
		g.Turn += n
	case A_Command_KICK:
		g.kick = true
		g.Message = "In what direction?"
	case A_Command_TRAVEL:
		c := Pos{g.Y, g.X}
		if g.travelTo != nil {
			c = *g.travelTo
		}
		g.cursor = &c
		g.Message = "Where do you want to travel to?"
	default:
		panic(fmt.Sprintf("botsim: key %d is not simulated", act))
	}
}

func dirOf(act Action) (Pos, bool) {
	for _, name := range []string{"n", "s", "e", "w", "ne", "se", "nw", "sw"} {
		if world.DirActions[name] == act {
			return Pos{B2i(strings.Contains(name, "s")) - B2i(strings.Contains(name, "n")),
				B2i(strings.Contains(name, "e")) - B2i(strings.Contains(name, "w"))}, true
		}
	}
	return Pos{}, false
}

// canStep: the hero may step from (y, x) by (dy, dx), not counting boulders on the target.
func (g *Game) canStep(y, x, dy, dx int) bool {
	ny, nx := y+dy, x+dx
	if !InBounds(ny, nx) || g.solid(ny, nx) {
		return false
	}
	if dy != 0 && dx != 0 && (g.T[y][x] == OpenDoor || g.T[ny][nx] == OpenDoor) {
		return false
	}
	return !(dy != 0 && dx != 0 && g.Sokoban && g.rock(y, nx) && g.rock(ny, x))
}

func (g *Game) move(dy, dx int) {
	if y, x := g.Y+dy, g.X+dx; InBounds(y, x) && g.T[y][x] == ClosedDoor { // autoopen
		g.Turn++
		switch {
		case dy != 0 && dx != 0:
		case g.Locked[y][x]:
			g.Message = "This door is locked."
		default:
			g.T[y][x] = OpenDoor
			g.Message = "The door opens."
		}
		return
	}
	if !g.canStep(g.Y, g.X, dy, dx) {
		if dy != 0 && dx != 0 && g.Sokoban {
			g.Message = "You cannot pass that way."
		}
		return
	}
	ny, nx := g.Y+dy, g.X+dx
	if g.Mimic[ny][nx] {
		g.reveal(ny, nx, "Wait!  That's a giant mimic!")
		g.Turn++
		return
	}
	if g.Boulder[ny][nx] && !g.push(ny, nx, dy, dx) {
		g.Turn++
		return
	}
	g.Y, g.X = ny, nx
	g.Turn++
	if t := g.T[ny][nx]; t == Pit || t == Hole {
		g.Fell = append(g.Fell, Pos{ny, nx})
	}
}

// push moves the boulder at (by, bx) by (dy, dx) like moverock: into a pit or hole it plugs it.
func (g *Game) push(by, bx, dy, dx int) bool {
	ry, rx := by+dy, bx+dx
	if g.Sokoban && dy != 0 && dx != 0 {
		g.Message = "Boulders won't roll diagonally on this floor.  You try to move the boulder, but in vain."
		return false
	}
	if !InBounds(ry, rx) || g.solid(ry, rx) || g.Boulder[ry][rx] {
		g.Message = "You try to move the boulder, but in vain."
		return false
	}
	if g.Mimic[ry][rx] {
		g.Message = "There's a giant mimic on the other side.  Perhaps that's why you cannot move it."
		return false
	}
	g.Boulder[by][bx] = false
	g.Pushes++
	switch g.T[ry][rx] {
	case Pit:
		g.T[ry][rx] = Floor
		g.Filled++
		g.Message = "The boulder fills a pit."
	case Hole:
		g.T[ry][rx] = Floor
		g.Filled++
		g.Message = "The boulder falls into and plugs a hole in the floor!"
	default:
		g.Boulder[ry][rx] = true
		g.Message = "With great effort you move the boulder."
	}
	return true
}

func (g *Game) reveal(y, x int, msg string) {
	g.Mimic[y][x] = false
	g.Revealed++
	g.Message = msg
	if g.OnReveal != nil {
		g.pending = g.OnReveal
	}
}

// travelDistances: steps from the hero along the cells travel walks (no boulders, no pits or holes).
func (g *Game) travelDistances() I32Grid {
	var dis I32Grid
	for y := range SY {
		for x := range SX {
			dis[y][x] = -1
		}
	}
	dis[g.Y][g.X] = 0
	q := []Pos{{g.Y, g.X}}
	for len(q) > 0 {
		p := q[0]
		q = q[1:]
		for dy := -1; dy <= 1; dy++ {
			for dx := -1; dx <= 1; dx++ {
				ny, nx := p.Y+dy, p.X+dx
				if (dy == 0 && dx == 0) || !g.canStep(p.Y, p.X, dy, dx) || dis[ny][nx] != -1 ||
					g.Boulder[ny][nx] || g.Mimic[ny][nx] || g.T[ny][nx] == Pit || g.T[ny][nx] == Hole {
					continue
				}
				dis[ny][nx] = dis[p.Y][p.X] + 1
				q = append(q, Pos{ny, nx})
			}
		}
	}
	return dis
}

// stepBack is the tile one step closer to the hero on a shortest travel path to p.
func (g *Game) stepBack(dis *I32Grid, p Pos) Pos {
	for dy := -1; dy <= 1; dy++ {
		for dx := -1; dx <= 1; dx++ {
			q := Pos{p.Y + dy, p.X + dx}
			if InBounds(q.Y, q.X) && dis[q.Y][q.X] == dis[p.Y][p.X]-1 && g.canStep(q.Y, q.X, -dy, -dx) {
				return q
			}
		}
	}
	panic("botsim: no way back on the travel path")
}

// Observe writes the game into the agent, like reading an observation and the update phase.
func (g *Game) Observe(a *world.Agent) {
	o := new(Obs)
	for y := range SY {
		for x := range SX {
			o.Glyphs[y][x] = g.Glyph(y, x)
		}
	}
	a.Blstats.Y, a.Blstats.X, a.Blstats.Time = g.Y, g.X, g.Turn
	b := &a.Blstats
	o.Blstats[0], o.Blstats[1], o.Blstats[20] = int64(b.X), int64(b.Y), int64(b.Time)
	o.Blstats[23], o.Blstats[24] = int64(b.DungeonNumber), int64(b.LevelNumber)
	if a.LastObservation == nil {
		a.PreviousObs = o
	} else {
		a.PreviousObs = a.LastObservation
	}
	a.LastObservation, a.Observation = o, o
	a.StepCount++
	a.LastTurn = g.Turn
	a.CursorPos = Pos{g.Y, g.X}
	if g.cursor != nil {
		a.CursorPos = *g.cursor
	}
	a.Message = g.Message
	a.MessageHistory = append(a.MessageHistory, g.Message)
	a.UpdateLevel()
	if !a.InAtomOperation() {
		g.atom = nil
		a.CallAllUpdateFunctions()
	} else {
		g.atom = outerAtom(a)
	}
}

// outerAtom is the frame of the outermost atom operation on the stack (nil: none).
func outerAtom(a *world.Agent) any {
	for _, f := range a.M.Stack {
		if reflect.TypeOf(f).String() == "*sense.atomF" {
			return f
		}
	}
	return nil
}

// GiveItem puts an item into the agent's inventory under letter, parsed from its inventory text like the
// inventory update does ("a +0 long sword"); glyph is the object's glyph.
func GiveItem(a *world.Agent, letter byte, text string, glyph int) *world.Item {
	ii := a.Inventory.Items
	it := a.Inventory.ItemManager.GetItemFromText(text, ObjClassOf(GlyphToObj(glyph)), glyph, nil)
	ii.AllItems = append(ii.AllItems, it)
	ii.AllLetters = append(ii.AllLetters, letter)
	return it
}

// ObjectGlyph is the glyph of the object named name (the first when it has several).
func ObjectGlyph(name string) int { return O_PossibleGlyphsFromObject(O_FromName1(name))[0] }

// NewAgent is a fresh agent standing in the game on level (dungeon, level), with the terminal of the game.
func (g *Game) NewAgent(dungeon, level int) *world.Agent {
	a := world.NewAgent()
	a.Term = Terminal{G: g}
	a.M.OpLimit = 1 << 16
	a.IsUpdatingState = true
	a.Inventory.ItemsBelowMe, a.Inventory.ItemsBelowMeNone = nil, false
	a.Blstats.DungeonNumber, a.Blstats.LevelNumber = dungeon, level
	a.Blstats.Hitpoints, a.Blstats.MaxHitpoints = 20, 20
	g.Observe(a)
	return a
}

// Run plays the routine root to its end, at most maxKeys keys of its own. It returns root's result and the
// exception that ended it (nil if none; *TooManyKeys when it did not end in time).
func (g *Game) Run(a *world.Agent, root Frame, maxKeys int) (ret any, exc any) {
	top := &topF{Body: root}
	exc = Try(func() {
		start := len(g.Keys)
		act, ok := a.M.Start(a, top)
		for ok {
			if len(g.Keys)-start >= maxKeys {
				panic(&TooManyKeys{N: maxKeys})
			}
			g.Play(act)
			act, ok = a.M.Feed(a, nil)
		}
	})
	return top.Ret, exc
}

// String draws the map rows that have anything on them, the hero as '@'.
func (g *Game) String() string {
	var b strings.Builder
	for y := range SY {
		row := make([]byte, SX)
		empty := true
		for x := range SX {
			ch := " |.#+/<>_^{A"[g.T[y][x]]
			switch {
			case y == g.Y && x == g.X:
				ch = '@'
			case g.Boulder[y][x]:
				ch = '0'
			case g.Mimic[y][x]:
				ch = 'm'
			case g.Object[y][x] != 0:
				ch = '?'
			}
			row[x] = ch
			empty = empty && ch == ' '
		}
		if !empty {
			b.WriteString(strings.TrimRight(string(row), " ") + "\n")
		}
	}
	return b.String()
}

// Report says where a routine stopped: the keys played, the frames on the stack (types only), the last
// messages and the map.
func (g *Game) Report(a *world.Agent) string {
	var b strings.Builder
	fmt.Fprintf(&b, "%d keys, turn %d, hero at (%d, %d); frames:", len(g.Keys), g.Turn, g.Y, g.X)
	for _, f := range a.M.Stack {
		fmt.Fprintf(&b, " %T", f)
	}
	h := a.MessageHistory
	fmt.Fprintf(&b, "\nlast messages: %q\n%s", h[max(0, len(h)-4):], g)
	return b.String()
}

// TooManyKeys: the routine did not end within the keys Run allowed.
type TooManyKeys struct{ N int }

func (e *TooManyKeys) Error() string { return fmt.Sprintf("botsim: no end after %d keys", e.N) }

// topF runs the routine and keeps its result.
type topF struct {
	Body Frame
	Ret  any
	PC   int
}

func (s *topF) Resume(c *Ctx) Cmd {
	if s.PC == 0 {
		s.PC = 1
		return engine.Call(s.Body)
	}
	s.Ret = c.Ret
	return engine.Return[*world.Agent](nil)
}

// Terminal plays keys in the game (world.Terminal).
type Terminal struct{ G *Game }

func (t Terminal) Key(x Action) Frame { return &keyF{G: t.G, Action: x} }

func (t Terminal) Keys(x Action, it world.ActIter) Frame { return &keyF{G: t.G, Action: x, It: it} }

func (t Terminal) Dialog(first Action, gen Frame) Frame { panic("botsim: dialogs are not simulated") }

func (t Terminal) RawKey(x Action) Frame { panic("botsim: raw keys are not simulated") }

func (t Terminal) RawText(s string) Frame { panic("botsim: raw text is not simulated") }

// keyF sends a key (and the keys of its iterator), and observes the game after each.
type keyF struct {
	G      *Game
	Action Action
	It     world.ActIter
	PC     int
}

func (s *keyF) Resume(c *Ctx) Cmd {
	a := c.W
	switch s.PC {
	case 0:
		// the atom operation of the last key has ended: its final update runs the watches (sense.Atom)
		if g := s.G; g.atom != nil && outerAtom(a) != g.atom {
			g.atom = nil
			a.CallAllUpdateFunctions()
		}
		s.PC = 1
		return engine.Send[*world.Agent](s.Action)
	case 1:
		s.G.Observe(a)
		if strings.Contains(s.G.Message, "[yn]") { // the terminal answers y, like term's step
			s.PC = 2
			return engine.Call(Terminal{s.G}.Key(Action('y')))
		}
	}
	s.PC = 1
	if p := s.G.pending; p != nil && !a.InAtomOperation() {
		s.G.pending = nil
		panic(p)
	}
	if s.It != nil {
		if next, ok := s.It.Next(a); ok {
			s.Action = next
			return engine.Send[*world.Agent](next)
		}
	}
	return engine.Return[*world.Agent](nil)
}
