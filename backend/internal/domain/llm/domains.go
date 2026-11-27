package llm

import (
	"fmt"
	"strings"
)

// DomainsBlockedError is a web search the provider refused as a whole
// because some allowed domains block its crawler (Anthropic: 400 "The
// following domains are not accessible to our user agent"). Retrying
// without Domains can succeed.
type DomainsBlockedError struct {
	Domains []string
	Err     error
}

func (e *DomainsBlockedError) Error() string {
	return fmt.Sprintf("web search domains blocked: %s: %v", strings.Join(e.Domains, ", "), e.Err)
}

func (e *DomainsBlockedError) Unwrap() error { return e.Err }
