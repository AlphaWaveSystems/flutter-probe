package record

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"strings"
)

//go:embed viewer/index.html
var viewerTemplate string

// RenderViewer embeds recording metadata in a standalone, offline viewer.
func RenderViewer(manifestJSON, timelineJSON []byte) ([]byte, error) {
	if !json.Valid(manifestJSON) {
		return nil, fmt.Errorf("invalid manifest JSON")
	}
	if !json.Valid(timelineJSON) {
		return nil, fmt.Errorf("invalid timeline JSON")
	}
	escape := strings.NewReplacer("<", `\u003c`, ">", `\u003e`, "&", `\u0026`)
	return []byte(strings.NewReplacer(
		"/*__MANIFEST__*/null", escape.Replace(string(manifestJSON)),
		"/*__TIMELINE__*/null", escape.Replace(string(timelineJSON)),
	).Replace(viewerTemplate)), nil
}
