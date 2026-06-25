package portal

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/PuerkitoBio/goquery"
	"github.com/pquerna/otp/totp"
	"github.com/yz4230/uec-portal-mcp/pkg/httpx"
	"golang.org/x/net/html"
)

const (
	portalLoginURL          = "https://portalweb.uec.ac.jp/Portal/login/login.php"
	portalListArticlesURL   = "https://portalweb.uec.ac.jp/Portal/u008/getNoticeList.php"
	portalArticleDetailURL  = "https://portalweb.uec.ac.jp/Portal/u008/getNoticeDetailBody.php"
	maxLoginFormSubmissions = 10

	browserUserAgent      = "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/149.0.0.0 Safari/537.36"
	browserAccept         = "text/html,application/xhtml+xml,application/xml;q=0.9,image/avif,image/webp,*/*;q=0.8"
	browserAcceptLanguage = "ja,en-US;q=0.9,en;q=0.8"
	portalDateLayout      = "2006.01.02 15:04:05"
)

var (
	noticeLinkPattern     = regexp.MustCompile(`^javascript:openWin\(\s*(\d+)\s*,.*?\);$`)
	detailMetadataPattern = regexp.MustCompile(`(?s)開始日:\s*([0-9. :]+).*?終了日:\s*([0-9. :]+)\s*(.*?)\s*通知対象`)
)

type PortalClient struct {
	c    *http.Client
	auth *AuthConfig
}

func NewPortalClient(auth *AuthConfig) *PortalClient {
	jar, _ := cookiejar.New(nil)

	header := make(http.Header)
	header.Set("User-Agent", browserUserAgent)
	header.Set("Accept", browserAccept)
	header.Set("Accept-Language", browserAcceptLanguage)

	return &PortalClient{
		c: &http.Client{
			Jar:       jar,
			Transport: &httpx.DefaultHeaderTransport{Headers: header},
		},
		auth: auth,
	}
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

	for _, sel := range form.Find("select").EachIter() {
		name, exists := sel.Attr("name")
		if !exists {
			continue
		}

		selected := sel.Find("option[selected]")
		if selected.Length() == 0 {
			selected = sel.Find("option").First()
		}
		values.Add(name, selected.AttrOr("value", ""))
	}

	return values
}

func isPortalTop(url *url.URL) bool {
	return url.Host == "portalweb.uec.ac.jp" && strings.HasPrefix(url.Path, "/Portal/u001/")
}

func (pc *PortalClient) doDoc(req *http.Request) (*goquery.Document, *http.Response, error) {
	resp, err := pc.c.Do(req)
	if err != nil {
		return nil, nil, err
	}
	defer resp.Body.Close()
	doc, err := goquery.NewDocumentFromReader(resp.Body)
	if err != nil {
		return nil, nil, err
	}
	return doc, resp, nil
}

func (pc *PortalClient) doGet(ctx context.Context, target string) (*goquery.Document, *http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", target, nil)
	if err != nil {
		return nil, nil, err
	}
	return pc.doDoc(req)
}

func (pc *PortalClient) submitForm(ctx context.Context, actionURL *url.URL, values url.Values) (*goquery.Document, *http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, "POST", actionURL.String(), strings.NewReader(values.Encode()))
	if err != nil {
		return nil, nil, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	return pc.doDoc(req)
}

func (pc *PortalClient) Login(ctx context.Context) error {
	doc, resp, err := pc.doGet(ctx, portalLoginURL)
	if err != nil {
		return err
	}

	stack := make([]*url.URL, 0, maxLoginFormSubmissions)
	stack = append(stack, resp.Request.URL)

	for range maxLoginFormSubmissions {
		if isPortalTop(resp.Request.URL) {
			return nil
		}

		form := doc.Find("form").First()
		if form.Length() == 0 {
			return fmt.Errorf("login stopped at %s: no form found", resp.Request.URL.String())
		}

		action, exists := form.Attr("action")
		if !exists {
			return fmt.Errorf("login form action not found at %s", resp.Request.URL.String())
		}
		actionURL, err := url.Parse(action)
		if err != nil {
			return fmt.Errorf("invalid form action URL at %s: %v", resp.Request.URL.String(), err)
		}
		actionURL = resp.Request.URL.ResolveReference(actionURL)

		formdata := buildForm(form)
		if formdata.Has("j_username") && formdata.Has("j_password") {
			formdata.Set("j_username", pc.auth.Username)
			formdata.Set("j_password", pc.auth.Password)
			formdata.Set("_eventId_proceed", "") // submit button
		}
		if formdata.Has("authcode") {
			code, err := totp.GenerateCode(pc.auth.OTPSecret, time.Now())
			if err != nil {
				return fmt.Errorf("failed to generate TOTP code: %v", err)
			}
			formdata.Set("authcode", code)
			formdata.Set("login", "Login") // submit button
		}

		doc, resp, err = pc.submitForm(ctx, actionURL, formdata)
		if err != nil {
			return err
		}
		stack = append(stack, resp.Request.URL)
	}

	slog.Warn("login did not reach portal top page", "stack", stack)

	return fmt.Errorf("login did not reach portal top page after %d form submissions", maxLoginFormSubmissions)
}

// curl 'https://portalweb.uec.ac.jp/Portal/u008/getNoticeList.php' \
// -H 'Accept: */*' \
// -H 'Accept-Language: en-US,en;q=0.9,ja;q=0.8' \
// -H 'Connection: keep-alive' \
// -H 'Content-Type: application/x-www-form-urlencoded;charset=UTF-8' \
// -b '_ga=GA1.1.489448210.1770793970; _ga_1PE3JQPKLP=GS2.1.s1776149790$o2$g1$t1776149821$j29$l0$h0; _ga_BRCFJR3G3Z=GS2.1.s1780557761$o6$g0$t1780557768$j53$l0$h0; _shibsession_64656661756c7468747470733a2f2f706f7274616c7765622e7565632e61632e6a702f73686962626f6c6574682d7370=_e19c65f4d4f88935c147c5612bbcd10e; PHPSESSID=vp3g99shsmj948ekvebkktvikv' \
// -H 'Origin: https://portalweb.uec.ac.jp' \
// -H 'Referer: https://portalweb.uec.ac.jp/Portal/u008/noticeTop.php' \
// -H 'Sec-Fetch-Dest: empty' \
// -H 'Sec-Fetch-Mode: cors' \
// -H 'Sec-Fetch-Site: same-origin' \
// -H 'User-Agent: Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/149.0.0.0 Safari/537.36' \
// -H 'sec-ch-ua: "Google Chrome";v="149", "Chromium";v="149", "Not)A;Brand";v="24"' \
// -H 'sec-ch-ua-mobile: ?0' \
// -H 'sec-ch-ua-platform: "macOS"' \
// --data-raw 'method=getNoticeList&type=99&cate=&gadget=0&list=1'

func must[T any](value T, err error) T {
	if err != nil {
		panic(err)
	}
	return value
}

func buildListArticlesForm(opts *ListArticlesOptions) (url.Values, error) {
	formdata := url.Values{}
	formdata.Set("method", "getNoticeList")
	formdata.Set("type", "99")
	formdata.Set("cate", "")
	formdata.Set("gadget", "0")
	formdata.Set("list", "1")
	if opts == nil {
		return formdata, nil
	}
	if opts.Page < 0 {
		return nil, fmt.Errorf("page must be greater than or equal to zero")
	}
	if opts.Year < 0 {
		return nil, fmt.Errorf("year must be greater than or equal to zero")
	}
	if opts.Type != "" {
		formdata.Set("type", opts.Type)
	}
	if opts.Category != "" {
		formdata.Set("cate", opts.Category)
	}

	useSearchList := opts.Page > 0 || opts.Keyword != "" || opts.Year > 0
	if !useSearchList {
		return formdata, nil
	}

	page := opts.Page
	if page == 0 {
		page = 1
	}
	year := opts.Year
	if year == 0 {
		year = time.Now().Year()
	}

	formdata.Set("history", "1")
	formdata.Set("keyword", opts.Keyword)
	formdata.Set("year", strconv.Itoa(year))
	formdata.Set("page", strconv.Itoa(page))
	formdata.Set("showstudent", "0")
	formdata.Set("pld_sect1_val", "")
	formdata.Set("pld_sect2_val", "")
	formdata.Set("pld_sect3_val", "")
	formdata.Set("pld_sect4_val", "")
	formdata.Set("pld_year_val1", "")
	formdata.Set("list", "2")
	return formdata, nil
}

func textWithLineBreaks(sel *goquery.Selection) string {
	clone := sel.Clone()
	clone.Find("br").Each(func(_ int, br *goquery.Selection) {
		br.ReplaceWithNodes(&html.Node{Type: html.TextNode, Data: "\n"})
	})
	return strings.TrimSpace(clone.Text())
}

func parsePortalDate(text string) (time.Time, error) {
	return time.Parse(portalDateLayout, strings.TrimSpace(text))
}

func parseArticleList(doc *goquery.Document) ([]*Article, error) {
	articles := make([]*Article, 0)
	for _, table := range doc.Find("table.def_table_info").EachIter() {
		article := &Article{}

		titleEl := table.Find("h3 a").First()
		titleHref, exists := titleEl.Attr("href")
		if !exists {
			slog.Warn("article title link not found")
			continue
		}
		matches := noticeLinkPattern.FindStringSubmatch(strings.Join(strings.Fields(titleHref), ""))
		if len(matches) < 2 {
			slog.Warn("article title link does not match expected pattern", "href", titleHref)
			continue
		}
		article.ArticleID = strings.TrimSpace(matches[1])
		article.Title = strings.TrimSpace(titleEl.Text())

		authorEl := table.Find("th.th_name").First()
		article.Author = strings.TrimSpace(authorEl.Text())

		publishDateEl := table.Find("th.th_date")
		publishStartEl := publishDateEl.First()
		publishStartEl.Find("strong").Remove()
		publishStartText := strings.TrimSpace(publishStartEl.Text())
		publishStart, err := parsePortalDate(publishStartText)
		if err != nil {
			slog.Warn("failed to parse publish start date", "text", publishStartText, "error", err)
			continue
		}
		article.PublishStart = publishStart

		publishEndEl := publishDateEl.Last()
		publishEndEl.Find("strong").Remove()
		publishEndText := strings.TrimSpace(publishEndEl.Text())
		publishEnd, err := parsePortalDate(publishEndText)
		if err != nil {
			slog.Warn("failed to parse publish end date", "text", publishEndText, "error", err)
			continue
		}
		article.PublishEnd = publishEnd

		contentEl := table.Find("p.def_p").First()
		article.Content = textWithLineBreaks(contentEl)

		articles = append(articles, article)
	}

	return articles, nil
}

func parseArticleDetail(doc *goquery.Document, articleID string) (*Article, error) {
	titleEl := doc.Find("#src1_subject").First()
	if titleEl.Length() == 0 {
		return nil, fmt.Errorf("article detail title not found")
	}

	article := &Article{
		ArticleID: strings.TrimSpace(articleID),
		Title:     strings.TrimSpace(titleEl.Clone().Children().Remove().End().Text()),
	}
	if article.Title == "" {
		article.Title = strings.TrimSpace(titleEl.Text())
	}

	metadataText := strings.Join(strings.Fields(doc.Find("span.def_date").First().Text()), " ")
	matches := detailMetadataPattern.FindStringSubmatch(metadataText)
	if len(matches) >= 4 {
		publishStart, err := parsePortalDate(matches[1])
		if err != nil {
			return nil, fmt.Errorf("parse publish start date %q: %w", matches[1], err)
		}
		publishEnd, err := parsePortalDate(matches[2])
		if err != nil {
			return nil, fmt.Errorf("parse publish end date %q: %w", matches[2], err)
		}
		article.PublishStart = publishStart
		article.PublishEnd = publishEnd
		article.Author = strings.TrimSpace(matches[3])
	}

	bodyEl := doc.Find("#src1_body").First()
	if bodyEl.Length() == 0 {
		return nil, fmt.Errorf("article detail body not found")
	}
	article.Content = textWithLineBreaks(bodyEl)

	return article, nil
}

func (pc *PortalClient) ListArticles(ctx context.Context, opts *ListArticlesOptions) ([]*Article, error) {
	formdata, err := buildListArticlesForm(opts)
	if err != nil {
		return nil, err
	}
	doc, _, err := pc.submitForm(ctx, must(url.Parse(portalListArticlesURL)), formdata)
	if err != nil {
		return nil, err
	}
	return parseArticleList(doc)
}

func (pc *PortalClient) GetArticle(ctx context.Context, articleID string, opts *GetArticleOptions) (*Article, error) {
	articleID = strings.TrimSpace(articleID)
	if articleID == "" {
		return nil, fmt.Errorf("article ID is required")
	}

	formdata := url.Values{}
	formdata.Set("method", "getNoticeDetail")
	formdata.Set("notice_idx", articleID)
	formdata.Set("history", "0")
	if opts != nil && opts.History {
		formdata.Set("history", "1")
	}

	doc, _, err := pc.submitForm(ctx, must(url.Parse(portalArticleDetailURL)), formdata)
	if err != nil {
		return nil, err
	}
	return parseArticleDetail(doc, articleID)
}
