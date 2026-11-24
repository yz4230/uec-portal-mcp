package uecauth

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestSessionRefreshesOnceAfterInterval(t *testing.T) {
	var loginCount atomic.Int32
	server := newAuthenticationServer(t, &loginCount)

	now := time.Date(2026, time.July, 17, 12, 0, 0, 0, time.UTC)
	authenticator := newTestAuthenticator(t, server.URL, &now)
	session, err := authenticator.Authenticate(context.Background())
	if err != nil {
		t.Fatalf("authenticate: %v", err)
	}
	firstState := session.current.Load()

	now = now.Add(29*time.Minute + 59*time.Second)
	if got := getResource(t, session, server.URL); got != "generation=1" {
		t.Fatalf("resource before refresh = %q, want generation=1", got)
	}
	if loginCount.Load() != 1 {
		t.Fatalf("login count before interval = %d, want 1", loginCount.Load())
	}

	now = now.Add(time.Second)
	const callers = 20
	start := make(chan struct{})
	results := make(chan string, callers)
	var wg sync.WaitGroup
	for range callers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			results <- getResource(t, session, server.URL)
		}()
	}
	close(start)
	wg.Wait()
	close(results)

	for result := range results {
		if result != "generation=2" {
			t.Errorf("resource after refresh = %q, want generation=2", result)
		}
	}
	if loginCount.Load() != 2 {
		t.Fatalf("login count after concurrent refresh = %d, want 2", loginCount.Load())
	}
	if session.current.Load() == firstState {
		t.Fatal("refresh did not swap the client state")
	}
	if got := session.AuthenticatedAt(); !got.Equal(now) {
		t.Fatalf("AuthenticatedAt = %s, want %s", got, now)
	}
}

func TestSessionKeepsCurrentClientWhenRefreshFails(t *testing.T) {
	var loginCount atomic.Int32
	var failLogin atomic.Bool
	server := newAuthenticationServerWithFailure(t, &loginCount, &failLogin)

	now := time.Date(2026, time.July, 17, 12, 0, 0, 0, time.UTC)
	authenticator := newTestAuthenticator(t, server.URL, &now)
	session, err := authenticator.Authenticate(context.Background())
	if err != nil {
		t.Fatalf("authenticate: %v", err)
	}
	firstState := session.current.Load()

	now = now.Add(30 * time.Minute)
	failLogin.Store(true)
	req, err := http.NewRequest(http.MethodGet, server.URL+"/resource", nil)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	if _, err := session.Do(req); err == nil || !strings.Contains(err.Error(), "refresh authentication") {
		t.Fatalf("refresh error = %v, want refresh authentication error", err)
	}
	if session.current.Load() != firstState {
		t.Fatal("failed refresh replaced the current client")
	}
}

func TestSessionAuthenticatesLazilyAndRetriesInitialFailure(t *testing.T) {
	var loginCount atomic.Int32
	var failLogin atomic.Bool
	failLogin.Store(true)
	server := newAuthenticationServerWithFailure(t, &loginCount, &failLogin)

	now := time.Date(2026, time.July, 17, 12, 0, 0, 0, time.UTC)
	authenticator := newTestAuthenticator(t, server.URL, &now)
	session := authenticator.NewSession()
	if loginCount.Load() != 0 || session.current.Load() != nil {
		t.Fatal("NewSession performed authentication eagerly")
	}

	req, err := http.NewRequest(http.MethodGet, server.URL+"/resource", nil)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	if _, err := session.Do(req); err == nil {
		t.Fatal("first request succeeded, want login error")
	}
	if session.current.Load() != nil {
		t.Fatal("failed initial login stored a client")
	}

	failLogin.Store(false)
	if got := getResource(t, session, server.URL); got != "generation=2" {
		t.Fatalf("resource after retry = %q, want generation=2", got)
	}
	if loginCount.Load() != 2 {
		t.Fatalf("login count = %d, want 2", loginCount.Load())
	}
}

func TestNewCopiesCredentials(t *testing.T) {
	credentials := &Credentials{Username: "alice", Password: "secret", OTPSecret: "otp"}
	authenticator, err := New(credentials)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	credentials.Password = "changed"
	if authenticator.credentials.Password != "secret" {
		t.Fatal("authenticator retained the caller-owned credentials pointer")
	}
}

func newTestAuthenticator(t *testing.T, serverURL string, now *time.Time) *Authenticator {
	t.Helper()
	credentials := &Credentials{Username: "alice", Password: "secret", OTPSecret: "otp-secret"}
	authenticator, err := New(credentials, WithRefreshInterval(30*time.Minute))
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	loginURL, err := url.Parse(serverURL + "/login")
	if err != nil {
		t.Fatalf("parse login URL: %v", err)
	}
	authenticator.loginURL = loginURL
	authenticator.validateURL = func(target *url.URL) error {
		if target.Host != loginURL.Host {
			return fmt.Errorf("unexpected host %q", target.Host)
		}
		return nil
	}
	authenticator.isAuthenticated = func(target *url.URL) bool {
		return target.Path == "/Portal/u001/home"
	}
	authenticator.generateOTP = func(secret string, at time.Time) (string, error) {
		if secret != "otp-secret" {
			t.Fatalf("OTP secret = %q", secret)
		}
		return "123456", nil
	}
	authenticator.now = func() time.Time { return *now }
	return authenticator
}

func newAuthenticationServer(t *testing.T, loginCount *atomic.Int32) *httptest.Server {
	t.Helper()
	return newAuthenticationServerWithFailure(t, loginCount, new(atomic.Bool))
}

func newAuthenticationServerWithFailure(t *testing.T, loginCount *atomic.Int32, failLogin *atomic.Bool) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/login", func(w http.ResponseWriter, r *http.Request) {
		generation := loginCount.Add(1)
		if failLogin.Load() {
			http.Error(w, "login unavailable", http.StatusServiceUnavailable)
			return
		}
		http.SetCookie(w, &http.Cookie{Name: "generation", Value: fmt.Sprint(generation), Path: "/"})
		_, _ = io.WriteString(w, `<form action="/password" method="post"><input name="csrf" value="token"><input name="j_username"><input name="j_password"></form>`)
	})
	mux.HandleFunc("/password", func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			t.Errorf("parse password form: %v", err)
		}
		if r.Form.Get("csrf") != "token" || r.Form.Get("j_username") != "alice" || r.Form.Get("j_password") != "secret" {
			t.Errorf("password form = %#v", r.Form)
		}
		_, _ = io.WriteString(w, `<form action="/otp" method="post"><input name="authcode"></form>`)
	})
	mux.HandleFunc("/otp", func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			t.Errorf("parse OTP form: %v", err)
		}
		if r.Form.Get("authcode") != "123456" {
			t.Errorf("authcode = %q, want 123456", r.Form.Get("authcode"))
		}
		_, _ = io.WriteString(w, `<form action="/consent" method="post"><input type="hidden" name="_shib_idp_consentIds" value="email" checked>`+
			`<button type="submit" name="_eventId_proceed">同意する</button><button type="submit" name="_eventId_AttributeReleaseRejected">拒否する</button></form>`)
	})
	mux.HandleFunc("/consent", func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			t.Errorf("parse consent form: %v", err)
		}
		if !r.Form.Has("_eventId_proceed") || r.Form.Has("_eventId_AttributeReleaseRejected") || r.Form.Get("_shib_idp_consentIds") != "email" {
			t.Errorf("consent form = %#v", r.Form)
		}
		http.Redirect(w, r, "/Portal/u001/home", http.StatusFound)
	})
	mux.HandleFunc("/Portal/u001/home", func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, "authenticated")
	})
	mux.HandleFunc("/resource", func(w http.ResponseWriter, r *http.Request) {
		cookie, err := r.Cookie("generation")
		if err != nil {
			http.Error(w, "missing generation cookie", http.StatusUnauthorized)
			return
		}
		_, _ = fmt.Fprintf(w, "generation=%s", cookie.Value)
	})
	return httptest.NewServer(mux)
}

func getResource(t *testing.T, session *Session, serverURL string) string {
	t.Helper()
	req, err := http.NewRequest(http.MethodGet, serverURL+"/resource", nil)
	if err != nil {
		t.Fatalf("new resource request: %v", err)
	}
	resp, err := session.Do(req)
	if err != nil {
		t.Errorf("get resource: %v", err)
		return ""
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Errorf("read resource: %v", err)
		return ""
	}
	return string(body)
}
