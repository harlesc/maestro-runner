package devicelab

import (
	"encoding/xml"
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/devicelab-dev/maestro-runner/pkg/core"
	"github.com/devicelab-dev/maestro-runner/pkg/flow"
)

// ParsedElement represents an element from page source XML.
type ParsedElement struct {
	Text        string
	ResourceID  string
	ContentDesc string
	HintText    string // hint attribute for EditText fields
	ClassName   string
	Bounds      core.Bounds
	Enabled     bool
	Selected    bool
	Focused     bool
	Displayed   bool
	Clickable   bool
	Scrollable  bool
	Children    []*ParsedElement
	Parent      *ParsedElement // parent element for clickable lookup
	Depth       int            // depth in hierarchy (for deepestMatchingElement)
}

// ParsePageSource parses Android UI hierarchy XML into elements.
// Supports both formats:
// - UIAutomator dump: uses class name as element tag (e.g., <android.widget.FrameLayout>)
// - Appium format: uses <node> elements
func ParsePageSource(xmlData string) ([]*ParsedElement, error) {
	// Use a flexible decoder that handles any element names
	decoder := xml.NewDecoder(strings.NewReader(xmlData))

	var elements []*ParsedElement
	foundHierarchy := false
	var parseElement func() (*ParsedElement, error)

	parseElement = func() (*ParsedElement, error) {
		for {
			token, err := decoder.Token()
			if err != nil {
				return nil, err
			}

			switch t := token.(type) {
			case xml.StartElement:
				// Skip the hierarchy element
				if t.Name.Local == "hierarchy" {
					foundHierarchy = true
					continue
				}

				// Parse attributes
				elem := &ParsedElement{
					ClassName: t.Name.Local, // Class name is the element tag
				}

				for _, attr := range t.Attr {
					switch attr.Name.Local {
					case "text":
						elem.Text = attr.Value
					case "resource-id":
						elem.ResourceID = attr.Value
					case "content-desc":
						elem.ContentDesc = attr.Value
					case "hint":
						elem.HintText = attr.Value
					case "class":
						elem.ClassName = attr.Value // Override if class attr exists
					case "bounds":
						elem.Bounds = parseBounds(attr.Value)
					case "enabled":
						elem.Enabled = attr.Value == "true"
					case "selected":
						elem.Selected = attr.Value == "true"
					case "focused":
						elem.Focused = attr.Value == "true"
					case "displayed":
						elem.Displayed = attr.Value != "false"
					case "clickable":
						elem.Clickable = attr.Value == "true"
					case "scrollable":
						elem.Scrollable = attr.Value == "true"
					}
				}

				// Parse children recursively
				for {
					child, err := parseElement()
					if err != nil || child == nil {
						break
					}
					elem.Children = append(elem.Children, child)
				}

				return elem, nil

			case xml.EndElement:
				return nil, nil // End of current element
			}
		}
	}

	// Parse all root-level elements under hierarchy
	var parseErr error
	for {
		elem, err := parseElement()
		if err != nil {
			// io.EOF is expected at end of document
			if err.Error() != "EOF" {
				parseErr = err
			}
			break
		}
		if elem != nil {
			elements = append(elements, flattenElement(elem, 0)...)
		}
	}

	// Return error for invalid XML
	if parseErr != nil && len(elements) == 0 {
		return nil, parseErr
	}

	// Return error if no valid hierarchy found
	if !foundHierarchy {
		return nil, fmt.Errorf("invalid page source: no hierarchy element found")
	}

	return elements, nil
}

// flattenElement flattens a tree of elements into a list, setting depth and parent.
func flattenElement(elem *ParsedElement, depth int) []*ParsedElement {
	elem.Depth = depth
	result := []*ParsedElement{elem}
	for _, child := range elem.Children {
		child.Parent = elem // Set parent reference
		result = append(result, flattenElement(child, depth+1)...)
	}
	return result
}

// parseBounds parses Android bounds string "[x1,y1][x2,y2]" to Bounds.
func parseBounds(s string) core.Bounds {
	// Format: [x1,y1][x2,y2]
	s = strings.ReplaceAll(s, "][", ",")
	s = strings.Trim(s, "[]")
	parts := strings.Split(s, ",")
	if len(parts) != 4 {
		return core.Bounds{}
	}

	x1, _ := strconv.Atoi(parts[0])
	y1, _ := strconv.Atoi(parts[1])
	x2, _ := strconv.Atoi(parts[2])
	y2, _ := strconv.Atoi(parts[3])

	return core.Bounds{
		X:      x1,
		Y:      y1,
		Width:  x2 - x1,
		Height: y2 - y1,
	}
}

// FilterBySelector filters elements by non-relative selector properties.
func FilterBySelector(elements []*ParsedElement, sel flow.Selector) []*ParsedElement {
	var result []*ParsedElement

	for _, elem := range elements {
		if !matchesSelector(elem, sel) {
			continue
		}
		// Skip elements that the accessibility tree marks not visible to
		// the user (displayed="false"). React Navigation routinely leaves
		// off-active-pane content in the tree with displayed=false; if we
		// return those as matches, assertVisible fails with "element exists
		// but is not visible" — what we actually want is for the search to
		// continue past them, matching only what the user can see. Matches
		// Maestro's pass-through behaviour for displayed=false nodes.
		if !elem.Displayed {
			continue
		}
		result = append(result, elem)
	}

	return result
}

func matchesSelector(elem *ParsedElement, sel flow.Selector) bool {
	// Text matching - supports regex patterns and literal contains
	// Checks text, content-desc (accessibility text), and hint text
	if sel.Text != "" {
		if !matchesText(sel.Text, elem.Text, elem.ContentDesc, elem.HintText) {
			return false
		}
	}

	// ID matching (partial, supports regex)
	if sel.ID != "" {
		if !matchesID(sel.ID, elem.ResourceID) {
			return false
		}
	}

	// Size matching with tolerance
	if sel.Width > 0 || sel.Height > 0 {
		tolerance := sel.Tolerance
		if tolerance == 0 {
			tolerance = 5 // default 5px tolerance
		}
		if sel.Width > 0 && !withinTolerance(elem.Bounds.Width, sel.Width, tolerance) {
			return false
		}
		if sel.Height > 0 && !withinTolerance(elem.Bounds.Height, sel.Height, tolerance) {
			return false
		}
	}

	// State filters
	if sel.Enabled != nil && elem.Enabled != *sel.Enabled {
		return false
	}
	if sel.Selected != nil && elem.Selected != *sel.Selected {
		return false
	}
	if sel.Focused != nil && elem.Focused != *sel.Focused {
		return false
	}
	if sel.Checked != nil && elem.Selected != *sel.Checked {
		// checked maps to selected in Android
		return false
	}

	return true
}

// withinTolerance checks if actual is within tolerance of expected.
func withinTolerance(actual, expected, tolerance int) bool {
	diff := actual - expected
	if diff < 0 {
		diff = -diff
	}
	return diff <= tolerance
}

// matchesID checks if an ID pattern matches the given resource ID.
// Always tries regex matching first; falls back to substring contains on compile error.
func matchesID(pattern, id string) bool {
	re, err := regexp.Compile(pattern)
	if err != nil {
		return strings.Contains(id, pattern)
	}
	return re.MatchString(id)
}

// matchesText checks if pattern matches the element's text, content-desc, or hint.
// If pattern looks like a regex (contains metacharacters), use regex matching.
// Otherwise, use case-insensitive contains matching.
// This matches Maestro's behavior: it checks text, hintText, and accessibilityText.
func matchesText(pattern, text, contentDesc, hintText string) bool {
	// Check if pattern looks like a regex
	if looksLikeRegex(pattern) {
		re, err := regexp.Compile("(?i)" + pattern)
		if err != nil {
			// Invalid regex - fall back to literal matching
			return containsIgnoreCase(text, pattern) ||
				containsIgnoreCase(contentDesc, pattern) ||
				containsIgnoreCase(hintText, pattern)
		}

		// Match against text (with newline stripping like Maestro)
		if text != "" {
			strippedText := strings.ReplaceAll(text, "\n", " ")
			if re.MatchString(text) || re.MatchString(strippedText) || pattern == text || pattern == strippedText {
				return true
			}
		}

		// Match against content-desc (accessibility text)
		if contentDesc != "" {
			strippedDesc := strings.ReplaceAll(contentDesc, "\n", " ")
			if re.MatchString(contentDesc) || re.MatchString(strippedDesc) || pattern == contentDesc || pattern == strippedDesc {
				return true
			}
		}

		// Match against hint text
		if hintText != "" {
			strippedHint := strings.ReplaceAll(hintText, "\n", " ")
			if re.MatchString(hintText) || re.MatchString(strippedHint) || pattern == hintText || pattern == strippedHint {
				return true
			}
		}

		return false
	}

	// Literal text - case-insensitive contains
	return containsIgnoreCase(text, pattern) ||
		containsIgnoreCase(contentDesc, pattern) ||
		containsIgnoreCase(hintText, pattern)
}

// containsIgnoreCase checks if s contains substr (case-insensitive).
func containsIgnoreCase(s, substr string) bool {
	return strings.Contains(strings.ToLower(s), strings.ToLower(substr))
}

// Position filter functions.
//
// Defect: relative selectors picked the wrong candidate (green step, wrong
// element tapped). These filters used to SORT the survivors by 1-D distance
// to the anchor (sortByDistance*); SelectByIndex then discarded even that
// ordering by returning the globally deepest node. Measured on device
// (synthetic app, two "Next Up" elements disambiguated by leftOf:"Today",
// 4/4 paired runs): maestro-runner tapped the deep bottom-nav label while
// Maestro CLI 2.7.0 tapped the on-row tab. A diagonal discriminator screen
// (same-depth candidates: one in the anchor's row but far in X, one row
// below but nearer in 2-D centre distance; 3/3 paired runs) showed Maestro
// taps the candidate that comes FIRST in hierarchy order, NOT the nearest:
// Maestro's Filters.relativeTo sort by distance is discarded by
// Filters.intersect, which preserves the basic filter's order
// (deepestMatchingElement over the pre-order aggregate), followed by a
// stable clickableFirst and first-wins (Orchestra.buildFilter, v2.7.0).
//
// Reference behaviour (Maestro): relative filters are pure predicates and
// MUST preserve the input (hierarchy) order of candidates. Do not re-add
// distance sorting here.

// FilterBelow returns elements below the anchor element.
// Input order is preserved (see header above).
func FilterBelow(elements []*ParsedElement, anchor *ParsedElement) []*ParsedElement {
	anchorBottom := anchor.Bounds.Y + anchor.Bounds.Height
	var result []*ParsedElement

	for _, elem := range elements {
		// Element's top must be below anchor's bottom
		if elem.Bounds.Y >= anchorBottom {
			result = append(result, elem)
		}
	}

	return result
}

// FilterAbove returns elements above the anchor element.
// Input order is preserved (see header above).
func FilterAbove(elements []*ParsedElement, anchor *ParsedElement) []*ParsedElement {
	anchorTop := anchor.Bounds.Y
	var result []*ParsedElement

	for _, elem := range elements {
		// Element's bottom must be above anchor's top
		elemBottom := elem.Bounds.Y + elem.Bounds.Height
		if elemBottom <= anchorTop {
			result = append(result, elem)
		}
	}

	return result
}

// FilterLeftOf returns elements left of the anchor element.
// Input order is preserved (see header above).
func FilterLeftOf(elements []*ParsedElement, anchor *ParsedElement) []*ParsedElement {
	anchorLeft := anchor.Bounds.X
	var result []*ParsedElement

	for _, elem := range elements {
		// Element's right must be left of anchor's left
		elemRight := elem.Bounds.X + elem.Bounds.Width
		if elemRight <= anchorLeft {
			result = append(result, elem)
		}
	}

	return result
}

// FilterRightOf returns elements right of the anchor element.
// Input order is preserved (see header above).
func FilterRightOf(elements []*ParsedElement, anchor *ParsedElement) []*ParsedElement {
	anchorRight := anchor.Bounds.X + anchor.Bounds.Width
	var result []*ParsedElement

	for _, elem := range elements {
		// Element's left must be right of anchor's right
		if elem.Bounds.X >= anchorRight {
			result = append(result, elem)
		}
	}

	return result
}

// FilterChildOf returns elements that are children of anchor.
func FilterChildOf(elements []*ParsedElement, anchor *ParsedElement) []*ParsedElement {
	// Element must be fully inside anchor bounds
	var result []*ParsedElement

	for _, elem := range elements {
		if isInside(elem.Bounds, anchor.Bounds) {
			result = append(result, elem)
		}
	}

	return result
}

// FilterContainsChild returns elements that contain anchor as child.
func FilterContainsChild(elements []*ParsedElement, anchor *ParsedElement) []*ParsedElement {
	var result []*ParsedElement

	for _, elem := range elements {
		if isInside(anchor.Bounds, elem.Bounds) {
			result = append(result, elem)
		}
	}

	return result
}

// FilterInsideOf returns elements whose center point is inside anchor bounds.
// Different from ChildOf - uses visual center containment, not full bounds.
func FilterInsideOf(elements []*ParsedElement, anchor *ParsedElement) []*ParsedElement {
	var result []*ParsedElement

	for _, elem := range elements {
		if elem.Bounds.CenterInside(anchor.Bounds) {
			result = append(result, elem)
		}
	}

	return result
}

func isInside(inner, outer core.Bounds) bool {
	return inner.X >= outer.X &&
		inner.Y >= outer.Y &&
		inner.X+inner.Width <= outer.X+outer.Width &&
		inner.Y+inner.Height <= outer.Y+outer.Height
}

// sortByPosition returns a copy of elements sorted by on-screen position:
// top-to-bottom, then left-to-right. This is Maestro's INDEX_COMPARATOR
// (compareBy bounds.y, then bounds.x), used only when a selector carries an
// explicit `index:`. The sort is stable, so position ties keep hierarchy
// order.
func sortByPosition(elements []*ParsedElement) []*ParsedElement {
	sorted := make([]*ParsedElement, len(elements))
	copy(sorted, elements)
	sort.SliceStable(sorted, func(i, j int) bool {
		if sorted[i].Bounds.Y != sorted[j].Bounds.Y {
			return sorted[i].Bounds.Y < sorted[j].Bounds.Y
		}
		return sorted[i].Bounds.X < sorted[j].Bounds.X
	})
	return sorted
}

// FilterContainsDescendants returns elements that contain ALL specified descendants.
// Each descendant selector must match at least one child within the element's bounds.
func FilterContainsDescendants(elements []*ParsedElement, allElements []*ParsedElement, descendants []*flow.Selector) []*ParsedElement {
	var result []*ParsedElement

	for _, elem := range elements {
		if containsAllDescendants(elem, allElements, descendants) {
			result = append(result, elem)
		}
	}

	return result
}

func containsAllDescendants(parent *ParsedElement, allElements []*ParsedElement, descendants []*flow.Selector) bool {
	for _, descSel := range descendants {
		found := false
		for _, elem := range allElements {
			// Check if elem is inside parent and matches selector
			if isInside(elem.Bounds, parent.Bounds) && matchesSelector(elem, *descSel) {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}
	return true
}

// DeepestMatchingElement returns the element with the highest depth (deepest in hierarchy).
//
// Deprecated for candidate selection: Maestro does not pick the globally
// deepest node (see the position-filter header above). Retained for
// compatibility; new code should use DeepestMatchingPerBranch plus
// order-preserving selection.
func DeepestMatchingElement(elements []*ParsedElement) *ParsedElement {
	if len(elements) == 0 {
		return nil
	}

	deepest := elements[0]
	for _, elem := range elements[1:] {
		if elem.Depth > deepest.Depth {
			deepest = elem
		}
	}
	return deepest
}

// DeepestMatchingPerBranch drops every candidate that has a candidate
// descendant, keeping only the deepest match on each branch of the
// hierarchy. Matches Maestro's Filters.deepestMatchingElement applied to
// the basic (non-relative) filters: when a container and its child both
// match the selector, only the child competes. Input (hierarchy) order is
// preserved.
func DeepestMatchingPerBranch(candidates []*ParsedElement) []*ParsedElement {
	if len(candidates) == 0 {
		return candidates
	}

	// Mark every node that is an ancestor of some candidate; candidates so
	// marked have a matching descendant and are dropped.
	ancestorOfCandidate := make(map[*ParsedElement]bool, len(candidates))
	for _, c := range candidates {
		for p := c.Parent; p != nil; p = p.Parent {
			if ancestorOfCandidate[p] {
				break // everything above p is already marked
			}
			ancestorOfCandidate[p] = true
		}
	}

	var result []*ParsedElement
	for _, c := range candidates {
		if !ancestorOfCandidate[c] {
			result = append(result, c)
		}
	}
	return result
}

// SelectByIndex picks an element from candidates using the selector's index.
//
// Defect fix (see the position-filter header for the full story): with no
// explicit index this used to return the globally DEEPEST candidate,
// discarding the ordering established upstream (hierarchy order +
// SortClickableFirst) — that is how a bottom-nav label beat the on-row tab
// the leftOf anchor was meant to select. Reference behaviour (Maestro
// Orchestra.buildFilter + Filters.index, measured on device):
//   - no index: return the FIRST candidate, honouring the established
//     ordering (hierarchy order, stable clickable-first).
//   - explicit index: sort candidates by position (Maestro's
//     INDEX_COMPARATOR: bounds.y, then bounds.x) and pick the Nth; negative
//     counts from the end; out of range yields nil ("element not found").
//
// An unparseable index is tolerated as "no index" (Maestro would fail the
// command; keeping the runner's historical leniency there).
func SelectByIndex(candidates []*ParsedElement, index string) *ParsedElement {
	if len(candidates) == 0 {
		return nil
	}

	if index != "" {
		if i, err := strconv.Atoi(index); err == nil {
			sorted := sortByPosition(candidates)
			if i < 0 {
				i = len(sorted) + i
			}
			if i < 0 || i >= len(sorted) {
				return nil
			}
			return sorted[i]
		}
	}

	return candidates[0]
}

// SortClickableFirst reorders elements to prioritize clickable ones.
// Clickable elements come first, maintaining relative order within each group.
func SortClickableFirst(elements []*ParsedElement) []*ParsedElement {
	var clickable, nonClickable []*ParsedElement

	for _, elem := range elements {
		if elem.Clickable {
			clickable = append(clickable, elem)
		} else {
			nonClickable = append(nonClickable, elem)
		}
	}

	return append(clickable, nonClickable...)
}

// FilterScrollable returns only scrollable elements from the list.
// Used to find scrollable containers for swipe operations.
func FilterScrollable(elements []*ParsedElement) []*ParsedElement {
	var result []*ParsedElement
	for _, elem := range elements {
		if elem.Scrollable && elem.Bounds.Width > 0 && elem.Bounds.Height > 0 {
			result = append(result, elem)
		}
	}
	return result
}

// FindLargestScrollable returns the scrollable element with the largest area.
// Returns nil if no scrollable elements are found.
func FindLargestScrollable(elements []*ParsedElement) *ParsedElement {
	scrollables := FilterScrollable(elements)
	if len(scrollables) == 0 {
		return nil
	}

	largest := scrollables[0]
	largestArea := largest.Bounds.Width * largest.Bounds.Height

	for _, elem := range scrollables[1:] {
		area := elem.Bounds.Width * elem.Bounds.Height
		if area > largestArea {
			largest = elem
			largestArea = area
		}
	}

	return largest
}

// GetClickableElement returns the element to tap on.
// If the element itself is clickable, returns it.
// If not clickable, walks up the parent chain to find the first clickable parent.
// Returns the original element if no clickable parent is found.
// This handles React Native pattern where text nodes aren't clickable but their containers are.
func GetClickableElement(elem *ParsedElement) *ParsedElement {
	if elem == nil {
		return nil
	}

	// If element itself is clickable, use it
	if elem.Clickable {
		return elem
	}

	// Walk up parent chain to find clickable parent
	parent := elem.Parent
	for parent != nil {
		if parent.Clickable {
			return parent
		}
		parent = parent.Parent
	}

	// No clickable parent found - return original element
	return elem
}
