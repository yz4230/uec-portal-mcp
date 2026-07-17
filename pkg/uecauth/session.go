package uecauth

import (
	"context"
	"fmt"
	"net/http"
	"sync"
	"sync/atomic"
	"time"
)

type clientState struct {
	client          *http.Client
	authenticatedAt time.Time
}

// Session is an authenticated HTTP client that refreshes itself before the
// first request made after its refresh interval. Requests are serialized until
// the preceding response headers are received.
type Session struct {
	authenticator *Authenticator
	current       atomic.Pointer[clientState]
	requestMu     sync.Mutex
}

// Do implements the HTTP client interface used by portal.Client.
func (s *Session) Do(req *http.Request) (*http.Response, error) {
	if req == nil {
		return nil, fmt.Errorf("request must not be nil")
	}

	s.requestMu.Lock()
	defer s.requestMu.Unlock()

	if err := s.ensureFresh(req.Context()); err != nil {
		return nil, err
	}
	return s.current.Load().client.Do(req)
}

// Refresh immediately creates a new authenticated client and atomically swaps
// it into the session. In-flight requests continue using the previous client.
func (s *Session) Refresh(ctx context.Context) error {
	s.requestMu.Lock()
	defer s.requestMu.Unlock()
	return s.refreshLocked(ctx)
}

// AuthenticatedAt returns when the current session generation logged in.
func (s *Session) AuthenticatedAt() time.Time {
	state := s.current.Load()
	if state == nil {
		return time.Time{}
	}
	return state.authenticatedAt
}

func (s *Session) ensureFresh(ctx context.Context) error {
	state := s.current.Load()
	if state != nil && s.authenticator.now().Before(state.authenticatedAt.Add(s.authenticator.refreshInterval)) {
		return nil
	}
	return s.refreshLocked(ctx)
}

func (s *Session) refreshLocked(ctx context.Context) error {
	client, err := s.authenticator.login(ctx)
	if err != nil {
		return fmt.Errorf("refresh authentication: %w", err)
	}
	s.current.Store(&clientState{
		client:          client,
		authenticatedAt: s.authenticator.now(),
	})
	return nil
}
