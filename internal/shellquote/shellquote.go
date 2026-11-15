// Package shellquote quotes words for the shell of an Android device. `adb shell` joins the words after "shell" into one line and the device's shell (a POSIX-style shell) reads it, so any word that comes from
// a script, a file name or a configuration file must be quoted, or characters such as ; & | $ ( ) ` ' " and a new line are read as shell syntax and run as commands on the device.
package shellquote

import "strings"

// Quote returns s as one single-quoted shell word: a single quote inside is closed, written escaped, and reopened. Every other character, a new line included, is literal inside single quotes. The empty string
// gives two quotes, one empty word.
func Quote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

// Arg is Quote for a word that is usually plain: a word made only of letters, digits and the characters _ @ % + = : , . / - (a package name, a permission, a path) is returned unchanged, anything else, and
// the empty string, is quoted. The unchanged form keeps ordinary command lines readable in logs and error messages.
func Arg(s string) string {
	if s == "" {
		return "''"
	}
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9':
		case strings.ContainsRune("_@%+=:,./-", r):
		default:
			return Quote(s)
		}
	}
	return s
}
