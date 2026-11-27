package httpfetch

import (
	"errors"
	"strings"
)

// userAgentToken is the product token robots.txt groups address Plantry by.
const userAgentToken = "plantry"

// aiUserAgents are the agents that act for a user (FIND-6); their groups
// apply to Plantry unless the site is exempt. Groups naming only training or
// search crawlers (GPTBot, ClaudeBot, anthropic-ai, Claude-SearchBot, CCBot,
// Google-Extended, Bytespider, ...) never apply: Plantry is none of those.
var aiUserAgents = map[string]bool{
	"claude-user":     true,
	"chatgpt-user":    true,
	"chatgpt agent":   true,
	"perplexity-user": true,
	"mistralai-user":  true,
	"gemini-user":     true,
}

// errMalformedRobots marks a robots.txt that cannot be read; the host is then
// treated as disallowing everything.
var errMalformedRobots = errors.New("httpfetch: malformed robots.txt")

type robotsRule struct {
	allow   bool
	pattern string
}

type robotsGroup struct {
	agents []string // lower case
	rules  []robotsRule
}

// robots is a parsed robots.txt.
type robots struct{ groups []robotsGroup }

// parseRobots reads body. A line that is neither blank, a comment nor a
// "name: value" directive, a NUL byte, or an Allow/Disallow before any
// User-agent is a parse error.
func parseRobots(body string) (*robots, error) {
	if strings.ContainsRune(body, 0) {
		return nil, errMalformedRobots
	}
	r := &robots{}
	var cur *robotsGroup
	collecting := false // the last directive was a User-agent
	for line := range strings.SplitSeq(body, "\n") {
		if i := strings.IndexByte(line, '#'); i >= 0 {
			line = line[:i]
		}
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		name, value, ok := strings.Cut(line, ":")
		if !ok {
			return nil, errMalformedRobots
		}
		value = strings.TrimSpace(value)
		switch strings.ToLower(strings.TrimSpace(name)) {
		case "user-agent":
			if !collecting {
				r.groups = append(r.groups, robotsGroup{})
				cur = &r.groups[len(r.groups)-1]
			}
			cur.agents = append(cur.agents, strings.ToLower(value))
			collecting = true
		case "allow", "disallow":
			if cur == nil {
				return nil, errMalformedRobots
			}
			collecting = false
			if value != "" { // an empty Disallow allows everything; an empty Allow adds nothing
				cur.rules = append(cur.rules, robotsRule{allow: strings.EqualFold(strings.TrimSpace(name), "allow"), pattern: value})
			}
		default: // Sitemap, Crawl-delay, Host, ...
			collecting = false
		}
	}
	return r, nil
}

// allowed reports whether path (with query) may be fetched. Every group that
// applies must allow it: `*`, `Plantry`, and the AI user-agent groups unless
// aiExempt.
func (r *robots) allowed(path string, aiExempt bool) bool {
	for _, g := range r.groups {
		if g.applies(aiExempt) && !g.allows(path) {
			return false
		}
	}
	return true
}

func (g robotsGroup) applies(aiExempt bool) bool {
	for _, a := range g.agents {
		if a == "*" || a == userAgentToken || (!aiExempt && aiUserAgents[a]) {
			return true
		}
	}
	return false
}

// allows applies the longest matching rule; Allow wins a tie; no match allows.
func (g robotsGroup) allows(path string) bool {
	best, allow := -1, true
	for _, rule := range g.rules {
		if n := len(rule.pattern); n >= best && matchRobots(rule.pattern, path) {
			if n > best || rule.allow {
				allow = rule.allow
			}
			best = n
		}
	}
	return allow
}

// matchRobots matches a robots pattern (`*` wildcard, trailing `$` anchor)
// against the start of path.
func matchRobots(pattern, path string) bool {
	anchored := strings.HasSuffix(pattern, "$")
	pattern = strings.TrimSuffix(pattern, "$")
	parts := strings.Split(pattern, "*")
	if !strings.HasPrefix(path, parts[0]) {
		return false
	}
	path = path[len(parts[0]):]
	for i, p := range parts[1:] {
		last := i == len(parts)-2
		if last && anchored && p == "" {
			return true
		}
		if last && anchored {
			return strings.HasSuffix(path, p)
		}
		idx := strings.Index(path, p)
		if idx < 0 {
			return false
		}
		path = path[idx+len(p):]
	}
	return !anchored || path == ""
}
