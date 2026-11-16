package main

import (
	"strings"

	"github.com/alphawavesystems/flutter-probe/internal/probelink"
)

// What Studio shows of the build of the app under test. The agent reports the Flutter and Dart versions, the channel and the framework revision
// the app was built with (the Flutter tool puts them into the app as compile-time defines); the Go side has already dropped anything that is not
// plain version text. They are facts of that build as the build reports them, not a promise: a fork or a custom embedder can report values that
// are not the real ones, so the line says what the app reports and Studio draws no conclusion from it.

// appBuildText is the one line for the device pane: "App built with Flutter 3.44.8 (stable), Dart 3.12.2 · framework revision 1a2b3c4d". Each part is
// there only when the agent sent it: an old agent, or a build whose tool did not set the values, gives "" and the window shows nothing (never a
// placeholder or a guess). A part is a fact the app reported, so it is joined as it came.
func appBuildText(res probelink.HandshakeResult) string {
	var built []string
	switch {
	case res.FlutterVersion != "" && res.FlutterChannel != "":
		built = append(built, "Flutter "+res.FlutterVersion+" ("+res.FlutterChannel+")")
	case res.FlutterVersion != "":
		built = append(built, "Flutter "+res.FlutterVersion)
	case res.FlutterChannel != "":
		built = append(built, "Flutter channel "+res.FlutterChannel)
	}
	if res.DartVersion != "" {
		built = append(built, "Dart "+res.DartVersion)
	}
	text := ""
	if len(built) > 0 {
		text = "App built with " + strings.Join(built, ", ")
	}
	if res.FrameworkRevision != "" {
		if text != "" {
			text += " · framework revision " + res.FrameworkRevision
		} else {
			text = "Framework revision " + res.FrameworkRevision
		}
	}
	return text
}
