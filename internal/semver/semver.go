// Package semver implements the minimal version arithmetic shu needs for
// skill constraints: exact matches and caret ranges over the behavioral
// semver convention (MAJOR = changed behavior, MINOR = additive, PATCH =
// clarification). Content pins (sha256:...) are compared by hash by the
// caller, not here.
package semver

import (
	"strconv"
	"strings"
)

// Version is a parsed three-part version; missing parts are zero.
type Version struct {
	Major, Minor, Patch int
}

// Parse reads "1", "1.4", "v1.4.0", or "1.4.0-rc1" (prerelease ignored).
func Parse(s string) (Version, bool) {
	s = strings.TrimPrefix(strings.TrimSpace(s), "v")
	if i := strings.IndexAny(s, "-+"); i >= 0 {
		s = s[:i]
	}
	parts := strings.Split(s, ".")
	if len(parts) == 0 || len(parts) > 3 {
		return Version{}, false
	}
	var v Version
	for i, p := range parts {
		n, err := strconv.Atoi(p)
		if err != nil || n < 0 {
			return Version{}, false
		}
		switch i {
		case 0:
			v.Major = n
		case 1:
			v.Minor = n
		case 2:
			v.Patch = n
		}
	}
	return v, true
}

// Compare returns -1, 0, or 1 for a<b, a==b, a>b.
func Compare(a, b Version) int {
	for _, d := range [3]int{a.Major - b.Major, a.Minor - b.Minor, a.Patch - b.Patch} {
		if d < 0 {
			return -1
		}
		if d > 0 {
			return 1
		}
	}
	return 0
}

// Satisfies reports whether ver satisfies constraint:
//
//	""/"latest"  anything
//	"1.4.0"      exactly that version
//	"^1.4"       >= 1.4.0 and same major (behavioral compatibility)
func Satisfies(ver, constraint string) bool {
	c := strings.TrimSpace(constraint)
	if c == "" || c == "latest" {
		return true
	}
	if strings.HasPrefix(c, "^") {
		base, ok := Parse(c[1:])
		v, ok2 := Parse(ver)
		return ok && ok2 && v.Major == base.Major && Compare(v, base) >= 0
	}
	base, ok := Parse(c)
	v, ok2 := Parse(ver)
	if ok && ok2 {
		return Compare(v, base) == 0
	}
	return ver == c
}
