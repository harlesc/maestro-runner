package devicelab

import (
	"strings"
	"testing"

	"github.com/devicelab-dev/maestro-runner/pkg/core"
)

// TestNotTappableErrorNamesWhatItRefused pins the CONTENT of this message, because the message is the
// evidence. The guard refuses a matched node without issuing a driver RPC, so the client log cannot
// name it; and the driver diagnostic log is off whenever flows run concurrently. This error is the
// only channel that reaches junit-report.xml on every run, so "which selector" and "which strategy"
// have to be in it — a bare geometry is unattributable, and for a step inside a runFlow the parent
// report does not record the nested command either.
func TestNotTappableErrorNamesWhatItRefused(t *testing.T) {
	// The real rect from tests/racing/bonus-bets.yaml, 2026-08-03: a laid-out top of 2852 on a 2340px
	// screen with the bottom clamped to its container at 2274 — top>bottom, centre off the display.
	b := core.Bounds{X: 283, Y: 2852, Width: 250, Height: -578}
	err := notTappableError(`id: "betting-keyboard-key-5"`, "resourceIdMatches", ".*key-5.*", b, 1080, 2340)
	if err == nil {
		t.Fatal("want an error")
	}
	msg := err.Error()

	for _, want := range []string{
		"betting-keyboard-key-5", // WHICH element — the whole point
		"resourceIdMatches",      // and by which strategy it matched
		"w=250",
		"h=-578", // the degenerate shape
		"screen=1080x2340",
		"bounds=[283,2852][533,2274]", // the RAW rect: top below the fold, bottom clamped
	} {
		if !strings.Contains(msg, want) {
			t.Errorf("the refusal must contain %q so it can be attributed; got:\n  %s", want, msg)
		}
	}

	// The phrase downstream tooling keys on must survive a rewording of the rest.
	if !strings.Contains(msg, "rect not tappable") {
		t.Errorf("the message must keep the 'rect not tappable' phrase; got:\n  %s", msg)
	}
}
