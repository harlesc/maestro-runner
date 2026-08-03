package devicelab

import (
	"strconv"
	"strings"
	"testing"

	"github.com/devicelab-dev/maestro-runner/pkg/core"
)

// swipeTestDriver builds a Driver whose only capability is recording the shell
// commands a swipe issues. 1080x2340 is the pool emulator this defect was
// measured on.
func swipeTestDriver() (*Driver, *mockShell) {
	sh := &mockShell{}
	return &Driver{
		device: sh,
		info:   &core.PlatformInfo{ScreenWidth: 1080, ScreenHeight: 2340},
	}, sh
}

// TestSwipeCoordsHonourAbsolutePixelsAndPercentages asserts on the ARGV handed to
// the shell, never on the step verdict. That distinction is the whole point of the
// test: before this fix the step was already GREEN while injecting the gesture
// off-screen, so a verdict-level assertion could not have caught it.
//
// INCIDENT: tk 200mrb-urvl — `swipe: start: "540,1145"` was fed to
// ParsePercentageCoords unconditionally, becoming 540%/1145% of the screen:
// `input swipe 5832 26793 5832 20943 400`. `adb shell input swipe` exits 0 for
// off-screen coordinates, so eight repeat iterations reported PASS having moved 0px.
func TestSwipeCoordsHonourAbsolutePixelsAndPercentages(t *testing.T) {
	cases := []struct {
		name     string
		start    string
		end      string
		wantArgv string
	}{
		{
			// The exact form from tests/regression/account/user-settings-tote-selection.yaml
			// before it was re-anchored. Absolute pixels must be passed THROUGH.
			name:     "absolute pixels are passed through unscaled",
			start:    "540,1145",
			end:      "540,895",
			wantArgv: "input swipe 540 1145 540 895 400",
		},
		{
			// flows/racing/select-pyoo.yaml — the last absolute-px swipe in the neds
			// suite. Horizontal, and a silent no-op before this fix.
			name:     "absolute horizontal swipe",
			start:    "900,1024",
			end:      "200,1024",
			wantArgv: "input swipe 900 1024 200 1024 400",
		},
		{
			// Percentage geometry must be BYTE-IDENTICAL to the pre-fix behaviour:
			// 45% of 2340 = 1053, 85% = 1989. Three flows in the suite use this form
			// and are green.
			name:     "percentages keep their historical geometry",
			start:    "50%,45%",
			end:      "50%,85%",
			wantArgv: "input swipe 540 1053 540 1989 400",
		},
		{
			name:     "percentages with spaces",
			start:    "50%, 60%",
			end:      "50%, 42%",
			wantArgv: "input swipe 540 1404 540 982 400",
		},
		{
			// Maestro resolves each endpoint independently, so a mixed pair is legal.
			name:     "mixed forms resolve per endpoint",
			start:    "50%,45%",
			end:      "540,895",
			wantArgv: "input swipe 540 1053 540 895 400",
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			d, sh := swipeTestDriver()
			res := d.swipeWithCoordinates(c.start, c.end, 400)
			if res == nil || !res.Success {
				t.Fatalf("swipe %q -> %q failed: %+v", c.start, c.end, res)
			}
			if len(sh.commands) != 1 {
				t.Fatalf("want exactly 1 shell command, got %d: %v", len(sh.commands), sh.commands)
			}
			if sh.commands[0] != c.wantArgv {
				t.Errorf("argv mismatch\n want: %s\n got:  %s", c.wantArgv, sh.commands[0])
			}
		})
	}
}

// TestSwipeCoordsRefuseOffScreenAbsolute is the NEGATIVE half, and it is the more
// important one. An out-of-bounds coordinate used to be indistinguishable from
// success at the step level, because `input swipe` exits 0 wherever you aim it.
// The fix must make it a LOUD error AND must not issue the gesture at all.
func TestSwipeCoordsRefuseOffScreenAbsolute(t *testing.T) {
	cases := []struct {
		name  string
		start string
		end   string
	}{
		// The literal argv the defect produced from "540,1145" on this screen.
		{"the argv the defect produced", "5832,26793", "5832,20943"},
		{"x beyond the display", "2000,500", "100,500"},
		{"y beyond the display", "500,3000", "500,500"},
		{"negative coordinate", "-10,500", "500,500"},
		{"end off-screen, start valid", "540,1145", "540,9999"},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			d, sh := swipeTestDriver()
			res := d.swipeWithCoordinates(c.start, c.end, 400)
			if res != nil && res.Success {
				t.Fatalf("want refusal for %q -> %q, got success: %+v", c.start, c.end, res)
			}
			// A refusal that still swiped would be the same silent no-op wearing a
			// red verdict.
			if len(sh.commands) != 0 {
				t.Errorf("refused swipe must issue NO gesture, got: %v", sh.commands)
			}
		})
	}
}

// TestSwipeCoordsPercentageOutOfRangeIsRefused guards the other direction: a
// percentage above 100 is a flow bug, not a licence to swipe off-screen.
func TestSwipeCoordsPercentageOutOfRangeIsRefused(t *testing.T) {
	d, sh := swipeTestDriver()
	res := d.swipeWithCoordinates("50%,145%", "50%,45%", 400)
	if res != nil && res.Success {
		t.Fatalf("want refusal for an out-of-range percentage, got success: %+v", res)
	}
	if len(sh.commands) != 0 {
		t.Errorf("refused swipe must issue NO gesture, got: %v", sh.commands)
	}
}

// TestSwipeCoordsNeverParsesAbsoluteAsPercentage is the anti-regression assertion
// stated as the property rather than as a case list: for any in-bounds absolute
// pair, the emitted coordinates must equal the authored ones. If someone re-points
// this function at ParsePercentageCoords, every row here moves.
func TestSwipeCoordsNeverParsesAbsoluteAsPercentage(t *testing.T) {
	for _, coord := range []string{"540,1145", "900,1024", "1,1", "1080,2340", "0,0"} {
		d, sh := swipeTestDriver()
		res := d.swipeWithCoordinates(coord, coord, 400)
		if res == nil || !res.Success {
			t.Fatalf("in-bounds absolute pair %q was refused: %+v", coord, res)
		}
		x, y, err := core.ParsePointCoords(coord, 1080, 2340)
		if err != nil {
			t.Fatalf("fixture %q is not a valid absolute pair: %v", coord, err)
		}
		want := []string{
			// start and end are the same point here, so both halves must appear as authored.
			strconv.Itoa(x), strconv.Itoa(y), strconv.Itoa(x), strconv.Itoa(y),
		}
		got := strings.Fields(sh.commands[0])
		if len(got) != 6 {
			t.Fatalf("unexpected argv shape: %q", sh.commands[0])
		}
		for i, w := range want {
			if got[i+2] != w {
				t.Errorf("%q: argv field %d = %s, want %s (full: %s)", coord, i+2, got[i+2], w, sh.commands[0])
			}
		}
	}
}
