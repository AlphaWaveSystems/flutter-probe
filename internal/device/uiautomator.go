package device

import (
	"encoding/xml"
	"fmt"
	"image"
	"strings"
)

// uiNode mirrors a <node> element from a `uiautomator dump` XML hierarchy.
// Only the attributes native-UI matching actually needs are captured —
// class, package, and the various boolean state attrs are left unparsed.
type uiNode struct {
	Text       string   `xml:"text,attr"`
	ResourceID string   `xml:"resource-id,attr"`
	Bounds     string   `xml:"bounds,attr"`
	Nodes      []uiNode `xml:"node"`
}

type uiHierarchy struct {
	XMLName xml.Name `xml:"hierarchy"`
	Nodes   []uiNode `xml:"node"`
}

// FindNativeElement parses a uiautomator XML dump and returns the on-screen
// bounds of the first node whose text or resource-id contains query
// (case-insensitive substring match — the same matching semantics probe's
// Flutter text selectors already use, so native and Flutter queries behave
// consistently).
func FindNativeElement(dumpXML, query string) (image.Rectangle, bool, error) {
	var root uiHierarchy
	if err := xml.Unmarshal([]byte(dumpXML), &root); err != nil {
		return image.Rectangle{}, false, fmt.Errorf("parse uiautomator dump: %w", err)
	}

	q := strings.ToLower(query)
	var found *uiNode
	var walk func(n *uiNode)
	walk = func(n *uiNode) {
		if found != nil {
			return
		}
		if strings.Contains(strings.ToLower(n.Text), q) || strings.Contains(strings.ToLower(n.ResourceID), q) {
			found = n
			return
		}
		for i := range n.Nodes {
			walk(&n.Nodes[i])
		}
	}
	for i := range root.Nodes {
		walk(&root.Nodes[i])
	}
	if found == nil {
		return image.Rectangle{}, false, nil
	}

	rect, err := parseBounds(found.Bounds)
	if err != nil {
		return image.Rectangle{}, false, fmt.Errorf("element matching %q: %w", query, err)
	}
	return rect, true, nil
}

// parseBounds parses uiautomator's "[x1,y1][x2,y2]" bounds format.
func parseBounds(s string) (image.Rectangle, error) {
	var x1, y1, x2, y2 int
	if _, err := fmt.Sscanf(s, "[%d,%d][%d,%d]", &x1, &y1, &x2, &y2); err != nil {
		return image.Rectangle{}, fmt.Errorf("parse bounds %q: %w", s, err)
	}
	return image.Rect(x1, y1, x2, y2), nil
}
