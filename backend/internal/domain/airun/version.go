package airun

import "runtime/debug"

// BuildVersion identifies the running build from its VCS stamp: the commit
// (vcs.revision, shortened) with "-dirty" when the tree had changes. It is
// empty for a binary without VCS info, such as `go test`.
func BuildVersion() string {
	bi, ok := debug.ReadBuildInfo()
	if !ok {
		return ""
	}
	var rev string
	dirty := false
	for _, s := range bi.Settings {
		switch s.Key {
		case "vcs.revision":
			rev = s.Value
		case "vcs.modified":
			dirty = s.Value == "true"
		}
	}
	if rev == "" {
		return ""
	}
	if len(rev) > 12 {
		rev = rev[:12]
	}
	if dirty {
		rev += "-dirty"
	}
	return rev
}
