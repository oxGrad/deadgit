package azure

// newClientForTest exposes newClient for external tests.
func newClientForTest(pat string) *client { return newClient(pat) }
