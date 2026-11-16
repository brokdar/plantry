package handlers

// SetProviderBaseURLs points the handler at fake provider servers (empty
// keeps the default). Test-only: it lives in a _test file so production code
// has no such option.
func (h *SettingsHandler) SetProviderBaseURLs(anthropicURL, openaiURL string) {
	if anthropicURL != "" {
		h.anthropicURL = anthropicURL
	}
	if openaiURL != "" {
		h.openaiURL = openaiURL
	}
}
