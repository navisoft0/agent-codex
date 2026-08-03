// Package drift computes the sync state of an installed skill by comparing
// three content hashes: the working copy in the consuming repo, the recorded
// ancestor (the exact upstream content last synced), and the latest upstream.
package drift

// State describes how an installed skill relates to its upstream.
type State string

const (
	// Aligned: working == ancestor == latest. Nothing to do.
	Aligned State = "aligned"
	// Behind: working == ancestor, upstream moved. Clean fast-forward possible.
	Behind State = "behind"
	// Drifted: working != ancestor, upstream unmoved. Local learnings to harvest.
	Drifted State = "drifted"
	// Diverged: both moved. A 3-way merge is required.
	Diverged State = "diverged"
	// Missing: in the lockfile but not on disk.
	Missing State = "missing"
)

// Compute derives the drift state from the three content hashes. An empty
// working hash means the install path does not exist.
func Compute(working, ancestor, latest string) State {
	switch {
	case working == "":
		return Missing
	case working == ancestor && ancestor == latest:
		return Aligned
	case working == ancestor:
		return Behind
	case ancestor == latest:
		return Drifted
	default:
		return Diverged
	}
}
