package devicelab

import (
	"fmt"
	"testing"

	"github.com/devicelab-dev/maestro-runner/pkg/core"
	"github.com/devicelab-dev/maestro-runner/pkg/flow"
)

// Tests for the centerElement implementation of scrollUntilVisible.
// Reference: Maestro v2.7.0 Orchestra.scrollUntilVisible +
// UiElement.isElementNearScreenCenter/getVisiblePercentage. Every
// scrollUntilVisible test here fails against the pre-fix implementation,
// which never read step.CenterElement and stopped at the first
// any-overlap sighting.

// scrollFormPage builds a one-element page source whose "Submit" TextView
// sits at the given bounds.
func scrollFormPage(bounds string) string {
	return fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<hierarchy rotation="0">
  <android.widget.FrameLayout bounds="[0,0][1080,2400]" displayed="true">
    <android.widget.TextView text="Submit" bounds="%s" clickable="true" displayed="true"/>
  </android.widget.FrameLayout>
</hierarchy>`, bounds)
}

func TestVisiblePercentage(t *testing.T) {
	cases := []struct {
		name               string
		b                  core.Bounds
		screenW, screenH   int
		want               float64
	}{
		{"fully on screen", core.Bounds{X: 0, Y: 100, Width: 100, Height: 100}, 1080, 2400, 1.0},
		{"half clipped at bottom", core.Bounds{X: 0, Y: 2350, Width: 100, Height: 100}, 1080, 2400, 0.5},
		{"off screen", core.Bounds{X: 0, Y: 2500, Width: 100, Height: 100}, 1080, 2400, 0.0},
		{"overflow counts as fully visible", core.Bounds{X: -10, Y: -10, Width: 1100, Height: 2420}, 1080, 2400, 1.0},
		{"zero size", core.Bounds{X: 0, Y: 0, Width: 0, Height: 0}, 1080, 2400, 0.0},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := visiblePercentage(c.b, c.screenW, c.screenH); got != c.want {
				t.Errorf("visiblePercentage(%v) = %v, want %v", c.b, got, c.want)
			}
		})
	}
}

// Maestro parity for the near-centre rule, with the scroll-direction
// semantics folded in: "down" (reveal below) accepts centres ABOVE
// screenCentre+screenH/5, "up" the mirror image.
func TestIsElementNearScreenCenter(t *testing.T) {
	// 1080x2400: centre=(540,1200), margin=480 → down: y<1680, up: y>720,
	// right: x<756, left: x>324.
	cases := []struct {
		name      string
		b         core.Bounds
		direction string
		want      bool
	}{
		{"down accepts centred element", core.Bounds{X: 0, Y: 1339, Width: 992, Height: 132}, "down", true},   // cy=1405 — Maestro's measured stop
		{"down rejects element in bottom band", core.Bounds{X: 0, Y: 2215, Width: 992, Height: 132}, "down", false}, // cy=2281 — runner's measured stop
		{"down rejects element near top", core.Bounds{X: 0, Y: 100, Width: 992, Height: 100}, "down", true},   // cy=150 < 1680: in the reveal half
		{"up accepts element low", core.Bounds{X: 0, Y: 2000, Width: 992, Height: 100}, "up", true},           // cy=2050 > 720
		{"up rejects element near top", core.Bounds{X: 0, Y: 100, Width: 992, Height: 100}, "up", false},      // cy=150
		{"down boundary just above margin", core.Bounds{X: 0, Y: 1613, Width: 992, Height: 132}, "down", true},  // cy=1679 < 1680
		{"down boundary just past margin", core.Bounds{X: 0, Y: 1615, Width: 992, Height: 132}, "down", false},  // cy=1681
		{"right", core.Bounds{X: 600, Y: 0, Width: 100, Height: 100}, "right", true},                          // cx=650 < 756
		{"right rejects", core.Bounds{X: 700, Y: 0, Width: 200, Height: 100}, "right", false},               // cx=800 >= 756
		{"left", core.Bounds{X: 300, Y: 0, Width: 100, Height: 100}, "left", true},                            // cx=350 > 324
		{"left rejects", core.Bounds{X: 200, Y: 0, Width: 100, Height: 100}, "left", false},                   // cx=250
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := isElementNearScreenCenter(c.b, c.direction, 1080, 2400); got != c.want {
				t.Errorf("isElementNearScreenCenter(%v, %q) = %v, want %v", c.b, c.direction, got, c.want)
			}
		})
	}
}

// centerElement must keep scrolling while the element is visible but not
// near the centre, and stop once it is. The same starting position without
// centerElement succeeds with zero scrolls (control proving the option now
// has an effect — pre-fix both arms did 0 scrolls).
func TestScrollUntilVisibleCenterElementScrollsToCenter(t *testing.T) {
	client := &mockDeviceLabClient{}
	// Element fully visible but low (cy=2266 > 1680 on 1080x2400) until two
	// scrolls have happened, then centred (cy=1405).
	client.sourceFunc = func() (string, error) {
		if client.scrollCalls >= 2 {
			return scrollFormPage("[44,1339][1036,1471]"), nil
		}
		return scrollFormPage("[44,2200][1036,2332]"), nil
	}

	driver := New(client, &core.PlatformInfo{ScreenWidth: 1080, ScreenHeight: 2400}, nil)
	step := &flow.ScrollUntilVisibleStep{
		Element:       flow.Selector{Text: "Submit"},
		Direction:     "down",
		CenterElement: true,
		BaseStep:      flow.BaseStep{TimeoutMs: 30000},
	}

	result := driver.scrollUntilVisible(step)
	if !result.Success {
		t.Fatalf("expected success, got %v", result.Message)
	}
	if client.scrollCalls != 2 {
		t.Errorf("expected 2 scrolls to centre the element, got %d", client.scrollCalls)
	}

	// Control: same geometry, centerElement off → immediate success, 0 scrolls.
	client2 := &mockDeviceLabClient{}
	client2.sourceFunc = func() (string, error) {
		return scrollFormPage("[44,2200][1036,2332]"), nil
	}
	driver2 := New(client2, &core.PlatformInfo{ScreenWidth: 1080, ScreenHeight: 2400}, nil)
	step2 := &flow.ScrollUntilVisibleStep{
		Element:   flow.Selector{Text: "Submit"},
		Direction: "down",
		BaseStep:  flow.BaseStep{TimeoutMs: 30000},
	}
	result2 := driver2.scrollUntilVisible(step2)
	if !result2.Success {
		t.Fatalf("control: expected success, got %v", result2.Message)
	}
	if client2.scrollCalls != 0 {
		t.Errorf("control: expected 0 scrolls without centerElement, got %d", client2.scrollCalls)
	}
}

// When the content cannot scroll far enough (element stuck off-centre but
// fully visible), Maestro gives up centring after maxRetryCenterCount=4 and
// applies the plain visibility criterion: 5 scrolls, then success.
func TestScrollUntilVisibleCenterElementRetryExhaustion(t *testing.T) {
	client := &mockDeviceLabClient{}
	client.sourceFunc = func() (string, error) {
		return scrollFormPage("[44,2200][1036,2332]"), nil // fully visible, never centres
	}

	driver := New(client, &core.PlatformInfo{ScreenWidth: 1080, ScreenHeight: 2400}, nil)
	step := &flow.ScrollUntilVisibleStep{
		Element:       flow.Selector{Text: "Submit"},
		Direction:     "down",
		CenterElement: true,
		BaseStep:      flow.BaseStep{TimeoutMs: 30000},
	}

	result := driver.scrollUntilVisible(step)
	if !result.Success {
		t.Fatalf("expected success via retry-exhaustion fallback, got %v", result.Message)
	}
	if client.scrollCalls != maxCenterRetries+1 {
		t.Errorf("expected %d scrolls before fallback, got %d", maxCenterRetries+1, client.scrollCalls)
	}
}

// Retry exhaustion with only PARTIAL visibility is a failure in Maestro
// (visibility < visibilityPercentageNormalized), not a silent success.
// Pre-fix this passed immediately on any-overlap.
func TestScrollUntilVisibleCenterElementPartialVisibilityFails(t *testing.T) {
	client := &mockDeviceLabClient{}
	client.sourceFunc = func() (string, error) {
		return scrollFormPage("[44,2350][1036,2482]"), nil // 50px of 132 visible: 0.38
	}

	driver := New(client, &core.PlatformInfo{ScreenWidth: 1080, ScreenHeight: 2400}, nil)
	step := &flow.ScrollUntilVisibleStep{
		Element:       flow.Selector{Text: "Submit"},
		Direction:     "down",
		CenterElement: true,
		MaxScrolls:    8,
		BaseStep:      flow.BaseStep{TimeoutMs: 30000},
	}

	result := driver.scrollUntilVisible(step)
	if result.Success {
		t.Error("expected failure when the element stays partially visible and off-centre")
	}
	if client.scrollCalls != 8 {
		t.Errorf("expected all %d scrolls, got %d", 8, client.scrollCalls)
	}
}

// visibilityNormalized: unset means Maestro's default (100 → fully visible).
func TestVisibilityNormalized(t *testing.T) {
	if got := visibilityNormalized(0); got != 1.0 {
		t.Errorf("visibilityNormalized(0) = %v, want 1.0 (Maestro default)", got)
	}
	if got := visibilityNormalized(50); got != 0.5 {
		t.Errorf("visibilityNormalized(50) = %v, want 0.5", got)
	}
	if got := visibilityNormalized(100); got != 1.0 {
		t.Errorf("visibilityNormalized(100) = %v, want 1.0", got)
	}
}
