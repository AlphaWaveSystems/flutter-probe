package record

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// viewStep is a timeline step with its exchanges and widget tree inlined. The viewer cannot read files once it is
// opened from disk, so everything it shows must be in the page itself.
type viewStep struct {
	Step
	Exchanges []json.RawMessage `json:"exchanges,omitempty"`
	Tree      *viewTree         `json:"tree,omitempty"`
}

type viewTree struct {
	Content string `json:"content"`
}

// WriteViewer writes index.html, a standalone page for this bundle. It inlines the manifest, the timeline, and the
// exchanges and widget trees that the timeline names, so the page works from disk with no server.
func (b *Bundle) WriteViewer(manifest Manifest, timeline Timeline) error {
	manifestJSON, err := json.Marshal(manifest)
	if err != nil {
		return err
	}
	steps := make([]viewStep, 0, len(timeline.Steps))
	for _, step := range timeline.Steps {
		vs := viewStep{Step: step}
		for _, id := range step.Exchanges {
			if !bundleID.MatchString(id) {
				return fmt.Errorf("invalid exchange ID %q", id)
			}
			raw, err := os.ReadFile(filepath.Join(b.dir, "http", id+".json"))
			if err != nil {
				return err
			}
			vs.Exchanges = append(vs.Exchanges, raw)
		}
		if step.Tree != "" {
			if !bundleID.MatchString(step.Tree) {
				return fmt.Errorf("invalid tree ID %q", step.Tree)
			}
			text, err := os.ReadFile(filepath.Join(b.dir, "tree", step.Tree+".txt"))
			if err != nil {
				return err
			}
			vs.Tree = &viewTree{Content: string(text)}
		}
		steps = append(steps, vs)
	}
	timelineJSON, err := json.Marshal(struct {
		Steps []viewStep `json:"steps"`
	}{steps})
	if err != nil {
		return err
	}
	page, err := RenderViewer(manifestJSON, timelineJSON)
	if err != nil {
		return err
	}
	return b.writeFile("index.html", page)
}
