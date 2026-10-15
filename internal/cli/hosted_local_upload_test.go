package cli

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/cobra"

	"github.com/alphawavesystems/flutter-probe/internal/config"
	"github.com/alphawavesystems/flutter-probe/internal/runner"
)

// localUploadCmd is a probe test command with the upload flags a local run reads.
func localUploadCmd(dry bool) *cobra.Command {
	c := &cobra.Command{Use: "test"}
	c.Flags().Bool("upload-report", false, "")
	c.Flags().Bool("upload-dry-run", dry, "")
	c.Flags().Bool("upload-local", true, "")
	c.Flags().String("upload-label", "", "")
	return c
}

// localGitRepo makes a repository with one commit on main and the given origin remote, for a test.
func localGitRepo(t *testing.T, origin string) string {
	t.Helper()
	dir := t.TempDir()
	git := func(args ...string) {
		t.Helper()
		full := append([]string{"-C", dir, "-c", "user.name=probe test", "-c", "user.email=test@example.invalid", "-c", "commit.gpgsign=false"}, args...)
		if out, err := exec.Command("git", full...).CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	git("init", "-q", "-b", "main")
	git("remote", "add", "origin", origin)
	if err := os.WriteFile(filepath.Join(dir, "tracked.txt"), []byte("a\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	git("add", "tracked.txt")
	git("commit", "-q", "-m", "init")
	return dir
}

// useTempHome points the user's home (and so ~/.flutterprobe, where the local signer lives) at a temporary directory.
func useTempHome(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	return home
}

func TestUploadFlagsErrorRefusesBothUploads(t *testing.T) {
	both := localUploadCmd(false)
	if err := both.Flags().Set("upload-report", "true"); err != nil {
		t.Fatal(err)
	}
	if err := uploadFlagsError(both); err == nil {
		t.Fatal("--upload-local with --upload-report must be refused")
	}
	local := localUploadCmd(false)
	if err := uploadFlagsError(local); err != nil {
		t.Fatalf("--upload-local alone: %v", err)
	}
}

func TestLocalUploadDryRunPrintsTheContractEnvelope(t *testing.T) {
	home := useTempHome(t)
	repo := localGitRepo(t, "git@github.com:owner/name.git")
	cmd := localUploadCmd(true)
	var status, out bytes.Buffer
	cmd.SetOut(&out)

	localUpload(cmd, &status, &config.Config{}, hostedSampleResults(), runner.RunMetadata{Platform: "ios"}, "simulator", repo)

	body := out.String()
	for _, want := range []string{`"local": {`, `"repository": "owner/name"`, `"branch": "main"`, `"dirty": false`, `"seq":`, "dry run: nothing is sent"} {
		if !strings.Contains(body, want) {
			t.Fatalf("dry-run output lacks %q:\n%s", want, body)
		}
	}
	if strings.Contains(body, `"ci"`) {
		t.Fatal("a local dry run carries CI facts")
	}
	for _, leak := range []string{repo, home} {
		if strings.Contains(body, leak) {
			t.Fatalf("the dry run prints a local path (%s)", leak)
		}
	}
	if !strings.Contains(status.String(), "a dry run reserves nothing") {
		t.Fatalf("status = %q, want the note that a dry run reserves nothing", status.String())
	}
}

func TestLocalUploadOutsideGitWarnsAndStops(t *testing.T) {
	useTempHome(t)
	cmd := localUploadCmd(true)
	var status bytes.Buffer
	localUpload(cmd, &status, &config.Config{}, hostedSampleResults(), runner.RunMetadata{Platform: "ios"}, "simulator", t.TempDir())
	if !strings.Contains(status.String(), "not a git repository") {
		t.Fatalf("status = %q, want a warning that this is not a git repository", status.String())
	}
}

func TestLocalUploadNeedsAGitHubRemote(t *testing.T) {
	useTempHome(t)
	repo := localGitRepo(t, "https://gitlab.com/owner/name.git")
	cmd := localUploadCmd(true)
	var status bytes.Buffer
	localUpload(cmd, &status, &config.Config{}, hostedSampleResults(), runner.RunMetadata{Platform: "ios"}, "simulator", repo)
	if !strings.Contains(status.String(), "not a GitHub repository (local uploads support GitHub remotes only)") {
		t.Fatalf("status = %q, want the GitHub-only refusal", status.String())
	}
}
