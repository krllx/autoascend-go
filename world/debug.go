package world

import (
	"fmt"
	"strings"
)

// DebugString describes where the agent is: the frames on its machine stack and the last messages.
// For tests and crash reports only.
func (a *Agent) DebugString() string {
	var b strings.Builder
	fmt.Fprintf(&b, "step %d turn %d pos (%d,%d) level %v\n", a.StepCount, a.Blstats.Time, a.Blstats.Y, a.Blstats.X,
		LevelKey{a.Blstats.DungeonNumber, a.Blstats.LevelNumber})
	b.WriteString("frames (outer first):\n")
	for _, f := range a.M.Stack {
		fmt.Fprintf(&b, "  %T %+v\n", f, f)
	}
	b.WriteString("messages:\n")
	h := a.MessageHistory
	for _, m := range h[max(0, len(h)-8):] {
		fmt.Fprintf(&b, "  %q\n", m)
	}
	fmt.Fprintf(&b, "message %q popup %q\n", a.Message, a.Popup)
	return b.String()
}
