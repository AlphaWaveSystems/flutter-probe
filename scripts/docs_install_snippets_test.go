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
// copied it to.

const snippetRoot = ".."

var (
	curlOut  = regexp.MustCompile(`curl\b[^\n]*?\s(?:-[A-Za-z]*o|--output)[ =]+("[^"]+"|'[^']+'|[^\s\\]+)`)
	curlO    = regexp.MustCompile(`curl\b[^\n]*?\s-[A-Za-z]*O\b[^\n]*?(https?://\S+)`)
	chmodX   = regexp.MustCompile(`chmod\s+(?:\+x|a\+x|755|0755)\s+((?:"[^"]+"|'[^']+'|[^\s;&|\\]+)(?:\s+(?:"[^"]+"|'[^']+'|[^\s;&|\\]+))*)`)
	moveCopy = regexp.MustCompile(`\b(?:mv|cp|install)\s+(?:-\S+\s+)*("[^"]+"|'[^']+'|[^\s\\;&|]+)\s+("[^"]+"|'[^']+'|[^\s\\;&|]+)`)
)

func unquote(s string) string { return path.Clean(strings.Trim(s, `"'`)) } // ./probe and probe are one file

// snippetBlocks splits text into the runs of lines a reader copies together: a fenced code block, or the lines of one
// workflow step. A blank line, a fence or a new list item ends a block.
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
		if trim == "" || strings.HasPrefix(trim, "```") || strings.HasPrefix(trim, "- name:") || strings.HasPrefix(trim, "- uses:") {
			flush()
			continue
		}
		cur = append(cur, line)
	}
	flush()
	return blocks
}

// chmodMismatches returns, for one snippet, the chmod targets that name a downloaded file from another path.
func chmodMismatches(block string) []string {
	var downloaded []string
	for _, m := range curlOut.FindAllStringSubmatch(block, -1) {
		downloaded = append(downloaded, unquote(m[1]))
	}
	for _, m := range curlO.FindAllStringSubmatch(block, -1) {
		downloaded = append(downloaded, path.Base(strings.TrimRight(m[1], `\"'`)))
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
		{"no download in the snippet", "chmod +x ./scripts/run.sh", 0},
		{"a chmod of an unrelated name", "curl -fsSL -o probe https://x/probe\nchmod +x ./gradlew", 0},
	} {
		if got := len(chmodMismatches(tc.block)); got != tc.want {
			t.Errorf("%s: %d mismatches, want %d (%v)", tc.name, got, tc.want, chmodMismatches(tc.block))
		}
	}
}
