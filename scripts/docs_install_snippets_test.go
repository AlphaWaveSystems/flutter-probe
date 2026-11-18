package scripts

import (
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"
)

// An install snippet that downloads a binary to one path and then makes a file of the same name executable in another
// directory (`curl -o /usr/local/probe ...` then `chmod +x /usr/local/bin/probe`) fails on the first run: the chmod finds
// no file, or the binary is not on PATH. This test reads every snippet in the docs, the READMEs and the workflows and
// reports a chmod +x whose target has the name of a downloaded file but is not that file, nor where the snippet moved or
// copied it to. Not handled: a `cd` before the download (a relative path then means another directory), and a path with a space in
// it inside quotes is read as one word but is not split further.

const snippetRoot = ".."

var (
	curlOut   = regexp.MustCompile(`curl\b[^\n]*?\s(?:-[A-Za-z]*o|--output)[ =]+("[^"]+"|'[^']+'|[^\s\\]+)`)
	wgetOut   = regexp.MustCompile(`wget\b[^\n]*?\s(?:-[A-Za-z]*O|--output-document)[ =]*("[^"]+"|'[^']+'|[^\s\\]+)`)
	chmodX    = regexp.MustCompile(`chmod\s+(?:-\w+\s+)*(?:[ugoa]*[+=][rwXst]*x[rwXst]*|[0-7]{3,4})\s+((?:"[^"]+"|'[^']+'|[^\s;&|\\]+)(?:\s+(?:"[^"]+"|'[^']+'|[^\s;&|\\]+))*)`)
	curlLine  = regexp.MustCompile(`curl\b[^\n]*`)
	curlFlagO = regexp.MustCompile(`\s-[A-Za-z]*O\b`)
	urlWord   = regexp.MustCompile(`https?://[^\s"'\\]+`)
	// mv, cp or install, then flags (a flag may take a value: install -m 755, cp -t dir), then the source and the destination
	// the -t form names the destination directory first: install -t /usr/local/bin probe
	moveToDir = regexp.MustCompile(`\b(?:mv|cp|install)\s+(?:-\S+\s+)*-t\s+("[^"]+"|'[^']+'|[^\s\\;&|]+)\s+("[^"]+"|'[^']+'|[^\s\\;&|]+)`)
	moveCopy  = regexp.MustCompile(`\b(?:mv|cp|install)\s+(?:-[mtTogS]\s+\S+\s+|-\S+\s+)*("[^"]+"|'[^']+'|[^\s\\;&|]+)\s+("[^"]+"|'[^']+'|[^\s\\;&|]+)`)
)

func unquote(s string) string { return path.Clean(strings.Trim(s, `"'`)) } // ./probe and probe are one file

var stepStart = regexp.MustCompile(`^-\s+[A-Za-z_][\w-]*:`)

var continuation = regexp.MustCompile(`\\[ \t]*\n[ \t]*`)

// joinContinuations puts the lines of a command that ends in a backslash on one line, so a flag on the next line is seen.
func joinContinuations(s string) string { return continuation.ReplaceAllString(s, " ") }

// snippetBlocks splits text into the runs of lines a reader copies together: a fenced code block, or the lines of one
// workflow step. A fence or a new workflow step (a list item that starts with a key: `- name:`, `- uses:`, `- id:`, `- run:`)
// ends a block; a blank line inside one does not.
func snippetBlocks(text string) []string {
	var blocks []string
	var cur []string
	flush := func() {
		if len(cur) > 0 {
			blocks = append(blocks, strings.Join(cur, "\n"))
			cur = nil
		}
	}
	for _, line := range strings.Split(text, "\n") {
		trim := strings.TrimSpace(line)
		if strings.HasPrefix(trim, "```") || stepStart.MatchString(trim) {
			flush()
			continue
		}
		if trim == "" {
			continue // a blank line inside a snippet does not end it
		}
		cur = append(cur, line)
	}
	flush()
	return blocks
}

// chmodMismatches returns, for one snippet, the chmod targets that name a downloaded file from another path.
func chmodMismatches(block string) []string {
	block = joinContinuations(block)
	var downloaded []string
	for _, m := range curlOut.FindAllStringSubmatch(block, -1) {
		downloaded = append(downloaded, unquote(m[1]))
	}
	for _, m := range wgetOut.FindAllStringSubmatch(block, -1) {
		downloaded = append(downloaded, unquote(m[1]))
	}
	for _, line := range curlLine.FindAllString(block, -1) {
		if curlFlagO.MatchString(line) { // -O saves each URL under its own file name
			for _, u := range urlWord.FindAllString(line, -1) {
				downloaded = append(downloaded, path.Base(u))
			}
		}
	}
	if len(downloaded) == 0 {
		return nil
	}
	allowed := map[string]bool{}
	for _, d := range downloaded {
		allowed[d] = true
	}
	for _, m := range moveCopy.FindAllStringSubmatch(block, -1) {
		src, dst := unquote(m[1]), unquote(m[2])
		if allowed[src] {
			allowed[dst] = true
			allowed[path.Join(dst, path.Base(src))] = true // a directory destination
		}
	}
	for _, m := range moveToDir.FindAllStringSubmatch(block, -1) {
		dir, src := unquote(m[1]), unquote(m[2])
		if allowed[src] {
			allowed[path.Join(dir, path.Base(src))] = true
		}
	}
	var bad []string
	for _, m := range chmodX.FindAllStringSubmatch(block, -1) {
		for _, target := range strings.Fields(m[1]) {
			target = unquote(target)
			if allowed[target] {
				continue
			}
			for _, d := range downloaded {
				if path.Base(d) == path.Base(target) {
					bad = append(bad, target+" (downloaded to "+d+")")
					break
				}
			}
		}
	}
	return bad
}

func TestInstallSnippetsChmodWhatTheyDownload(t *testing.T) {
	exts := map[string]bool{".md": true, ".mdx": true, ".yml": true, ".yaml": true}
	scanned := 0
	var problems []string
	err := filepath.WalkDir(snippetRoot, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			switch d.Name() {
			case ".git", ".worktrees", "node_modules", "research":
				return filepath.SkipDir
			}
			return nil
		}
		if !exts[filepath.Ext(p)] || d.Name() == "CHANGELOG.md" {
			return nil
		}
		b, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		scanned++
		for _, block := range snippetBlocks(string(b)) {
			for _, bad := range chmodMismatches(block) {
				problems = append(problems, filepath.ToSlash(strings.TrimPrefix(p, snippetRoot+string(filepath.Separator)))+": chmod +x "+bad)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if scanned < 50 {
		t.Fatalf("scanned only %d files: the walk is not reaching the docs", scanned)
	}
	sort.Strings(problems)
	for _, p := range problems {
		t.Errorf("an install snippet makes a different file executable than the one it downloaded: %s", p)
	}
}

func TestChmodMismatchCases(t *testing.T) {
	for _, tc := range []struct {
		name, block string
		want        int
	}{
		{"download and chmod agree", "sudo curl -fsSL -o /usr/local/bin/probe https://x/probe-linux-amd64\nsudo chmod +x /usr/local/bin/probe", 0},
		{"download to another directory", "sudo curl --fail --location -o /usr/local/probe \\\n  https://x/probe-darwin-arm64\nsudo chmod +x /usr/local/bin/probe", 1},
		{"the same file spelled with ./", "curl -fsSL -o probe https://x/probe\nchmod +x ./probe\n./probe version", 0},
		{"download in the working directory, chmod elsewhere", "curl -fsSL -o probe https://x/probe\nchmod +x /usr/local/bin/probe", 1},
		{"download then move, then chmod the moved file", "curl -Lo probe https://x/probe\nsudo mv probe /usr/local/bin/\nsudo chmod +x /usr/local/bin/probe", 0},
		{"chmod before the move on the downloaded name", "curl -Lo probe https://x/probe\nchmod +x probe\nsudo mv probe /usr/local/bin/", 0},
		{"remote file name with -O", "curl -LO https://x/probe-darwin-arm64\nchmod +x probe-darwin-arm64", 0},
		{"two files, one chmod", "curl -LO https://x/probe-darwin-arm64 https://x/probe-mcp-darwin-arm64\nchmod +x probe-darwin-arm64 probe-mcp-darwin-arm64", 0},
		{"quoted variable path agrees", "curl -fsSL -o \"$HOME/bin/probe\" https://x/probe\nchmod +x \"$HOME/bin/probe\"", 0},
		{"a blank line between download and chmod", "sudo curl -fsSL -o /usr/local/probe https://x/probe\n\nsudo chmod +x /usr/local/bin/probe", 1},
		{"-o on the continuation line", "sudo curl -fsSL \\\n  -o /usr/local/probe \\\n  https://x/probe\nsudo chmod +x /usr/local/bin/probe", 1},
		{"wget -O to another directory", "sudo wget -qO /usr/local/probe https://x/probe\nsudo chmod +x /usr/local/bin/probe", 1},
		{"wget --output-document", "wget --output-document=/opt/probe https://x/probe\nchmod +x /usr/local/bin/probe", 1},
		{"wget and chmod agree", "wget -qO /usr/local/bin/probe https://x/probe\nchmod +x /usr/local/bin/probe", 0},
		{"chmod u+x", "curl -fsSL -o /usr/local/probe https://x/probe\nchmod u+x /usr/local/bin/probe", 1},
		{"chmod 755", "curl -fsSL -o /usr/local/probe https://x/probe\nchmod 755 /usr/local/bin/probe", 1},
		{"chmod with a flag", "curl -fsSL -o /usr/local/probe https://x/probe\nchmod -v +x /usr/local/bin/probe", 1},
		{"chmod ug+x agrees", "curl -fsSL -o /usr/local/bin/probe https://x/probe\nchmod ug+x /usr/local/bin/probe", 0},
		{"install -m 755 moves the file and chmod follows", "curl -fsSL -o probe https://x/probe\nsudo install -m 755 probe /usr/local/bin/probe\nsudo chmod +x /usr/local/bin/probe", 0},
		{"install -t takes a directory", "curl -fsSL -o probe https://x/probe\nsudo install -t /usr/local/bin probe\nsudo chmod +x /usr/local/bin/probe", 0},
		{"a move to a full file path", "curl -fsSL -o probe https://x/probe\nsudo mv probe /usr/local/bin/probe\nsudo chmod +x /usr/local/bin/probe", 0},
		{"a move to another name is not the downloaded file", "curl -fsSL -o probe https://x/probe\nsudo mv probe /usr/local/bin/other\nsudo chmod +x /usr/local/bin/probe", 1},
		{"two files with -O, chmod elsewhere on the second", "curl -LO https://x/probe-darwin-arm64 https://x/probe-mcp-darwin-arm64\nchmod +x /usr/local/bin/probe-mcp-darwin-arm64", 1},
		{"two files with -O, chmod on both in place", "curl -LO https://x/probe-darwin-arm64 https://x/probe-mcp-darwin-arm64\nchmod +x probe-darwin-arm64 probe-mcp-darwin-arm64", 0},
		{"chmod a+rx", "curl -fsSL -o /usr/local/probe https://x/probe\nchmod a+rx /usr/local/bin/probe", 1},
		{"chmod +rx", "curl -fsSL -o /usr/local/probe https://x/probe\nchmod +rx /usr/local/bin/probe", 1},
		{"chmod u=rwx", "curl -fsSL -o /usr/local/probe https://x/probe\nchmod u=rwx /usr/local/bin/probe", 1},
		{"chmod a+r does not make it executable", "curl -fsSL -o /usr/local/probe https://x/probe\nchmod a+r /usr/local/bin/probe", 0},
		{"no download in the snippet", "chmod +x ./scripts/run.sh", 0},
		{"a chmod of an unrelated name", "curl -fsSL -o probe https://x/probe\nchmod +x ./gradlew", 0},
	} {
		if got := len(chmodMismatches(tc.block)); got != tc.want {
			t.Errorf("%s: %d mismatches, want %d (%v)", tc.name, got, tc.want, chmodMismatches(tc.block))
		}
	}
}

// mismatchesInText is what the docs test does for one file: split into snippets, then check each.
func mismatchesInText(text string) int {
	n := 0
	for _, b := range snippetBlocks(text) {
		n += len(chmodMismatches(b))
	}
	return n
}

// The cases that go through snippetBlocks, which the table above does not: where a snippet starts and ends.
func TestSnippetBlocksCases(t *testing.T) {
	const fence = "```"
	for _, tc := range []struct {
		name, text string
		want       int
	}{
		{"a blank line inside a fence does not end the snippet", fence + "bash\nsudo curl -fsSL -o /usr/local/probe https://x/probe\n\nsudo chmod +x /usr/local/bin/probe\n" + fence, 1},
		{"two fences are two snippets", fence + "\ncurl -fsSL -o /usr/local/probe https://x/probe\n" + fence + "\n\n" + fence + "\nchmod +x /usr/local/bin/probe\n" + fence, 0},
		{"two workflow steps with name", "- name: a\n  run: curl -fsSL -o /usr/local/probe https://x/probe\n- name: b\n  run: chmod +x /usr/local/bin/probe\n", 0},
		{"two workflow steps with uses", "- uses: a\n  run: curl -fsSL -o /usr/local/probe https://x/probe\n- uses: b\n  run: chmod +x /usr/local/bin/probe\n", 0},
		{"two workflow steps with id", "- id: a\n  run: curl -fsSL -o /usr/local/probe https://x/probe\n- id: b\n  run: chmod +x /usr/local/bin/probe\n", 0},
		{"two workflow steps with run", "- run: curl -fsSL -o /usr/local/probe https://x/probe\n- run: chmod +x /usr/local/bin/probe\n", 0},
		{"the same step is one snippet", "- name: a\n  run: |\n    curl -fsSL -o /usr/local/probe https://x/probe\n\n    chmod +x /usr/local/bin/probe\n", 1},
	} {
		if got := mismatchesInText(tc.text); got != tc.want {
			t.Errorf("%s: %d mismatches, want %d", tc.name, got, tc.want)
		}
	}
}
