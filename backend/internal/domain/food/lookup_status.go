package food

import "context"

// Lookup unavailability reasons a handler reports with an empty result, so
// "nothing found" is told apart from "the sources could not be asked".
const (
	LookupReasonNotConfigured      = "not_configured"
	LookupReasonSourcesUnavailable = "sources_unavailable"
)

// LookupStatus records which external sources a lookup could not use. A
// handler puts one on ctx (WithLookupStatus) and reads Reason afterwards.
type LookupStatus struct {
	fdcNotConfigured bool
	sourceFailed     bool
}

type lookupStatusKey struct{}

// WithLookupStatus returns ctx carrying a fresh LookupStatus.
func WithLookupStatus(ctx context.Context) (context.Context, *LookupStatus) {
	st := &LookupStatus{}
	return context.WithValue(ctx, lookupStatusKey{}, st), st
}

func lookupStatusFrom(ctx context.Context) *LookupStatus {
	st, _ := ctx.Value(lookupStatusKey{}).(*LookupStatus)
	return st
}

func (s *LookupStatus) markFDCNotConfigured() {
	if s != nil {
		s.fdcNotConfigured = true
	}
}

func (s *LookupStatus) markSourceFailed() {
	if s != nil {
		s.sourceFailed = true
	}
}

// Reason is why an empty lookup may be no real "not found": a source that
// failed, else the nutrition source that is not configured, else "".
func (s *LookupStatus) Reason() string {
	switch {
	case s == nil:
		return ""
	case s.sourceFailed:
		return LookupReasonSourcesUnavailable
	case s.fdcNotConfigured:
		return LookupReasonNotConfigured
	}
	return ""
}
