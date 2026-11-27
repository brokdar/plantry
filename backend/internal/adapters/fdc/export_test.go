package fdc

// WithTestBaseURL points the provider's API client at u.
func WithTestBaseURL(d *DynamicProvider, u string) *DynamicProvider {
	d.clientOpts = []Option{WithBaseURL(u)}
	return d
}
