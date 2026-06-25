package httpx

import "net/http"

type DefaultHeaderTransport struct {
	Transport http.RoundTripper
	Headers   http.Header
}

func (t *DefaultHeaderTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	r2 := r.Clone(r.Context())
	for key, values := range t.Headers {
		if r2.Header.Get(key) == "" {
			for _, value := range values {
				r2.Header.Add(key, value)
			}
		}
	}

	base := t.Transport
	if base == nil {
		base = http.DefaultTransport
	}
	return base.RoundTrip(r2)
}
