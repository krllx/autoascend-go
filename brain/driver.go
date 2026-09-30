package brain

import (
	"fmt"
	"os"
	"runtime/debug"

	"aago/engine"
	. "aago/internal/pyrt"
	. "aago/nle"
	"aago/snapshot"
	"aago/term"
	"aago/world"
)

// Driver mirrors arena_adapter.AutoAscendDriver on top of the frame machine: Act answers every observation
// with the agent's next action. Once the agent dies (an error Python would raise, or a loop that never acts)
// it answers ESC until the game ends, as the Python adapter does after its thread dies or times out.
type Driver struct {
	Agent          *world.Agent
	started        bool
	dead           string
	fallbackAction int
}

// OpLimit bounds the frame operations between two actions: a thousand times more than any decision on the
// corpus (at most 64, see MaxOps), low enough to give up on an endless loop in a second or two (seed 1061:
// about 25 us per operation).
const OpLimit = 1 << 16

func NewDriver() *Driver { return &Driver{fallbackAction: ActionToIndex[A_Command_ESC]} }

func (d *Driver) Reset() {
	d.Agent = world.NewAgent()
	d.Agent.Term = term.Terminal{}
	d.Agent.M.OpLimit = OpLimit
	d.started, d.dead = false, ""
}

// Act returns the action index for obs. The first call only starts the agent: AutoAscend sends its first
// key before it looks at any observation.
func (d *Driver) Act(obs *Obs) int {
	if d.Agent == nil || d.dead != "" {
		return d.fallbackAction
	}
	a, ok, err := d.step(obs)
	if err != "" || !ok {
		if err == "" {
			err = "agent finished"
		}
		d.dead = err
		if os.Getenv("AABOT_QUIET") == "" {
			fmt.Fprintf(os.Stderr, "[aabot] agent died: %s\n", err)
		}
		return d.fallbackAction
	}
	idx, found := ActionToIndex[a]
	if !found {
		panic(fmt.Sprintf("invalid action %d", a))
	}
	return idx
}

func (d *Driver) step(obs *Obs) (a Action, ok bool, err string) {
	defer func() {
		if r := recover(); r != nil {
			stack := PanicStack(r)
			if _, hang := r.(*engine.Hang); hang {
				stack = []byte(d.Agent.DebugString()) // the frames that loop
			} else if stack == nil {
				stack = debug.Stack()
			}
			err = fmt.Sprintf("%v\n%s", r, stack)
		}
	}()
	if !d.started {
		d.started = true
		a, ok = d.Agent.M.Start(d.Agent, &mainF{})
		return
	}
	a, ok = d.Agent.M.Feed(d.Agent, obs)
	return
}

func (d *Driver) Close() { d.Agent = nil }

func (d *Driver) Dead() bool { return d.dead != "" }

// Snapshot saves the driver between two actions: the agent with its frame stack, and the driver flags.
func (d *Driver) Snapshot() ([]byte, error) { return snapshot.Save(d) }

// Restore replaces the driver with one saved by Snapshot of the same build (in any process).
func (d *Driver) Restore(data []byte) error {
	var n Driver
	if err := snapshot.Load(data, &n); err != nil {
		return err
	}
	*d = n
	return nil
}
