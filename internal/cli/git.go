package cli

import (
	"fmt"
	"os/exec"
	"regexp"
	"strings"
)

// git runs a git command in dir and returns its combined output.
func git(dir string, args ...string) (string, error) {
	cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return string(out), fmt.Errorf("git %s: %v\n%s", strings.Join(args, " "), err, out)
	}
	return string(out), nil
}

var githubRemote = regexp.MustCompile(`github\.com[:/]([^/]+)/([^/]+?)(?:\.git)?/?$`)

// githubCompareURL turns a GitHub remote plus branch into a ready-to-open
// PR compare link, or "" for non-GitHub remotes.
func githubCompareURL(remote, branch string) string {
	m := githubRemote.FindStringSubmatch(strings.TrimSpace(remote))
	if m == nil {
		return ""
	}
	return fmt.Sprintf("https://github.com/%s/%s/compare/%s?expand=1", m[1], m[2], branch)
}

// gitIdentity returns the user.name/user.email visible from dir, with
// fallbacks so commits made by shu never fail on missing config.
func gitIdentity(dir string) (name, email string) {
	name, email = "shu", "shu@localhost"
	if out, err := git(dir, "config", "--get", "user.name"); err == nil && strings.TrimSpace(out) != "" {
		name = strings.TrimSpace(out)
	}
	if out, err := git(dir, "config", "--get", "user.email"); err == nil && strings.TrimSpace(out) != "" {
		email = strings.TrimSpace(out)
	}
	return name, email
}

// commitAll stages everything in dir and commits with the given identity and
// message. Returns false when there was nothing to commit.
func commitAll(dir, name, email, message string) (bool, error) {
	if _, err := git(dir, "add", "-A"); err != nil {
		return false, err
	}
	if out, err := git(dir, "status", "--porcelain"); err != nil {
		return false, err
	} else if strings.TrimSpace(out) == "" {
		return false, nil
	}
	_, err := git(dir, "-c", "user.name="+name, "-c", "user.email="+email, "commit", "-m", message)
	return true, err
}
