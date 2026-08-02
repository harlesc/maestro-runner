package devicelab

import (
	"testing"

	"github.com/devicelab-dev/maestro-runner/pkg/core"
)

// Tie-break tests for the relative-selector candidate choice, pinned to
// Maestro's measured behaviour (see the "Defect: relative selectors" header
// in pagesource.go). Every test in this file fails against the pre-fix
// implementation, which (a) distance-sorted the survivors and (b) returned
// the globally deepest node when no index was given.

// Defect-1 scenario at function level: a shallow, document-first candidate
// (the on-row tab) and a deeper, document-last candidate (the bottom-nav
// label) both survive leftOf. Maestro taps the document-first one; the old
// code returned the deeper one.
func TestSelectByIndexNoIndexHonoursOrder(t *testing.T) {
	tab := &ParsedElement{Text: "Next Up", Bounds: core.Bounds{X: 44, Y: 326, Width: 243, Height: 147}, Depth: 6, Clickable: true}
	nav := &ParsedElement{Text: "Next Up", Bounds: core.Bounds{X: 0, Y: 2127, Width: 243, Height: 147}, Depth: 8, Clickable: true}
	anchor := &ParsedElement{Text: "Today", Bounds: core.Bounds{X: 353, Y: 326, Width: 207, Height: 147}}

	// candidates arrive in hierarchy order (tab first), as FilterBySelector
	// returns them from the flattened page source.
	candidates := FilterLeftOf([]*ParsedElement{tab, nav}, anchor)
	if len(candidates) != 2 {
		t.Fatalf("both candidates should survive leftOf, got %d", len(candidates))
	}
	candidates = SortClickableFirst(candidates)

	got := SelectByIndex(candidates, "")
	if got != tab {
		t.Errorf("SelectByIndex(no index) = %v (depth %d), want the document-first tab; old code picked the deepest (nav)",
			got.Text, got.Depth)
	}
}

// Diagonal discriminator: same-depth candidates where hierarchy order and
// 2-D centre distance to the anchor disagree. Maestro (measured 3/3 on
// device, CLI 2.7.0) taps the document-first candidate, NOT the nearer one.
// The old distance sort would have promoted the nearer one.
func TestSelectByIndexDiagonalPrefersHierarchyOrder(t *testing.T) {
	a := &ParsedElement{Text: "Next Up", Bounds: core.Bounds{X: 50, Y: 349, Width: 243, Height: 147}, Depth: 6, Clickable: true}
	b := &ParsedElement{Text: "Next Up", Bounds: core.Bounds{X: 567, Y: 550, Width: 243, Height: 147}, Depth: 6, Clickable: true}
	anchor := &ParsedElement{Text: "Today", Bounds: core.Bounds{X: 850, Y: 349, Width: 207, Height: 147}}

	candidates := FilterLeftOf([]*ParsedElement{a, b}, anchor)
	if len(candidates) != 2 || candidates[0] != a {
		t.Fatalf("FilterLeftOf must preserve input order [a b], got %d elements", len(candidates))
	}
	candidates = SortClickableFirst(candidates)

	if got := SelectByIndex(candidates, ""); got != a {
		t.Error("SelectByIndex(no index) should return the document-first candidate a, not the X-nearest b")
	}
}

// SortClickableFirst must stay a stable reorder (Maestro's clickableFirst is
// a stable sortByDescending): clickable candidates keep their relative
// hierarchy order, non-clickable ones likewise.
func TestSortClickableFirstIsStable(t *testing.T) {
	c1 := &ParsedElement{Text: "c1", Clickable: true, Bounds: core.Bounds{Y: 100}}
	n1 := &ParsedElement{Text: "n1", Clickable: false, Bounds: core.Bounds{Y: 200}}
	c2 := &ParsedElement{Text: "c2", Clickable: true, Bounds: core.Bounds{Y: 300}}
	n2 := &ParsedElement{Text: "n2", Clickable: false, Bounds: core.Bounds{Y: 400}}

	got := SortClickableFirst([]*ParsedElement{c1, n1, c2, n2})
	want := []*ParsedElement{c1, c2, n1, n2}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("SortClickableFirst order = [%s %s %s %s], want [c1 c2 n1 n2]",
				got[0].Text, got[1].Text, got[2].Text, got[3].Text)
		}
	}
}

// Explicit index: Maestro's Filters.index sorts by INDEX_COMPARATOR
// (bounds.y, then bounds.x) and picks the Nth; negative counts from the
// end; out of range is "element not found" (nil) — the old code silently
// fell back to the first candidate and sorted nothing.
func TestSelectByIndexExplicitIndexSortsByPosition(t *testing.T) {
	lower := &ParsedElement{Text: "lower", Bounds: core.Bounds{X: 10, Y: 900, Width: 50, Height: 50}}
	upperRight := &ParsedElement{Text: "upperRight", Bounds: core.Bounds{X: 500, Y: 100, Width: 50, Height: 50}}
	upperLeft := &ParsedElement{Text: "upperLeft", Bounds: core.Bounds{X: 10, Y: 100, Width: 50, Height: 50}}
	// hierarchy order differs from position order on purpose
	candidates := []*ParsedElement{lower, upperRight, upperLeft}

	if got := SelectByIndex(candidates, "0"); got != upperLeft {
		t.Errorf("index 0 = %q, want upperLeft (position sort y then x)", got.Text)
	}
	if got := SelectByIndex(candidates, "1"); got != upperRight {
		t.Errorf("index 1 = %q, want upperRight", got.Text)
	}
	if got := SelectByIndex(candidates, "2"); got != lower {
		t.Errorf("index 2 = %q, want lower", got.Text)
	}
	if got := SelectByIndex(candidates, "-1"); got != lower {
		t.Errorf("index -1 = %q, want lower (last by position)", got.Text)
	}
	if got := SelectByIndex(candidates, "3"); got != nil {
		t.Errorf("index 3 out of range = %q, want nil (Maestro: element not found)", got.Text)
	}
	if got := SelectByIndex(candidates, "-4"); got != nil {
		t.Errorf("index -4 out of range = %q, want nil", got.Text)
	}
	// unparseable index keeps the historical leniency: first candidate
	if got := SelectByIndex(candidates, "bogus"); got != lower {
		t.Errorf("unparseable index = %q, want first candidate", got.Text)
	}
}

// Maestro applies deepestMatchingElement to the basic filters: a container
// and its child matching the selector leaves only the child; matches on
// independent branches all survive, in hierarchy order.
func TestDeepestMatchingPerBranch(t *testing.T) {
	root := &ParsedElement{Text: "match", Depth: 1}
	mid := &ParsedElement{Text: "match", Depth: 2, Parent: root}
	leaf := &ParsedElement{Text: "match", Depth: 3, Parent: mid}
	otherBranch := &ParsedElement{Text: "match", Depth: 2}

	got := DeepestMatchingPerBranch([]*ParsedElement{root, mid, leaf, otherBranch})
	if len(got) != 2 || got[0] != leaf || got[1] != otherBranch {
		t.Fatalf("expected [leaf otherBranch], got %d elements (first depth %d)",
			len(got), got[0].Depth)
	}

	// no container+child collision: everything survives, order preserved
	a := &ParsedElement{Text: "a", Depth: 4}
	b := &ParsedElement{Text: "b", Depth: 2}
	got = DeepestMatchingPerBranch([]*ParsedElement{a, b})
	if len(got) != 2 || got[0] != a || got[1] != b {
		t.Fatal("independent candidates must all survive in input order")
	}
}
