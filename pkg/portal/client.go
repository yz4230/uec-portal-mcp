package portal

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/PuerkitoBio/goquery"
)

const (
	portalListArticlesURL  = "https://portalweb.uec.ac.jp/Portal/u008/getNoticeList.php"
	portalArticleDetailURL = "https://portalweb.uec.ac.jp/Portal/u008/getNoticeDetailBody.php"
	portalDateLayout       = "2006.01.02 15:04:05"
)

var noticeLinkPattern = regexp.MustCompile(`^javascript:openWin\(\s*(\d+)\s*,.*?\);$`)

type HTTPDoer interface {
	Do(*http.Request) (*http.Response, error)
}

type PortalClient struct {
	c HTTPDoer
}

func NewPortalClient(client HTTPDoer) *PortalClient {
	return &PortalClient{c: client}
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

func (pc *PortalClient) submitForm(ctx context.Context, actionURL *url.URL, values url.Values) (*goquery.Document, *http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, "POST", actionURL.String(), strings.NewReader(values.Encode()))
	if err != nil {
		return nil, nil, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	return pc.doDoc(req)
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
				article.Category = before
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
