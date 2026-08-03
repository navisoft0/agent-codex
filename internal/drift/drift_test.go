package drift

import "testing"

func TestCompute(t *testing.T) {
	const a, b, c = "sha256:aaa", "sha256:bbb", "sha256:ccc"
	cases := []struct {
		working, ancestor, latest string
		want                      State
	}{
		{a, a, a, Aligned},
		{a, a, b, Behind},
		{b, a, a, Drifted},
		{b, a, c, Diverged},
		{b, a, b, Diverged}, // local happens to match latest, but ancestor moved both ways
		{"", a, a, Missing},
		{"", a, b, Missing},
	}
	for _, tc := range cases {
		if got := Compute(tc.working, tc.ancestor, tc.latest); got != tc.want {
			t.Errorf("Compute(%q, %q, %q) = %s, want %s", tc.working, tc.ancestor, tc.latest, got, tc.want)
		}
	}
}
