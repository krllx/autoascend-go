package nle

import (
	"testing"
)

func TestGlyphZeroNotStoneWall(t *testing.T) {
	if G_STONE.Has(0) || G_WALL.Has(0) {
		t.Fatal("glyph 0 in STONE/WALL: translate fill assumption broken")
	}
}
