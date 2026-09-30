package nle

import (
	"unsafe"

	. "aago/grid"
)

// Obs holds the observation arrays AutoAscend reads.
type Obs struct {
	Glyphs      I16Grid
	Chars       [SY][SX]uint8
	Specials    [SY][SX]uint8
	Blstats     [27]int64
	Message     [256]uint8
	TtyChars    [24][80]uint8
	TtyCursor   [2]uint8
	Misc        [3]int32
	InvStrs     [55][80]uint8
	InvLetters  [55]uint8
	InvOclasses [55]uint8
	InvGlyphs   [55]int16
}

// WireParts views the observation arrays as bytes in the order and layout harness/bot.py sends them
// (numpy tobytes of each array, little-endian; valid on little-endian hosts).
func (o *Obs) WireParts() [12][]byte {
	return [12][]byte{
		asBytes(unsafe.Pointer(&o.Glyphs), int(unsafe.Sizeof(o.Glyphs))),
		asBytes(unsafe.Pointer(&o.Chars), int(unsafe.Sizeof(o.Chars))),
		asBytes(unsafe.Pointer(&o.Specials), int(unsafe.Sizeof(o.Specials))),
		asBytes(unsafe.Pointer(&o.Blstats), int(unsafe.Sizeof(o.Blstats))),
		asBytes(unsafe.Pointer(&o.Message), int(unsafe.Sizeof(o.Message))),
		asBytes(unsafe.Pointer(&o.TtyChars), int(unsafe.Sizeof(o.TtyChars))),
		asBytes(unsafe.Pointer(&o.TtyCursor), int(unsafe.Sizeof(o.TtyCursor))),
		asBytes(unsafe.Pointer(&o.Misc), int(unsafe.Sizeof(o.Misc))),
		asBytes(unsafe.Pointer(&o.InvStrs), int(unsafe.Sizeof(o.InvStrs))),
		asBytes(unsafe.Pointer(&o.InvLetters), int(unsafe.Sizeof(o.InvLetters))),
		asBytes(unsafe.Pointer(&o.InvOclasses), int(unsafe.Sizeof(o.InvOclasses))),
		asBytes(unsafe.Pointer(&o.InvGlyphs), int(unsafe.Sizeof(o.InvGlyphs))),
	}
}

// ObsWireSize is the number of bytes of one observation on the wire.
func ObsWireSize() int {
	var o Obs
	n := 0
	for _, p := range o.WireParts() {
		n += len(p)
	}
	return n
}

// ObsFromWire decodes one observation from its wire bytes (len(b) == ObsWireSize()).
func ObsFromWire(b []byte) *Obs {
	o := new(Obs)
	for _, p := range o.WireParts() {
		b = b[copy(p, b):]
	}
	return o
}

func asBytes(p unsafe.Pointer, n int) []byte { return unsafe.Slice((*byte)(p), n) }
