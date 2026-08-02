package devicelab

import (
	"os"
	"testing"
	"time"

	"github.com/devicelab-dev/maestro-runner/pkg/core"
	"github.com/devicelab-dev/maestro-runner/pkg/flow"
)

// Tests for the scrollUntilVisible loop-bounding policy (patch 0005).
//
// Maestro's loop has no scroll-count cap (bounded by the step timeout
// only). The runner keeps its historical default cap of 20 out of the
// box — a genuinely-absent element then fails fast and attributable
// instead of scrolling until the timeout — with a driver-level escape
// hatch (MAESTRO_DEVICELAB_SCROLL_MAX_SCROLLS: 0 = uncapped Maestro
// parity, N = default cap N) and per-step maxScrolls: winning over both.
//
// Non-inertness: TestScrollUntilVisibleDefaultCapIsTwenty fails when
// 0005 is reverted (the loop is then uncapped and overshoots 20). The
// env-override tests are guards for the escape hatch — they pass under
// both policies when uncapped is the default and are not the
// non-inertness vehicle.

func TestDefaultScrollCap(t *testing.T) {
	cases := []struct {
		env  string
		set  bool
		want int
	}{
		{"", false, 20},
		{"0", true, 0}, // uncapped (Maestro parity)
		{"7", true, 7},
		{"50", true, 50},
		{"bogus", true, 20},
		{"-3", true, 20},
	}
	for _, c := range cases {
		if c.set {
			os.Setenv("MAESTRO_DEVICELAB_SCROLL_MAX_SCROLLS", c.env)
		} else {
			os.Unsetenv("MAESTRO_DEVICELAB_SCROLL_MAX_SCROLLS")
		}
		if got := defaultScrollCap(); got != c.want {
			t.Errorf("defaultScrollCap() with env=%q (set=%v) = %d, want %d", c.env, c.set, got, c.want)
		}
	}
	os.Unsetenv("MAESTRO_DEVICELAB_SCROLL_MAX_SCROLLS")
}

// Out of the box (env unset), a never-appearing element stops at exactly
// 20 scrolls — the historical default cap, retained deliberately (see
// patch header for the blast-radius argument). Fails on revert of 0005:
// the loop is then bounded by the timeout only and overshoots 20.
func TestScrollUntilVisibleDefaultCapIsTwenty(t *testing.T) {
	os.Unsetenv("MAESTRO_DEVICELAB_SCROLL_MAX_SCROLLS")
	client := &mockDeviceLabClient{
		sourceFunc: func() (string, error) {
			return `<?xml version="1.0" encoding="UTF-8"?>
<hierarchy rotation="0">
  <android.widget.FrameLayout bounds="[0,0][1080,2400]">
    <android.widget.TextView text="Other" bounds="[100,100][300,150]"/>
  </android.widget.FrameLayout>
</hierarchy>`, nil
		},
	}

	driver := New(client, &core.PlatformInfo{ScreenWidth: 1080, ScreenHeight: 2400}, nil)

	step := &flow.ScrollUntilVisibleStep{
		Element:   flow.Selector{Text: "NonExistent"},
		Direction: "down",
		BaseStep:  flow.BaseStep{TimeoutMs: 30000},
		// MaxScrolls not set — the default cap applies.
	}

	result := driver.scrollUntilVisible(step)

	if result.Success {
		t.Error("Expected failure when element not found")
	}
	if client.scrollCalls != 20 {
		t.Errorf("Expected the default cap of exactly 20 scrolls, got %d", client.scrollCalls)
	}
}

// Escape hatch: MAESTRO_DEVICELAB_SCROLL_MAX_SCROLLS=0 removes the cap —
// the loop is bounded by the step timeout only (Maestro parity).
func TestScrollUntilVisibleUncappedViaEnv(t *testing.T) {
	defer func() {
		scrollRetryDelay = 300 * time.Millisecond
		scrollFindTimeoutMs = 1000
		os.Unsetenv("MAESTRO_DEVICELAB_SCROLL_MAX_SCROLLS")
	}()
	scrollRetryDelay = time.Millisecond
	scrollFindTimeoutMs = 1
	os.Setenv("MAESTRO_DEVICELAB_SCROLL_MAX_SCROLLS", "0")

	client := &mockDeviceLabClient{
		sourceFunc: func() (string, error) {
			return `<?xml version="1.0" encoding="UTF-8"?>
<hierarchy rotation="0">
  <android.widget.FrameLayout bounds="[0,0][1080,2400]">
    <android.widget.TextView text="Other" bounds="[100,100][300,150]"/>
  </android.widget.FrameLayout>
</hierarchy>`, nil
		},
	}

	driver := New(client, &core.PlatformInfo{ScreenWidth: 1080, ScreenHeight: 2400}, nil)

	step := &flow.ScrollUntilVisibleStep{
		Element:   flow.Selector{Text: "NonExistent"},
		Direction: "down",
		BaseStep:  flow.BaseStep{TimeoutMs: 2000},
	}

	result := driver.scrollUntilVisible(step)

	if result.Success {
		t.Error("Expected failure when element not found")
	}
	if client.scrollCalls <= 20 {
		t.Errorf("Expected more than the default 20-scroll cap with the env override, got %d", client.scrollCalls)
	}
}

// The reveal case end to end: the target starts absent from the
// hierarchy (below the modal's fold) and appears after swipes; uncapped
// via the env override, the loop keeps going past the default cap and
// succeeds once the element shows. (Under the default cap of 20 this
// geometry fails at 20 — that is the deliberate out-of-box behaviour.)
func TestScrollUntilVisibleRevealAfterManyScrolls(t *testing.T) {
	defer func() {
		scrollRetryDelay = 300 * time.Millisecond
		scrollFindTimeoutMs = 1000
		os.Unsetenv("MAESTRO_DEVICELAB_SCROLL_MAX_SCROLLS")
	}()
	scrollRetryDelay = time.Millisecond
	scrollFindTimeoutMs = 1
	os.Setenv("MAESTRO_DEVICELAB_SCROLL_MAX_SCROLLS", "0")

	client := &mockDeviceLabClient{}
	// Target enters the hierarchy only after 25 swipes (e.g. a modal's
	// ScrollView revealing a clipped row).
	client.sourceFunc = func() (string, error) {
		if client.scrollCalls >= 25 {
			return scrollFormPage("[44,1339][1036,1471]"), nil
		}
		return `<?xml version="1.0" encoding="UTF-8"?>
<hierarchy rotation="0">
  <android.widget.FrameLayout bounds="[0,0][1080,2400]">
    <android.widget.TextView text="Other" bounds="[100,100][300,150]"/>
  </android.widget.FrameLayout>
</hierarchy>`, nil
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
		t.Fatalf("expected success once the element is revealed, got %v", result.Message)
	}
	if client.scrollCalls != 25 {
		t.Errorf("expected 25 scrolls before reveal, got %d", client.scrollCalls)
	}
}
