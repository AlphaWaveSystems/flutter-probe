package scripts

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// A public page that says there is a Docker image ("Official Docker image") must be backed by a workflow that builds and pushes one. For a long time the
// landing page said so while the repository only held docker/Dockerfile, no workflow pushed an image, and no registry showed one.
var dockerImageClaim = regexp.MustCompile(`(?i)\b(?:official\s+)?docker\s+image\b`)

func TestSiteDoesNotClaimADockerImageWithoutAWorkflowThatPublishesOne(t *testing.T) {
	claims := dockerImageClaims(t, "../website/src/pages")
	if len(claims) == 0 {
		return
	}
	if dockerPublishingWorkflow(t, "../.github/workflows") {
		return
	}
	for _, c := range claims {
		t.Errorf("%s says %q, but no workflow in .github/workflows builds and pushes a Docker image: say what exists (a Dockerfile in the repository), or add the workflow", c.file, c.text)
	}
}

type dockerClaim struct{ file, text string }

// dockerImageClaims returns every "Docker image" phrase in the pages under dir.
func dockerImageClaims(t *testing.T, dir string) []dockerClaim {
	t.Helper()
	var out []dockerClaim
	err := filepath.WalkDir(dir, func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() || filepath.Ext(path) != ".astro" {
			return err
		}
		b, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		for _, m := range dockerImageClaim.FindAllString(string(b), -1) {
			out = append(out, dockerClaim{path, m})
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return out
}

// dockerPublishingWorkflow reports whether a workflow under dir builds and pushes an image.
func dockerPublishingWorkflow(t *testing.T, dir string) bool {
	t.Helper()
	files, err := filepath.Glob(filepath.Join(dir, "*.yml"))
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range files {
		b, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		s := string(b)
		if strings.Contains(s, "docker/build-push-action") || strings.Contains(s, "docker push") {
			return true
		}
	}
	return false
}

func TestDockerImageClaimPattern(t *testing.T) {
	for s, want := range map[string]bool{
		"Official Docker image.":        true,
		"a docker image for CI":         true,
		"A Dockerfile for self-hosted":  false,
		"A Docker setup is available":   false,
		"Docker images are not shipped": false,
	} {
		if got := dockerImageClaim.MatchString(s); got != want {
			t.Errorf("%q: matches = %v, want %v", s, got, want)
		}
	}
}
