// Package discovery verifies, annotates and stores recipe suggestions found
// by the research agent. It never calls an LLM itself.
package discovery

import (
	"context"
	"errors"
	"log/slog"
	"net/url"
	"runtime/debug"
	"sync"
	"time"

	"github.com/jaltszeimer/plantry/backend/internal/domain"
	"github.com/jaltszeimer/plantry/backend/internal/domain/importer"
)

// Discard reasons reported by Verify (counted in the run's notes).
const (
	ReasonBlockedURL = "blocked_url"
	ReasonDuplicate  = "duplicate"
	ReasonNoRecipe   = "no_recipe"
	ReasonNotHTML    = "not_html"
	ReasonFetch      = "fetch_failed"
	ReasonTimeout    = "timeout"
	ReasonError      = "error"
	ReasonPanic      = "panic"
	// ReasonAnnotate is the pipeline's: the candidate could not be annotated.
	ReasonAnnotate = "annotate"
)

const (
	defaultConcurrency = 3
	defaultTimeout     = 20 * time.Second
)

// Extractor is the import pipeline (importer.Service) as verify uses it.
type Extractor interface {
	Extract(ctx context.Context, in importer.ExtractInput) (*importer.Draft, error)
}

// URLIndex says whether a URL belongs to a saved recipe or an earlier
// suggestion. Implemented by the suggestion repository.
type URLIndex interface {
	SourceURLKnown(ctx context.Context, url string) (bool, error)
}

// Candidate is a page the research agent proposes (D5).
type Candidate struct {
	URL     string `json:"url"`
	Title   string `json:"title"`
	Cuisine string `json:"cuisine"`
	Why     string `json:"why"`
}

// Verified is a candidate whose page held a recipe.
type Verified struct {
	Candidate Candidate
	Draft     *importer.Draft
}

// Discarded is a candidate that did not make it, with the reason code.
type Discarded struct {
	URL    string
	Reason string
}

// Verifier checks candidate URLs the way the user's own import would: the
// app fetches every page itself (D9).
type Verifier struct {
	Extractor Extractor
	Index     URLIndex
	// Guard vets an LLM-supplied URL before it is requested (any error blocks
	// it); typically httpfetch.PublicURL with the dev-mode loopback switch.
	Guard func(rawURL string) error
	// Concurrency (default 3) and Timeout per URL (default 20 s).
	Concurrency int
	Timeout     time.Duration
	// Logger (default slog.Default) gets one line per discarded candidate.
	Logger *slog.Logger
}

// Verify extracts every candidate (JSON-LD only, no LLM fallback) and returns
// the drafts in candidate order plus the discarded ones. A URL that fails the
// guard is never requested. A candidate whose URL, or whose final URL after
// redirects, is already a food's source_url or an earlier suggestion's URL —
// or repeats an earlier candidate — is discarded as a duplicate. Only the
// caller's ctx ending is an error.
func (v *Verifier) Verify(ctx context.Context, candidates []Candidate) ([]Verified, []Discarded, error) {
	type slot struct {
		discard string
		err     error
		draft   *importer.Draft
	}
	slots := make([]slot, len(candidates))
	seen := map[string]bool{}

	// Pass 1 (sequential): guard and dedupe on the URL as given.
	var pending []int
	for i, c := range candidates {
		if err := ctx.Err(); err != nil {
			return nil, nil, err
		}
		if reason := v.vet(c.URL); reason != "" {
			slots[i].discard = reason
			continue
		}
		dup, err := v.duplicate(ctx, c.URL, seen)
		if err != nil {
			return nil, nil, err
		}
		if dup {
			slots[i].discard = ReasonDuplicate
			continue
		}
		pending = append(pending, i)
	}

	// Pass 2 (concurrent): the import pipeline per URL.
	n := v.Concurrency
	if n <= 0 {
		n = defaultConcurrency
	}
	timeout := v.Timeout
	if timeout <= 0 {
		timeout = defaultTimeout
	}
	var wg sync.WaitGroup
	sem := make(chan struct{}, n)
	for _, i := range pending {
		wg.Add(1)
		sem <- struct{}{}
		go func() {
			defer wg.Done()
			defer func() { <-sem }()
			defer func() {
				// Extraction parses untrusted pages: a parser panic discards
				// this candidate instead of killing the process.
				if p := recover(); p != nil {
					slots[i].draft, slots[i].discard = nil, ReasonPanic
					v.logger().ErrorContext(ctx, "discovery.verify panic",
						"host", host(candidates[i].URL), "panic", p, "stack", string(debug.Stack()))
				}
			}()
			cctx, cancel := context.WithTimeout(ctx, timeout)
			defer cancel()
			if v.Guard != nil {
				cctx = importer.WithURLGuard(cctx, v.Guard)
			}
			d, err := v.Extractor.Extract(cctx, importer.ExtractInput{
				URL: candidates[i].URL, Options: importer.Options{NoLLMFallback: true},
			})
			if err != nil {
				slots[i].discard, slots[i].err = discardReason(err), err
				return
			}
			slots[i].draft = d
		}()
	}
	wg.Wait()
	if err := ctx.Err(); err != nil {
		return nil, nil, err
	}

	// Pass 3 (sequential): the final URL after redirects.
	var kept []Verified
	var discarded []Discarded
	for i, c := range candidates {
		s := slots[i]
		if s.draft != nil && s.draft.SourceURL != c.URL {
			dup, err := v.duplicate(ctx, s.draft.SourceURL, seen)
			if err != nil {
				return nil, nil, err
			}
			if dup {
				s.draft, s.discard = nil, ReasonDuplicate
			}
		}
		if s.draft == nil {
			discarded = append(discarded, Discarded{URL: c.URL, Reason: s.discard})
			v.logDiscard(ctx, c.URL, s.discard, s.err)
			continue
		}
		kept = append(kept, Verified{Candidate: c, Draft: s.draft})
	}
	return kept, discarded, nil
}

func (v *Verifier) logger() *slog.Logger {
	if v.Logger == nil {
		return slog.Default()
	}
	return v.Logger
}

// logDiscard logs a discarded candidate by host only (an LLM-chosen URL's
// query is not logged). Unclassified errors carry the error at Warn.
func (v *Verifier) logDiscard(ctx context.Context, rawURL, reason string, err error) {
	if reason == ReasonError {
		v.logger().WarnContext(ctx, "discovery.verify discarded", "host", host(rawURL), "reason", reason, "error", err)
		return
	}
	v.logger().InfoContext(ctx, "discovery.verify discarded", "host", host(rawURL), "reason", reason)
}

func host(rawURL string) string {
	u, err := url.Parse(rawURL)
	if err != nil {
		return ""
	}
	return u.Host
}

func (v *Verifier) vet(rawURL string) string {
	if v.Guard == nil {
		return ""
	}
	if v.Guard(rawURL) != nil {
		return ReasonBlockedURL
	}
	return ""
}

// duplicate reports whether u is known, or already seen in this batch, and
// marks it seen.
func (v *Verifier) duplicate(ctx context.Context, u string, seen map[string]bool) (bool, error) {
	if seen[u] {
		return true, nil
	}
	known, err := v.Index.SourceURLKnown(ctx, u)
	if err != nil {
		return false, err
	}
	seen[u] = true
	return known, nil
}

func discardReason(err error) string {
	switch {
	case errors.Is(err, context.DeadlineExceeded):
		return ReasonTimeout
	case errors.Is(err, domain.ErrImportNoRecipe):
		return ReasonNoRecipe
	case errors.Is(err, domain.ErrImportNotHTML):
		return ReasonNotHTML
	case errors.Is(err, importer.ErrURLBlocked):
		return ReasonBlockedURL
	case errors.Is(err, domain.ErrImportFetchFailed), errors.Is(err, domain.ErrImportBodyTooLarge):
		return ReasonFetch
	default:
		return ReasonError
	}
}
