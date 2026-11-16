package agent

// WithToday lets the external tests pin the chat's today, which the past-date
// guard (D7) reads.
var WithToday = withToday

// WithWindow lets the external tests set the chat's plan window, which
// get_plan and regenerate read.
var WithWindow = withWindow

// ChatIssues and ChatFacts expose the chat prompt's issue list and fact
// sections to the external tests.
var (
	ChatIssues = chatIssues
	ChatFacts  = chatFacts
)
