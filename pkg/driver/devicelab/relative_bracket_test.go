package devicelab

import (
	"testing"

	"github.com/devicelab-dev/maestro-runner/pkg/core"
	"github.com/devicelab-dev/maestro-runner/pkg/flow"
)

// The fixture is the REAL hierarchy from an SRM race card, captured on emulator-5556 on 2026-08-03
// (runId 20260803T065629Z_neds-uat_srm-bet, assets/flow-004/cmd-008-hierarchy.xml). Its important
// property is not the numbers but the SHAPE: the entrant rows are NOT laid out in hierarchy order —
// entrant 1 sits FIRST in the tree and RENDERS BELOW entrants 2 and 3. That is what makes a
// one-sided `below:` ambiguous, and it is why the flows bracket.
//
//	tree idx  row        name y      its "Top 4" y
//	   40     entrant 1   1831        2035   <- renders BELOW entrants 2 and 3
//	   90     entrant 2   1013        1217
//	  142     entrant 3   1424        1628
//	  194     entrant 4   1835        2039
func srmFixture() (cells []*ParsedElement, names map[string]*ParsedElement) {
	mk := func(text string, y, h int) *ParsedElement {
		return &ParsedElement{Text: text, Bounds: core.Bounds{X: 662, Y: y, Width: 84, Height: h},
			Clickable: false, Displayed: true, Enabled: true}
	}
	name := func(text string, y int) *ParsedElement {
		return &ParsedElement{Text: text, Bounds: core.Bounds{X: 149, Y: y, Width: 378, Height: 42},
			Clickable: false, Displayed: true, Enabled: true}
	}
	// Hierarchy order, exactly as the device reported it.
	cells = []*ParsedElement{
		mk("Top 4", 2035, 18), // entrant 1's cell, clipped at the viewport edge
		mk("Top 4", 1217, 39), // entrant 2's
		mk("Top 4", 1628, 39), // entrant 3's
		mk("Top 4", 2039, 14), // entrant 4's, clipped
	}
	names = map[string]*ParsedElement{
		"1": name("1. Race 9 Test Entrant 1", 1831),
		"2": name("2. Race 9 Test Entrant 2", 1013),
		"3": name("3. Race 9 Test Entrant 3", 1424),
		"4": name("4. Race 9 Test Entrant 4", 1835),
	}
	return cells, names
}

// TestBracketedRelativeSelectorPicksOneRow is the regression test for tk 200mrb-rhqc.
//
// Before the fix getRelativeFilter returned ONE constraint (a switch, first match wins), so
// `{below: A, above: B}` applied only `below:` and silently discarded the bracket. On this geometry
// that made BOTH taps resolve to entrant 1's cell: it satisfies `below:` for every anchor and comes
// first in hierarchy order, so first-wins chose it twice — select, then deselect, netting zero
// selections while both steps reported PASS.
func TestBracketedRelativeSelectorPicksOneRow(t *testing.T) {
	cells, names := srmFixture()
	all := append([]*ParsedElement{}, cells...)
	for _, n := range names {
		all = append(all, n)
	}
	d := &Driver{}

	for _, c := range []struct {
		name         string
		lower, upper string // anchor entrant numbers
		wantCellY    int
	}{
		// The two selections select-srm.yaml makes.
		{"entrant 2's own cell", "2", "3", 1217},
		{"entrant 3's own cell", "3", "4", 1628},
	} {
		t.Run(c.name, func(t *testing.T) {
			sel := flow.Selector{
				Text:  "Top 4",
				Below: &flow.Selector{Text: names[c.lower].Text},
				Above: &flow.Selector{Text: names[c.upper].Text},
			}
			got, err := d.narrowByRelativeFilters(sel, cells, all)
			if err != nil {
				t.Fatalf("narrowByRelativeFilters: %v", err)
			}
			if len(got) != 1 {
				ys := []int{}
				for _, g := range got {
					ys = append(ys, g.Bounds.Y)
				}
				t.Fatalf("a bracket must isolate exactly ONE cell, got %d at y=%v", len(got), ys)
			}
			if got[0].Bounds.Y != c.wantCellY {
				t.Errorf("selected the cell at y=%d, want y=%d", got[0].Bounds.Y, c.wantCellY)
			}
		})
	}
}

// TestOneSidedRelativeSelectorIsAmbiguousOnThisGeometry pins the reason the flows must bracket, so
// nobody "simplifies" a bracket back to a single `below:`. This asserts the AMBIGUITY, not a bug:
// with one constraint the same cell wins for two different anchors, which is a property of the
// layout and is true of Maestro too.
func TestOneSidedRelativeSelectorIsAmbiguousOnThisGeometry(t *testing.T) {
	cells, names := srmFixture()
	all := append([]*ParsedElement{}, cells...)
	for _, n := range names {
		all = append(all, n)
	}
	d := &Driver{}

	first := func(anchor string) int {
		sel := flow.Selector{Text: "Top 4", Below: &flow.Selector{Text: names[anchor].Text}}
		got, err := d.narrowByRelativeFilters(sel, cells, all)
		if err != nil || len(got) == 0 {
			t.Fatalf("below:%s -> %v (err %v)", anchor, len(got), err)
		}
		return got[0].Bounds.Y // first-wins downstream
	}
	if first("2") != first("3") {
		t.Skip("the fixture no longer exhibits the ambiguity; the bracket rationale needs re-deriving")
	}
	if first("2") != 2035 {
		t.Errorf("expected the ambiguous winner to be entrant 1's cell at y=2035, got y=%d", first("2"))
	}
}

// TestGetRelativeFiltersReturnsEveryConstraint is the direct guard on the defect: the old switch
// returned one. A selector can legally carry several.
func TestGetRelativeFiltersReturnsEveryConstraint(t *testing.T) {
	a := &flow.Selector{Text: "A"}
	b := &flow.Selector{Text: "B"}
	c := &flow.Selector{Text: "C"}

	if n := len(getRelativeFilters(flow.Selector{Below: a, Above: b})); n != 2 {
		t.Errorf("below+above -> %d constraint(s), want 2 — a dropped bracket selects the wrong element silently", n)
	}
	if n := len(getRelativeFilters(flow.Selector{Below: a, Above: b, LeftOf: c})); n != 3 {
		t.Errorf("below+above+leftOf -> %d constraint(s), want 3", n)
	}
	if n := len(getRelativeFilters(flow.Selector{Text: "x"})); n != 0 {
		t.Errorf("a selector with no relative clause -> %d, want 0", n)
	}
	// The singular helper must still name the FIRST, since a test and three sibling drivers use that
	// shape; it is now defined in terms of the plural one so the two cannot disagree.
	if got, kind := getRelativeFilter(flow.Selector{Below: a, Above: b}); got != a || kind != filterBelow {
		t.Errorf("getRelativeFilter should still return the first constraint, got %v/%v", got, kind)
	}
}

// TestBracketRefusesWhenAnEitherSideAnchorIsMissing — a bracket whose upper bound does not exist must
// FAIL, not silently degrade to the one-sided form. Degrading is what produced the original defect.
func TestBracketRefusesWhenAnEitherSideAnchorIsMissing(t *testing.T) {
	cells, names := srmFixture()
	all := append([]*ParsedElement{}, cells...)
	for _, n := range names {
		all = append(all, n)
	}
	sel := flow.Selector{
		Text:  "Top 4",
		Below: &flow.Selector{Text: names["2"].Text},
		Above: &flow.Selector{Text: "9. Race 9 Test Entrant 9 (does not exist)"},
	}
	if _, err := (&Driver{}).narrowByRelativeFilters(sel, cells, all); err == nil {
		t.Error("a missing bracket anchor must be an error, never a silent fallback to one-sided")
	}
}
