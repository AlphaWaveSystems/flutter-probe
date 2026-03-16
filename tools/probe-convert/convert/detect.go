package convert

import (
	"bytes"
	"path/filepath"
	"strings"
)

// DetectFormat guesses the source format from the file extension and content.
func DetectFormat(path string, content []byte) (Format, error) {
	ext := strings.ToLower(filepath.Ext(path))

	switch ext {
	case ".feature":
		return FormatGherkin, nil
	case ".robot":
		return FormatRobot, nil
	case ".yaml", ".yml":
		if isMaestroYAML(content) {
			return FormatMaestro, nil
		}
		return "", &DetectError{Path: path, Reason: "YAML file does not look like a Maestro flow (missing appId/tapOn/launchApp markers)"}
	case ".js", ".ts":
		if isDetoxJS(content) {
			return FormatDetox, nil
		}
		return FormatAppium, nil
	case ".py":
		return FormatAppium, nil
	case ".java", ".kt":
		return FormatAppium, nil
	}

	return "", &DetectError{Path: path, Reason: "unrecognized file extension: " + ext}
}

// DetectError is returned when format auto-detection fails.
type DetectError struct {
	Path   string
	Reason string
}

func (e *DetectError) Error() string {
	return "detect: " + e.Path + ": " + e.Reason
}

func isMaestroYAML(content []byte) bool {
	markers := []string{"appId", "tapOn", "launchApp", "assertVisible", "inputText", "runFlow"}
	for _, m := range markers {
		if bytes.Contains(content, []byte(m)) {
			return true
		}
	}
	return false
}

func isDetoxJS(content []byte) bool {
	return bytes.Contains(content, []byte("element(by.")) ||
		bytes.Contains(content, []byte("device.launchApp")) ||
		bytes.Contains(content, []byte("device.reloadReactNative"))
}
