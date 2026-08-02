package devicelab

import (
	"testing"
)

// Tests for defect G: scrollUntilVisible's per-iteration gesture must use
// Maestro's swipeFromCenter geometry — start at the EXACT screen centre,
// end 10% from the edge toward the reveal direction (0.4 of the screen
// dimension), duration from the speed mapping (AndroidDriver.swipe(
// elementPoint, direction, durationMs) + speedToDuration, v2.7.0).
//
// Why the start point is load-bearing: a swipe's touch-DOWN decides which
// scrollable (if any) the gesture engages. The previous geometry (0.3 of
// the dimension, starting 0.15H below centre) missed scrollable
// containers occupying only the screen's middle band — e.g. a modal
// dialog's ScrollView — so a target below the modal's fold (pruned from
// the view hierarchy) was never revealed and the step burned its scroll
// budget while Maestro CLI passed on the same screen.
//
// Non-inertness: against the pre-fix implementation (performScroll(
// direction, w, h, engine, 0.3) + hardcoded scrollDurationMs=300, no
// maestroSwipeEndpoints/speedToDuration) these tests fail to compile or
// fail their assertions; and the burn reproduced on device reappears.

func TestMaestroSwipeEndpoints(t *testing.T) {
	// 1080x2340: centre = (540,1170).
	cases := []struct {
		direction      string
		x1, y1, x2, y2 int
	}{
		{"down", 540, 1170, 540, 234},   // reveal below: swipe UP to 0.1H
		{"up", 540, 1170, 540, 2106},    // reveal above: swipe DOWN to 0.9H
		{"left", 540, 1170, 972, 1170},  // reveal left: swipe RIGHT to 0.9W
		{"right", 540, 1170, 108, 1170}, // reveal right: swipe LEFT to 0.1W
	}
	for _, c := range cases {
		t.Run(c.direction, func(t *testing.T) {
			x1, y1, x2, y2 := maestroSwipeEndpoints(c.direction, 1080, 2340)
			if x1 != c.x1 || y1 != c.y1 || x2 != c.x2 || y2 != c.y2 {
				t.Errorf("maestroSwipeEndpoints(%q) = (%d,%d,%d,%d), want (%d,%d,%d,%d)",
					c.direction, x1, y1, x2, y2, c.x1, c.y1, c.x2, c.y2)
			}
		})
	}
}

// The swipe must START at the exact screen centre: this is the property
// that lets the gesture engage a modal's ScrollView whose viewport covers
// the middle band but ends above centre+0.15H (the former start point).
func TestMaestroSwipeStartsAtExactCenter(t *testing.T) {
	for _, direction := range []string{"down", "up", "left", "right"} {
		x1, y1, _, _ := maestroSwipeEndpoints(direction, 1080, 2340)
		if x1 != 540 || y1 != 1170 {
			t.Errorf("direction %q: swipe starts at (%d,%d), want exact centre (540,1170)", direction, x1, y1)
		}
	}
}

// speedToDuration mirrors Maestro's speedToDuration: 1000*(100-speed)/100+1.
func TestSpeedToDuration(t *testing.T) {
	cases := []struct {
		speed int
		want  int
	}{
		{0, 601},  // unset → Maestro's default speed 40
		{40, 601}, // Maestro default
		{100, 1},  // fastest
		{80, 201},
		{150, 1}, // clamped
	}
	for _, c := range cases {
		if got := speedToDuration(c.speed); got != c.want {
			t.Errorf("speedToDuration(%d) = %d, want %d", c.speed, got, c.want)
		}
	}
}

// The element reveal case end to end is covered by
// TestScrollUntilVisibleRevealAfterManyScrolls in
// commands_scrollcap_test.go (needs the uncapped mode from patch 0005).
