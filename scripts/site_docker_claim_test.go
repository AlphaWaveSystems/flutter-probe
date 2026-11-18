package scripts

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

// A page, a README or a Dockerfile that says there is a Docker image ("Official Docker image", "container images", an `ghcr.io/...` image to run) must be
// backed by a workflow that builds AND pushes one. For a long time the landing page said so while the repository only held docker/Dockerfile, no workflow
// pushed an image, and the Dockerfile's own header told people to `docker run` an image that does not resolve.
//
// What it reads: README.md, docker/README.md and docker/Dockerfile, the documentation of the repository (docs/, except the research, PRD and evidence
// folders, which describe history) and everything under website/src (pages, documentation pages). What counts as a claim: "Docker image(s)" or "container
// image(s)" (any case), and a `ghcr.io/` reference. What counts as publishing: a workflow step that uses docker/build-push-action with `push` not false (the
// action's default is not to push), or a `docker push` or `docker buildx build ... --push` command in a `run` script that is not a comment. Workflows are read
// as YAML, both *.yml and *.yaml. A phrase that only says what is NOT done ("no Docker image is published") is a claim too: say it without those words.
var dockerImageClaim = regexp.MustCompile(`(?i)\b(?:docker|container)\s+images?\b|\bghcr\.io/`)

// dockerRoots are the places the guard reads: a file, or a folder read for the extensions.
var dockerRoots = []struct {
	path string
	exts []string
	skip []string // folders under path that are not read
}{
	{"README.md", nil, nil},
	{"docker/README.md", nil, nil},
	{"docker/Dockerfile", nil, nil},
	{"docs", []string{".md"}, []string{"research", "prd", "evidence"}},
	{"website/src", []string{".astro", ".md", ".mdx"}, nil},
}

type dockerClaimHit struct{ file, text string }

// dockerClaimsIn returns the claims in text.
func dockerClaimsIn(file, text string) (out []dockerClaimHit) {
	for _, m := range dockerImageClaim.FindAllString(text, -1) {
		out = append(out, dockerClaimHit{file, m})
	}
	return out
}

// dockerClaimsUnder reads dockerRoots below base (the root of the repository) and returns every claim, the number of files it read and what it could not read.
// A root that does not exist is a problem: a renamed or removed path must be fixed here, not skipped.
func dockerClaimsUnder(base string) (out []dockerClaimHit, read int, problems []string) {
	for _, r := range dockerRoots {
		root := filepath.Join(base, r.path)
		st, err := os.Stat(root)
		if err != nil {
			problems = append(problems, r.path+" does not exist: this test reads it, so a renamed or removed path must be fixed here, not skipped")
			continue
		}
		if !st.IsDir() {
			b, err := os.ReadFile(root)
			if err != nil {
				problems = append(problems, err.Error())
				continue
			}
			read++
			out = append(out, dockerClaimsIn(r.path, string(b))...)
			continue
		}
		err = filepath.WalkDir(root, func(p string, d os.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if d.IsDir() {
				for _, sk := range r.skip {
					if p == filepath.Join(root, sk) {
						return filepath.SkipDir
					}
				}
				return nil
			}
			for _, e := range r.exts {
				if filepath.Ext(p) == e {
					b, err := os.ReadFile(p)
					if err != nil {
						return err
					}
					read++
					rel, _ := filepath.Rel(base, p)
					out = append(out, dockerClaimsIn(filepath.ToSlash(rel), string(b))...)
				}
			}
			return nil
		})
		if err != nil {
			problems = append(problems, err.Error())
		}
	}
	return out, read, problems
}

type workflowDoc struct {
	Jobs map[string]struct {
		Steps []struct {
			Uses string         `yaml:"uses"`
			Run  string         `yaml:"run"`
			With map[string]any `yaml:"with"`
		} `yaml:"steps"`
	} `yaml:"jobs"`
}

var (
	dockerPushCmd = regexp.MustCompile(`(?m)^[^#\n]*\bdocker\s+push\b`)
	buildxPush    = regexp.MustCompile(`(?m)^[^#\n]*\bdocker\s+buildx\s+build\b[^\n]*--push\b`)
)

// workflowPublishesImage says whether the workflow text builds and pushes an image.
func workflowPublishesImage(text string) bool {
	var wf workflowDoc
	if err := yaml.Unmarshal([]byte(text), &wf); err != nil {
		return false
	}
	for _, job := range wf.Jobs {
		for _, s := range job.Steps {
			if strings.HasPrefix(s.Uses, "docker/build-push-action") {
				push, ok := s.With["push"]
				if !ok {
					continue // the action does not push by default
				}
				if v, isBool := push.(bool); isBool && !v {
					continue
				}
				if v, isString := push.(string); isString && strings.TrimSpace(strings.ToLower(v)) == "false" {
					continue
				}
				return true
			}
			if dockerPushCmd.MatchString(s.Run) || buildxPush.MatchString(s.Run) {
				return true
			}
		}
	}
	return false
}

// publishingWorkflowsIn returns the workflow files under dir (*.yml and *.yaml) that publish an image, and how many workflow files it read.
func publishingWorkflowsIn(dir string) (publishing []string, read int, err error) {
	var files []string
	for _, ext := range []string{"*.yml", "*.yaml"} {
		m, err := filepath.Glob(filepath.Join(dir, ext))
		if err != nil {
			return nil, 0, err
		}
		files = append(files, m...)
	}
	for _, f := range files {
		b, err := os.ReadFile(f)
		if err != nil {
			return nil, 0, err
		}
		if workflowPublishesImage(string(b)) {
			publishing = append(publishing, filepath.Base(f))
		}
	}
	return publishing, len(files), nil
}

func TestRepositoryDoesNotClaimADockerImageWithoutAWorkflowThatPublishesOne(t *testing.T) {
	claims, read, problems := dockerClaimsUnder("..")
	for _, p := range problems {
		t.Error(p)
	}
	if read < 30 {
		t.Fatalf("read only %d files: the paths are wrong", read)
	}
	publishing, workflows, err := publishingWorkflowsIn("../.github/workflows")
	if err != nil {
		t.Fatal(err)
	}
	if workflows < 5 {
		t.Fatalf("found %d workflow files: the path is wrong", workflows)
	}
	if len(claims) == 0 || len(publishing) > 0 {
		return
	}
	for _, c := range claims {
		t.Errorf("%s says %q, but no workflow in .github/workflows builds and pushes a Docker image: say what exists (a Dockerfile in the repository, built by hand), or add the workflow", c.file, c.text)
	}
}

func TestDockerImageClaimPattern(t *testing.T) {
	for s, want := range map[string]bool{
		"Official Docker image.":                  true,
		"a docker image for CI":                   true,
		"Docker images are not shipped":           true,
		"a container image":                       true,
		"Container Images":                        true,
		"docker run ghcr.io/x/ci:latest":          true,
		"A Dockerfile for self-hosted":            false,
		"A Docker setup is available":             false,
		"Containers cannot run the iOS simulator": false,
		"build an image from the repository":      false,
		"a Docker container":                      false,
	} {
		if got := dockerImageClaim.MatchString(s); got != want {
			t.Errorf("%q: matches = %v, want %v", s, got, want)
		}
	}
}

func TestWorkflowPublishesImage(t *testing.T) {
	wrap := func(step string) string {
		return "name: x\non: push\njobs:\n  j:\n    runs-on: ubuntu-latest\n    steps:\n" + step
	}
	for name, c := range map[string]struct {
		text string
		want bool
	}{
		"build-push-action with push: true":        {wrap("      - uses: docker/build-push-action@v6\n        with:\n          push: true\n"), true},
		"build-push-action with a push expression": {wrap("      - uses: docker/build-push-action@v6\n        with:\n          push: ${{ github.event_name != 'pull_request' }}\n"), true},
		"build-push-action with push: false":       {wrap("      - uses: docker/build-push-action@v6\n        with:\n          push: false\n"), false},
		"build-push-action with push: \"false\"":   {wrap("      - uses: docker/build-push-action@v6\n        with:\n          push: \"false\"\n"), false},
		"build-push-action without push":           {wrap("      - uses: docker/build-push-action@v6\n        with:\n          context: .\n"), false},
		"a docker push command":                    {wrap("      - run: |\n          docker build -t x .\n          docker push x\n"), true},
		"docker buildx build --push":               {wrap("      - run: docker buildx build --push -t x .\n"), true},
		"docker push in a comment":                 {wrap("      - run: |\n          # docker push x\n          echo hi\n"), false},
		"docker push only in a step name":          {wrap("      - name: docker push x\n        run: echo hi\n"), false},
		"docker push only in an env value":         {wrap("      - run: echo hi\n        env:\n          NOTE: docker push x\n"), false},
		"a workflow with no docker at all":         {wrap("      - run: go test ./...\n"), false},
		"text that is not YAML":                    {"::: not yaml :::", false},
	} {
		if got := workflowPublishesImage(c.text); got != c.want {
			t.Errorf("%s: publishes = %v, want %v", name, got, c.want)
		}
	}
}

// Workflows with the .yaml extension are read as well as .yml.
func TestPublishingWorkflowsReadYamlFiles(t *testing.T) {
	dir := t.TempDir()
	for name, text := range map[string]string{
		"a.yml":  "name: a\non: push\njobs:\n  j:\n    steps:\n      - run: echo\n",
		"b.yaml": "name: b\non: push\njobs:\n  j:\n    steps:\n      - run: docker push x\n",
		"c.yml":  "name: c\non: push\njobs:\n  j:\n    steps:\n      - uses: docker/build-push-action@v6\n        with:\n          push: true\n",
	} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(text), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	got, read, err := publishingWorkflowsIn(dir)
	if err != nil || read != 3 {
		t.Fatalf("read %d workflow files (%v), want 3", read, err)
	}
	if strings.Join(got, ",") != "b.yaml,c.yml" && strings.Join(got, ",") != "c.yml,b.yaml" {
		t.Errorf("publishing workflows = %v, want b.yaml and c.yml", got)
	}
}

// The guard reads every root and file kind it names: a claim planted in each is found (and one under docs/research, which describes history, is not), a clean
// tree has none, and a missing root is reported.
func TestDockerClaimsUnderReadsEveryRoot(t *testing.T) {
	write := func(base, rel, text string) {
		p := filepath.Join(base, rel)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(text), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	tree := func(claim string) string {
		base := t.TempDir()
		for _, rel := range []string{"README.md", "docker/README.md", "docker/Dockerfile", "docs/ops/a.md", "website/src/pages/p.astro", "website/src/content/docs/d.md", "website/src/content/docs/e.mdx"} {
			write(base, rel, "text "+claim+" text\n")
		}
		write(base, "docs/research/old.md", "history: "+claim+"\n")
		write(base, "docs/notes.txt", "not markdown: "+claim+"\n")
		return base
	}
	claims, read, problems := dockerClaimsUnder(tree("a Docker image"))
	if len(problems) > 0 {
		t.Fatalf("problems: %v", problems)
	}
	if read != 7 {
		t.Errorf("read %d files, want the 7 planted (not docs/research, not a .txt)", read)
	}
	found := map[string]bool{}
	for _, c := range claims {
		found[c.file] = true
	}
	for _, want := range []string{"README.md", "docker/README.md", "docker/Dockerfile", "docs/ops/a.md", "website/src/pages/p.astro", "website/src/content/docs/d.md", "website/src/content/docs/e.mdx"} {
		if !found[want] {
			t.Errorf("a claim planted in %s was not found: %v", want, found)
		}
	}
	if found["docs/research/old.md"] || found["docs/notes.txt"] {
		t.Errorf("a claim in a skipped folder or a non-markdown file was reported: %v", found)
	}
	if clean, _, p := dockerClaimsUnder(tree("an image")); len(clean) > 0 || len(p) > 0 {
		t.Errorf("a clean tree reported %v %v", clean, p)
	}
	if _, _, p := dockerClaimsUnder(filepath.Join(t.TempDir(), "empty")); len(p) < 5 {
		t.Errorf("a base with no roots reported only %d problem(s), want one per root", len(p))
	}
}
