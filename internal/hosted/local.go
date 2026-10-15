package hosted

import (
	"bytes"
	"errors"
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
)

// A local run is one the user asked to upload with --upload-local. Its envelope carries a local block (git facts)
// instead of the CI block, and the server keeps it as unverified evidence that never counts toward a pull request's
// verdict. The field names and rules are the server contract (docs/PROBE-CLOUD-API.md, local runs). The server
// rejects any key inside local that the contract does not list, so LocalInfo has exactly those keys.

const (
	// MaxLocalPayloadBytes is the body limit for a local upload. The server answers 413 above it.
	MaxLocalPayloadBytes = 256 * 1024
	// MaxLocalResults is the result limit for a local upload. The server answers 422 too_many_results above it.
	MaxLocalResults = 2000
	// maxSeq is 2^53-1, the largest integer a JSON number carries exactly. The server takes no larger sequence number.
	maxSeq = 1<<53 - 1
)

var (
	// githubOwner is a GitHub account or organisation name: letters and digits, single hyphens, none at either end.
	githubOwner = regexp.MustCompile(`^[A-Za-z0-9](?:-?[A-Za-z0-9])*$`)
	// githubRepo is a repository name as the server accepts it.
	githubRepo = regexp.MustCompile(`^[A-Za-z0-9._-]{1,100}$`)
	// repositoryName is the envelope form of a repository: owner/name in GitHub's shape.
	repositoryName = regexp.MustCompile(`^[A-Za-z0-9](?:-?[A-Za-z0-9])*/[A-Za-z0-9._-]{1,100}$`)
	// signerID is the installation's signer: 128 random bits as 32 lower-case hex characters.
	signerID = regexp.MustCompile(`^[0-9a-f]{32}$`)
	// scpLike is the scp form of a git remote, for example git@github.com:owner/name.git.
	scpLike = regexp.MustCompile(`^(?:[^@/]+@)?([^:/]+):(.+)$`)
)

// errNotGitHub is the refusal for a remote the server cannot place. The server cannot tell hosts apart, so a remote on
// any other host is never sent.
var errNotGitHub = errors.New("not a GitHub repository (local uploads support GitHub remotes only)")

// LocalInfo is the local block of the envelope. Dirty says whether the working tree differs from HEAD, untracked files
// included. Tree is the git tree of the state that was tested: HEAD^{tree} for a clean tree, and for a dirty one the
// tree of the working state (tracked and untracked, non-ignored files).
type LocalInfo struct {
	Repository string `json:"repository"`
	Branch     string `json:"branch"`
	CommitSHA  string `json:"commit_sha"`
	Dirty      bool   `json:"dirty"`
	Tree       string `json:"tree"`
	Signer     string `json:"signer"`
	Seq        int64  `json:"seq"`
}

// RepositoryFromRemote turns a git remote URL into owner/name for a GitHub repository. Credentials, the host, the port
// and a trailing .git are dropped. A remote on another host, a path that is not exactly owner/name (a group path, for
// example) and a local path are refused.
func RepositoryFromRemote(remote string) (string, error) {
	raw := strings.TrimSpace(remote)
	if raw == "" {
		return "", errors.New("this repository has no origin remote, so local uploads cannot name it")
	}
	var host, path string
	switch {
	case strings.Contains(raw, "://"):
		u, err := url.Parse(raw)
		if err != nil {
			return "", errors.New("the origin remote is not a valid URL")
		}
		switch u.Scheme {
		case "https", "ssh", "git+ssh", "ssh+git":
		default:
			return "", errNotGitHub
		}
		host, path = u.Hostname(), u.Path
	case strings.Contains(raw, `\`):
		return "", errNotGitHub
	default:
		m := scpLike.FindStringSubmatch(raw)
		if m == nil {
			return "", errNotGitHub
		}
		host, path = m[1], m[2]
	}
	if !strings.EqualFold(host, "github.com") {
		return "", errNotGitHub
	}
	parts := strings.Split(strings.TrimSuffix(strings.Trim(path, "/"), ".git"), "/")
	if len(parts) != 2 || !githubOwner.MatchString(parts[0]) || len(parts[0]) > 39 ||
		!githubRepo.MatchString(parts[1]) || parts[1] == "." || parts[1] == ".." {
		return "", errors.New("the origin remote is not a GitHub owner/name repository (group paths and other shapes are refused)")
	}
	return parts[0] + "/" + parts[1], nil
}

// GitFacts are the git facts a local run sends.
type GitFacts struct {
	Repository string
	Branch     string
	CommitSHA  string
	Dirty      bool
	Tree       string
}

// ReadGitFacts reads the facts of the repository that contains dir. It refuses rather than return part of them: no
// repository, no commit, no origin remote, a remote that is not on GitHub, a SHA-256 repository and unresolved merge
// conflicts are all errors, and nothing is sent for them. It never writes to the repository's index.
func ReadGitFacts(dir string) (GitFacts, error) {
	var f GitFacts
	if _, err := gitOutput(dir, nil, "rev-parse", "--is-inside-work-tree"); err != nil {
		return f, errors.New("this is not a git repository (or git is not installed); local uploads need one")
	}
	sha, err := gitOutput(dir, nil, "rev-parse", "--verify", "-q", "HEAD^{commit}")
	if err != nil || sha == "" {
		return f, errors.New("this repository has no commit yet; local uploads need one")
	}
	if len(sha) == 64 {
		return f, errors.New("SHA-256 repositories are not supported for local uploads (the server requires 40-character ids)")
	}
	if !sha40.MatchString(sha) {
		return f, errors.New("git returned an unexpected commit id")
	}
	f.CommitSHA = sha
	remote, err := gitOutput(dir, nil, "remote", "get-url", "origin")
	if err != nil {
		return f, errors.New("this repository has no origin remote, so local uploads cannot name it")
	}
	if f.Repository, err = RepositoryFromRemote(remote); err != nil {
		return f, err
	}
	// A detached HEAD has no branch name; the contract sends the literal HEAD in that case.
	f.Branch = "HEAD"
	if ref, rerr := gitOutput(dir, nil, "symbolic-ref", "-q", "HEAD"); rerr == nil {
		f.Branch = strings.TrimPrefix(ref, "refs/heads/")
	}
	if !validRefName(f.Branch) {
		return f, errors.New("the branch name is not valid for an upload (1 to 255 characters, no control characters)")
	}
	status, err := gitOutput(dir, nil, "--no-optional-locks", "status", "--porcelain")
	if err != nil {
		return f, fmt.Errorf("git status failed: %w", err)
	}
	f.Dirty = status != ""
	if f.Dirty {
		f.Tree, err = workingTree(dir)
	} else {
		f.Tree, err = gitOutput(dir, nil, "rev-parse", "--verify", "-q", "HEAD^{tree}")
	}
	if err != nil || !sha40.MatchString(f.Tree) {
		return f, fmt.Errorf("the tree hash could not be read: %v", err)
	}
	return f, nil
}

// workingTree is the tree id of the working state: tracked files and the untracked files git does not ignore, named the
// way `git write-tree` names them. It is built in a temporary index, so the user's index is never touched. It is the
// only way to include untracked files: `git stash create` does not take them.
func workingTree(dir string) (string, error) {
	tmp, err := os.MkdirTemp("", "probe-local-index-")
	if err != nil {
		return "", err
	}
	defer os.RemoveAll(tmp)
	env := []string{"GIT_INDEX_FILE=" + filepath.Join(tmp, "index")}
	if _, err := gitOutput(dir, env, "add", "-A"); err != nil {
		return "", err
	}
	return gitOutput(dir, env, "write-tree")
}

// gitOutput runs git in dir with env added to the environment and returns its trimmed standard output.
func gitOutput(dir string, env []string, args ...string) (string, error) {
	cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
	cmd.Env = append(os.Environ(), env...)
	var out, stderr bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("git %s: %v: %s", args[0], err, strings.TrimSpace(stderr.String()))
	}
	return strings.TrimSpace(out.String()), nil
}

// PreflightLocal lists why a local upload would be refused before it is sent: the local block must be complete and in
// the form the server accepts, the report must name its platform, and the run must stay within the local result limit.
// The body limit is checked once the payload is built, in Upload.
func PreflightLocal(env Envelope, report []byte) []string {
	var problems []string
	if l := env.Local; l == nil {
		problems = append(problems, "the local run has no git facts")
	} else {
		if !repositoryName.MatchString(l.Repository) || len(l.Repository) < 3 || len(l.Repository) > 140 {
			problems = append(problems, "the repository is not owner/name for GitHub")
		}
		if !validRefName(l.Branch) {
			problems = append(problems, "the branch name is not valid (1 to 255 characters, no control characters)")
		}
		if !sha40.MatchString(l.CommitSHA) {
			problems = append(problems, "the commit is not a 40-character hash")
		}
		if !sha40.MatchString(l.Tree) {
			problems = append(problems, "the tree is not a 40-character hash")
		}
		if !signerID.MatchString(l.Signer) {
			problems = append(problems, "the signer is not 32 lower-case hex characters")
		}
		if l.Seq < 1 || l.Seq > maxSeq {
			problems = append(problems, "the sequence number is out of range")
		}
	}
	if !ValidLabel(env.RunLabel) {
		problems = append(problems, "the run label must be 1 to 64 characters of letters, digits, '.', '_', ':', '/' or '-'")
	}
	return append(problems, reportProblems(report, MaxLocalResults)...)
}
