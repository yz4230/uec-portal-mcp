package uecauth

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"strings"
	"time"

	"github.com/PuerkitoBio/goquery"
	"github.com/pquerna/otp/totp"
	"github.com/yz4230/uec-portal-mcp/pkg/httpx"
)

const (
	defaultLoginURL        = "https://portalweb.uec.ac.jp/Portal/login/login.php"
	defaultRefreshInterval = 30 * time.Minute
	maxFormSubmissions     = 10
	maxLoginResponseBytes  = 4 << 20

	browserUserAgent      = "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/149.0.0.0 Safari/537.36"
	browserAccept         = "text/html,application/xhtml+xml,application/xml;q=0.9,image/avif,image/webp,*/*;q=0.8"
	browserAcceptLanguage = "ja,en-US;q=0.9,en;q=0.8"
)

// Credentials contains the values needed by the UEC authentication flow.
type Credentials struct {
	Username  string `json:"username"`
	Password  string `json:"password"`
	OTPSecret string `json:"otp_secret"`
}

// Option configures an Authenticator.
type Option func(*Authenticator) error

// Authenticator creates and refreshes authenticated UEC HTTP sessions.
type Authenticator struct {
	credentials     Credentials
	baseClient      *http.Client
	loginURL        *url.URL
	refreshInterval time.Duration
	now             func() time.Time
	validateURL     func(*url.URL) error
	isAuthenticated func(*url.URL) bool
	generateOTP     func(string, time.Time) (string, error)
}

// New creates an authenticator using the UEC portal defaults.
func New(credentials *Credentials, options ...Option) (*Authenticator, error) {
	if credentials == nil {
		return nil, fmt.Errorf("credentials must not be nil")
	}
	loginURL, err := url.Parse(defaultLoginURL)
	if err != nil {
		return nil, err
	}

	a := &Authenticator{
		credentials:     *credentials,
		baseClient:      http.DefaultClient,
		loginURL:        loginURL,
		refreshInterval: defaultRefreshInterval,
		now:             time.Now,
		validateURL:     validateUECURL,
		isAuthenticated: isPortalTop,
		generateOTP:     totp.GenerateCode,
	}
	for _, option := range options {
		if err := option(a); err != nil {
			return nil, err
		}
	}
	if a.refreshInterval <= 0 {
		return nil, fmt.Errorf("refresh interval must be positive")
	}
	if err := a.validateURL(a.loginURL); err != nil {
		return nil, fmt.Errorf("invalid login URL: %w", err)
	}
	return a, nil
}

// WithHTTPClient supplies the base client whose timeout and transport are used.
// Each authentication attempt receives a fresh Cookie Jar.
func WithHTTPClient(client *http.Client) Option {
	return func(a *Authenticator) error {
		if client == nil {
			return fmt.Errorf("HTTP client must not be nil")
		}
		a.baseClient = client
		return nil
	}
}

// WithRefreshInterval changes how long an authenticated session is reused.
func WithRefreshInterval(interval time.Duration) Option {
	return func(a *Authenticator) error {
		a.refreshInterval = interval
		return nil
	}
}

// NewSession returns an automatically refreshing session. Authentication is
// deferred until its first request.
func (a *Authenticator) NewSession() *Session {
	return &Session{authenticator: a}
}

// Authenticate performs the initial login and returns an automatically
// refreshing session.
func (a *Authenticator) Authenticate(ctx context.Context) (*Session, error) {
	session := a.NewSession()
	if err := session.Refresh(ctx); err != nil {
		return nil, err
	}
	return session, nil
}

func (a *Authenticator) login(ctx context.Context) (*http.Client, error) {
	client, err := a.newHTTPClient()
	if err != nil {
		return nil, err
	}

	doc, resp, err := doGet(ctx, client, a.loginURL)
	if err != nil {
		return nil, fmt.Errorf("start login: %w", err)
	}

	visited := make([]string, 0, maxFormSubmissions+1)
	visited = append(visited, resp.Request.URL.Redacted())

	for range maxFormSubmissions {
		if a.isAuthenticated(resp.Request.URL) {
			return client, nil
		}

		form := doc.Find("form").First()
		if form.Length() == 0 {
			return nil, fmt.Errorf("login stopped at %s: no form found", resp.Request.URL.Redacted())
		}

		action, exists := form.Attr("action")
		if !exists {
			return nil, fmt.Errorf("login form action not found at %s", resp.Request.URL.Redacted())
		}
		actionURL, err := url.Parse(action)
		if err != nil {
			return nil, fmt.Errorf("invalid form action at %s: %w", resp.Request.URL.Redacted(), err)
		}
		actionURL = resp.Request.URL.ResolveReference(actionURL)
		if err := a.validateURL(actionURL); err != nil {
			return nil, fmt.Errorf("refusing login form action %s: %w", actionURL.Redacted(), err)
		}

		values := buildForm(form)
		if values.Has("j_username") && values.Has("j_password") {
			values.Set("j_username", a.credentials.Username)
			values.Set("j_password", a.credentials.Password)
			values.Set("_eventId_proceed", "")
		}
		if values.Has("authcode") {
			code, err := a.generateOTP(a.credentials.OTPSecret, a.now())
			if err != nil {
				return nil, fmt.Errorf("generate TOTP code: %w", err)
			}
			values.Set("authcode", code)
			values.Set("login", "Login")
		}

		doc, resp, err = submitForm(ctx, client, actionURL, values)
		if err != nil {
			return nil, fmt.Errorf("submit login form: %w", err)
		}
		visited = append(visited, resp.Request.URL.Redacted())
	}

	return nil, fmt.Errorf("login did not reach portal top after %d form submissions (visited %s)", maxFormSubmissions, strings.Join(visited, " -> "))
}

func (a *Authenticator) newHTTPClient() (*http.Client, error) {
	jar, err := cookiejar.New(nil)
	if err != nil {
		return nil, fmt.Errorf("create cookie jar: %w", err)
	}

	client := *a.baseClient
	client.Jar = jar

	headers := make(http.Header)
	headers.Set("User-Agent", browserUserAgent)
	headers.Set("Accept", browserAccept)
	headers.Set("Accept-Language", browserAcceptLanguage)
	client.Transport = &httpx.DefaultHeaderTransport{
		Transport: a.baseClient.Transport,
		Headers:   headers,
	}

	baseCheckRedirect := a.baseClient.CheckRedirect
	client.CheckRedirect = func(req *http.Request, via []*http.Request) error {
		if err := a.validateURL(req.URL); err != nil {
			return fmt.Errorf("refusing redirect to %s: %w", req.URL.Redacted(), err)
		}
		if baseCheckRedirect != nil {
			return baseCheckRedirect(req, via)
		}
		if len(via) >= 10 {
			return fmt.Errorf("stopped after 10 redirects")
		}
		return nil
	}

	return &client, nil
}

func validateUECURL(target *url.URL) error {
	if target.Scheme != "https" {
		return fmt.Errorf("scheme must be https")
	}
	host := strings.ToLower(target.Hostname())
	if host != "uec.ac.jp" && !strings.HasSuffix(host, ".uec.ac.jp") {
		return fmt.Errorf("host %q is outside uec.ac.jp", host)
	}
	return nil
}

func isPortalTop(target *url.URL) bool {
	return strings.EqualFold(target.Hostname(), "portalweb.uec.ac.jp") && strings.HasPrefix(target.Path, "/Portal/u001/")
}

func buildForm(form *goquery.Selection) url.Values {
	values := url.Values{}
	for _, input := range form.Find("input").EachIter() {
		name, exists := input.Attr("name")
		if !exists {
			continue
		}
		switch input.AttrOr("type", "text") {
		case "submit", "button", "reset", "image", "file":
			continue
		case "checkbox", "radio":
			if _, checked := input.Attr("checked"); !checked {
				continue
			}
		}
		values.Add(name, input.AttrOr("value", ""))
	}
	for _, selection := range form.Find("select").EachIter() {
		name, exists := selection.Attr("name")
		if !exists {
			continue
		}
		selected := selection.Find("option[selected]")
		if selected.Length() == 0 {
			selected = selection.Find("option").First()
		}
		values.Add(name, selected.AttrOr("value", ""))
	}
	return values
}

func doGet(ctx context.Context, client *http.Client, target *url.URL) (*goquery.Document, *http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, target.String(), nil)
	if err != nil {
		return nil, nil, err
	}
	return doDocument(client, req)
}

func submitForm(ctx context.Context, client *http.Client, actionURL *url.URL, values url.Values) (*goquery.Document, *http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, actionURL.String(), strings.NewReader(values.Encode()))
	if err != nil {
		return nil, nil, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	return doDocument(client, req)
}

func doDocument(client *http.Client, req *http.Request) (*goquery.Document, *http.Response, error) {
	resp, err := client.Do(req)
	if err != nil {
		return nil, nil, err
	}
	defer resp.Body.Close()
	doc, err := goquery.NewDocumentFromReader(io.LimitReader(resp.Body, maxLoginResponseBytes))
	if err != nil {
		return nil, nil, err
	}
	return doc, resp, nil
}
