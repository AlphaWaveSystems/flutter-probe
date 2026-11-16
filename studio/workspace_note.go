package main

import (
	"os"
	"path/filepath"

	"github.com/alphawavesystems/flutter-probe/internal/config"
)

// WorkspaceNote is the one line the Workspace pane shows about the workspace's probe.yaml, when something in it will trip a first run: no file,
// a file that cannot be read, or no project.app. Kind is "" when there is nothing to say.
type WorkspaceNote struct {
	Kind string `json:"kind"` // "", "no-file", "unreadable", "no-app"
	Text string `json:"text"`
}

// WorkspaceNote checks the open workspace's probe.yaml with the loader runs use (config.Load). It reads one file in the workspace and writes
// nothing. With no workspace open there is nothing to say.
func (a *App) WorkspaceNote() WorkspaceNote {
	a.mu.Lock()
	ws := a.workspace
	a.mu.Unlock()
	if ws == "" {
		return WorkspaceNote{}
	}
	_, statErr := os.Stat(filepath.Join(ws, "probe.yaml"))
	cfg, loadErr := config.Load(ws)
	appID := ""
	if loadErr == nil && cfg != nil {
		appID = cfg.Project.App
	}
	return workspaceNoteFor(statErr == nil, loadErr, appID, ws)
}

// workspaceNoteFor decides the note. A file that cannot be read comes before a missing app id (the id cannot be known), and the error text is
// cleaned like any engine text: one line, no absolute path.
func workspaceNoteFor(fileExists bool, loadErr error, appID, root string) WorkspaceNote {
	switch {
	case loadErr != nil:
		return WorkspaceNote{Kind: "unreadable", Text: errConfigUnreadable(loadErr, root).Error()}
	case !fileExists:
		return WorkspaceNote{Kind: "no-file", Text: "No probe.yaml in this folder: runs use the defaults (agent port 48686) and no app id. Create probe.yaml with project.app to set the app id."}
	case appID == "":
		return WorkspaceNote{Kind: "no-app", Text: "probe.yaml has no project.app: steps that restart or clear the app (restart the app, open the app, clear app data) will fail until it is set."}
	}
	return WorkspaceNote{}
}
