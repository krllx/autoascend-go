package world

import (
	"strings"

	"aago/grid"
	. "aago/internal/pyrt"
	. "aago/nle"
	"aago/perception"
)

func (a *Agent) getMessageAndPopup(obs *Obs) (string, []string, bool) {
	message := perception.CleanStripRow(obs.Message[:])
	if strings.HasSuffix(message, "--More--") {
		message = message[:len(message)-len("--More--")]
	}
	popup := []string{}
	// all == " ".join(lines); lines are substrings of it (one allocation for the whole screen)
	var buf [24 * 81]byte
	b := buf[:0]
	var ends [24]int
	for i := range obs.TtyChars {
		if i > 0 {
			b = append(b, ' ')
		}
		b = perception.AppendCleanRow(b, obs.TtyChars[i][:])
		ends[i] = len(b)
	}
	if perception.NoMarker(b) {
		return message, popup, true // what FindMarker below finds then
	}
	all := string(b)
	lines := make([]string, 24)
	start := 0
	for i := range lines {
		lines[i] = all[start:ends[i]]
		start = ends[i] + 1
	}
	markerPos, found, _ := perception.FindMarker(all, lines)
	if !found {
		return message, popup, true
	}
	pref := ""
	messageLinesCount := 0
	if message != "" {
		matched := false
		for i, line := range lines[:markerPos.Y+1] {
			if i == markerPos.Y {
				line = line[:markerPos.X]
			}
			messageLinesCount++
			pref += PyStrip(line)
			if perception.AlnumOnly(pref) == perception.AlnumOnly(message) {
				matched = true
				break
			}
		}
		if !matched {
			if markerPos.Y == 0 {
				elems1 := PySplit(message)
				elems2 := PySplit(pref)
				ok := len(elems1) < len(elems2)
				if ok {
					tail := elems2[len(elems2)-len(elems1):]
					for k := range elems1 {
						if tail[k] != elems1[k] {
							ok = false
						}
					}
				}
				Assert(ok, elems1, elems2)
				return pref, popup, false
			}
			if wrappedMessage(message, lines, markerPos) {
				return message, popup, false
			}
			Raise("ValueError", "Message doesn't match the screen")
		}
	}
	var cut []string
	if messageLinesCount < markerPos.Y {
		cut = append(cut, lines[messageLinesCount:markerPos.Y]...)
	}
	cut = append(cut, lines[markerPos.Y][:markerPos.X])
	for _, l := range cut {
		if markerPos.X < len(l) {
			l = PyStrip(l[markerPos.X:])
		} else {
			l = ""
		}
		if l != "" {
			popup = append(popup, l)
		}
	}
	return message, popup, false
}

// wrappedMessage: the --More-- belongs to a message longer than the top line. Not in the Python bot: tty wraps
// such a message over the map and writes only its own characters there (seed 3066: the second line kept the map
// but for its last column), so the lines never spell the message and the bot failed. When the first line begins
// the message and the text before the marker ends it, the screen is the message alone, without a popup.
func wrappedMessage(message string, lines []string, markerPos grid.Pos) bool {
	m := perception.AlnumOnly(message)
	first := perception.AlnumOnly(lines[0])
	last := perception.AlnumOnly(lines[markerPos.Y][:markerPos.X])
	return first != "" && last != "" && strings.HasPrefix(m, first) && strings.HasSuffix(m, last)
}

func (a *Agent) UpdateMessageAndPopup(obs *Obs) bool {
	messagePrefix := ""
	var popupPrefix []string
	if a.IsReadingMessageOrPopup {
		messagePrefix = a.Message
		if a.Message != "" {
			messagePrefix += " "
		}
		popupPrefix = a.Popup
	}
	var done bool
	a.SingleMessage, a.SinglePopup, done = a.getMessageAndPopup(obs)
	a.SingleMessage = PyStrip(a.SingleMessage)
	for i := range a.SinglePopup {
		a.SinglePopup[i] = PyStrip(a.SinglePopup[i])
	}
	a.Message = messagePrefix + a.SingleMessage
	a.Popup = append(append([]string{}, popupPrefix...), a.SinglePopup...)
	return done
}

type sliceIter struct {
	acts []Action
	i    int
}

func (s *sliceIter) Next(*Agent) (Action, bool) {
	if s.i >= len(s.acts) {
		return 0, false
	}
	s.i++
	return s.acts[s.i-1], true
}

// IterActs mirrors iter([...]) of actions.
func IterActs(acts ...Action) ActIter { return &sliceIter{acts: acts} }

type strIter struct {
	s string
	i int
}

func (s *strIter) Next(*Agent) (Action, bool) {
	if s.i >= len(s.s) {
		return 0, false
	}
	s.i++
	return Ch(s.s[s.i-1]), true
}

// IterStr mirrors iter('text') (each char is converted like step('c') when consumed).
func IterStr(text string) ActIter { return &strIter{s: text} }
