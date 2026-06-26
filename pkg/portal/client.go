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

var noticeLinkPattern = regexp.MustCompile(`^javascript:openWin\(\s*(\d+)\s*,.*?\);$`)

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

func buildListArticlesForm(opts *ListArticlesOptions) (url.Values, error) {
	formdata := url.Values{}
	formdata.Set("method", "getNoticeList")
	formdata.Set("type", "99")
	formdata.Set("cate", "")
	formdata.Set("gadget", "0")
	formdata.Set("history", "1")

	keyword, year, page := "", "", "1"
	if opts != nil {
		keyword = opts.Keyword
		if opts.Year > 0 {
			year = strconv.Itoa(opts.Year)
		}
		if opts.Page > 0 {
			page = strconv.Itoa(opts.Page)
		}
	}
	formdata.Set("keyword", keyword)
	formdata.Set("year", year)
	formdata.Set("page", page)

	formdata.Set("showstudent", "0")
	formdata.Set("pld_sect1_val", "")
	formdata.Set("pld_sect2_val", "")
	formdata.Set("pld_sect3_val", "")
	formdata.Set("pld_sect4_val", "")
	formdata.Set("pld_year_val1", "")
	formdata.Set("list", "2")

	return formdata, nil
}

func parseArticleList(doc *goquery.Document) ([]*ArticleHeading, error) {
	articles := make([]*ArticleHeading, 0)
	for _, table := range doc.Find("table.def_table_info").EachIter() {
		article := &ArticleHeading{}

		titleEl := table.Find("h3").First()
		titleAnchorEl := titleEl.Find("a").First()
		titleHref, exists := titleAnchorEl.Attr("href")
		if !exists {
			slog.Warn("article title link not found")
			continue
		}
		matches := noticeLinkPattern.FindStringSubmatch(titleHref)
		if len(matches) < 2 {
			slog.Warn("article title link does not match expected pattern", "href", titleHref)
			continue
		}
		article.ArticleID = strings.TrimSpace(matches[1])
		article.Title = strings.TrimSpace(titleAnchorEl.Text())
		titleAnchorEl.Remove()

		article.Read = true
		if readEl := titleEl.Find("span[id^=VIEW_HISTORY_]"); readEl.Length() > 0 {
			article.Read = false
			readEl.Remove()
		}

		categoryText := strings.TrimSpace(titleEl.Text())
		if after, found := strings.CutPrefix(categoryText, "("); found {
			if before, found := strings.CutSuffix(after, ")"); found {
				article.Category = strings.TrimSpace(before)
			}
		}

		authorEl := table.Find("th.th_name").First()
		article.Author = strings.TrimSpace(authorEl.Text())

		publishDateEl := table.Find("th.th_date")
		publishStartEl := publishDateEl.First()
		publishStartEl.Find("strong").Remove()
		publishStartText := strings.TrimSpace(publishStartEl.Text())
		publishStart, err := time.Parse(portalDateLayout, publishStartText)
		if err != nil {
			slog.Warn("failed to parse publish start date", "text", publishStartText, "error", err)
			continue
		}
		article.PublishStart = publishStart

		publishEndEl := publishDateEl.Last()
		publishEndEl.Find("strong").Remove()
		publishEndText := strings.TrimSpace(publishEndEl.Text())
		publishEnd, err := time.Parse(portalDateLayout, publishEndText)
		if err != nil {
			slog.Warn("failed to parse publish end date", "text", publishEndText, "error", err)
			continue
		}
		article.PublishEnd = publishEnd

		articles = append(articles, article)
	}

	return articles, nil
}

func parsePortalDate(prefix, f1, f2 string) (time.Time, error) {
	dateStr := fmt.Sprintf("%s %s", f1, f2)
	dateStr, found := strings.CutPrefix(dateStr, prefix)
	if !found {
		return time.Time{}, fmt.Errorf("failed to parse date from string: %s", dateStr)
	}
	return time.Parse(portalDateLayout, dateStr)
}

func parseArticleDetail(doc *goquery.Document, articleID string) (*Article, error) {
	article := &Article{ArticleID: articleID}

	titleEl := doc.Find("#SCHEDULER_SUBJECT").First()
	if titleEl.Length() == 0 {
		return nil, fmt.Errorf("article detail title not found")
	}
	if title, exists := titleEl.Attr("value"); exists {
		article.Title = strings.TrimSpace(title)
	}

	bodyEl := doc.Find("#SCHEDULER_BODY").First()
	if bodyEl.Length() == 0 {
		return nil, fmt.Errorf("article detail body not found")
	}
	if content, exists := bodyEl.Attr("value"); exists {
		article.Content = strings.TrimSpace(content)
	}

	metaEl := doc.Find("span.def_date").First()
	if metaEl.Length() == 0 {
		return nil, fmt.Errorf("article detail metadata not found")
	}
	fields := strings.Fields(metaEl.Text())
	if len(fields) < 5 {
		return nil, fmt.Errorf("unexpected metadata format: %s", metaEl.Text())
	}

	startDate, err := parsePortalDate("開始日:", fields[0], fields[1])
	if err != nil {
		return nil, fmt.Errorf("failed to parse start date from metadata: %v", err)
	}
	article.PublishStart = startDate

	endDate, err := parsePortalDate("終了日:", fields[2], fields[3])
	if err != nil {
		return nil, fmt.Errorf("failed to parse end date from metadata: %v", err)
	}
	article.PublishEnd = endDate

	article.Author = fields[4]

	return article, nil
}

func (pc *PortalClient) ListArticles(ctx context.Context, opts *ListArticlesOptions) ([]*ArticleHeading, error) {
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
