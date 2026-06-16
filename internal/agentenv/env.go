package agentenv

import "strings"

var allowedNames = map[string]struct{}{
	"PATH":     {},
	"HOME":     {},
	"SHELL":    {},
	"USER":     {},
	"LOGNAME":  {},
	"TMPDIR":   {},
	"TEMP":     {},
	"TMP":      {},
	"LANG":     {},
	"LC_ALL":   {},
	"LC_CTYPE": {},
	"TERM":     {},
}

// Filter returns the small environment allowlist passed to coding agents.
func Filter(env []string) []string {
	out := make([]string, 0, len(env))
	for _, entry := range env {
		name, _, ok := strings.Cut(entry, "=")
		if !ok {
			continue
		}
		if _, allowed := allowedNames[name]; !allowed {
			continue
		}
		out = append(out, entry)
	}
	return out
}
