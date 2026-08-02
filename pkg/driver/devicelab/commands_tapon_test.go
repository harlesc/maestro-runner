package devicelab

import (
	"strings"
	"testing"

	"github.com/devicelab-dev/maestro-runner/pkg/flow"
)

// Defect 3: the tap must land on the node bearing the text, not on a
// promoted clickable ancestor. tapOnStrategies must try general
// (matched-node) strategies before clickable-only ones — the first
// matching strategy decides where the tap lands, and with .clickable(true)
// the agent promotes the match to its clickable ancestor and clicks the
// ANCESTOR's centre (measured on device: the tap landed on a covered row
// centre while the step reported PASS). Fails against the pre-fix
// clickable-first ordering.
func TestTapOnStrategiesMatchedNodeFirst(t *testing.T) {
	sel := flow.Selector{Text: "Menu Row Two"}
	strategies, err := tapOnStrategies(sel)
	if err != nil {
		t.Fatalf("tapOnStrategies: %v", err)
	}
	if len(strategies) == 0 {
		t.Fatal("expected strategies")
	}

	firstClickable := -1
	firstGeneralText := -1
	for i, s := range strategies {
		if strings.Contains(s.Value, ".clickable(true)") && firstClickable < 0 {
			firstClickable = i
		}
		if strings.Contains(s.Value, "textContains(") && !strings.Contains(s.Value, ".clickable(true)") && firstGeneralText < 0 {
			firstGeneralText = i
		}
	}

	if firstGeneralText < 0 {
		t.Fatal("no general textContains strategy present")
	}
	if firstClickable < 0 {
		t.Fatal("clickable-only strategies should remain as fallback")
	}
	if firstClickable < firstGeneralText {
		t.Errorf("clickable-only strategy at index %d precedes general text strategy at %d — the tap would land on the promoted ancestor's centre",
			firstClickable, firstGeneralText)
	}
}
