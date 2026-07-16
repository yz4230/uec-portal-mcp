package tools

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/yz4230/uec-portal-mcp/pkg/portal"
)

const maxArticleListContentItems = 20

const portalClientTTL = 30 * time.Minute

var ListArticlesTool = &mcp.Tool{
	Name:        "list_articles",
	Title:       "List Portal Articles",
	Description: "Fetch UEC Student Portal bulletin board article headings. Use page for pagination, keyword for search text, and year to filter by publication year.",
	Annotations: &mcp.ToolAnnotations{
		ReadOnlyHint:    true,
		DestructiveHint: new(false),
		IdempotentHint:  true,
		OpenWorldHint:   new(true),
	},
}

var GetArticleTool = &mcp.Tool{
	Name:        "get_article",
	Title:       "Get Portal Article",
	Description: "Fetch the full body and metadata for a UEC Student Portal bulletin board article. Pass an article_id returned by list_articles; set history=true for search/history results.",
	Annotations: &mcp.ToolAnnotations{
		ReadOnlyHint:    true,
		DestructiveHint: new(false),
		IdempotentHint:  true,
		OpenWorldHint:   new(true),
	},
}

type ListArticlesOutput struct {
	Articles []*portal.ArticleHeading `json:"articles" jsonschema:"Article headings with article_id, title, author, category, read state, and publication dates"`
	Count    int                      `json:"count" jsonschema:"Number of article headings returned"`
}

type GetArticleInput struct {
	ArticleID string `json:"article_id" jsonschema:"Article ID returned by list_articles"`
	History   bool   `json:"history,omitempty" jsonschema:"Set true when retrieving an article ID from search/history results"`
}

type GetArticleOutput struct {
	Article *portal.Article `json:"article" jsonschema:"Full article detail including title, body, author, and publication dates"`
}

type portalClientCache struct {
	mu        sync.Mutex
	ttl       time.Duration
	now       func() time.Time
	load      func(context.Context) (*portal.PortalClient, error)
	client    *portal.PortalClient
	expiresAt time.Time
}

func (c *portalClientCache) get(ctx context.Context) (*portal.PortalClient, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.client != nil && c.now().Before(c.expiresAt) {
		return c.client, nil
	}

	client, err := c.load(ctx)
	if err != nil {
		return nil, err
	}

	c.client = client
	c.expiresAt = c.now().Add(c.ttl)
	return client, nil
}

func loadLoggedInPortalClient(ctx context.Context) (*portal.PortalClient, error) {
	auth, err := portal.LoadAuthConfig()
	if err != nil {
		return nil, err
	}

	pc := portal.NewPortalClient(auth)
	if err := pc.Login(ctx); err != nil {
		return nil, err
	}
	return pc, nil
}

var loggedInPortalClient = &portalClientCache{
	ttl:  portalClientTTL,
	now:  time.Now,
	load: loadLoggedInPortalClient,
}

func textResult(text string) *mcp.CallToolResult {
	return &mcp.CallToolResult{
		Content: []mcp.Content{
			&mcp.TextContent{Text: text},
		},
	}
}

func formatListArticlesContent(articles []*portal.ArticleHeading) string {
	if len(articles) == 0 {
		return "Retrieved 0 articles."
	}

	var b strings.Builder
	fmt.Fprintf(&b, "Retrieved %d articles.\n\n", len(articles))

	limit := min(len(articles), maxArticleListContentItems)
	for i, article := range articles[:limit] {
		fmt.Fprintf(&b, "%d. [%s] %s", i+1, article.ArticleID, article.Title)
		if !article.PublishStart.IsZero() {
			fmt.Fprintf(&b, " (%s)", article.PublishStart.Format("2006-01-02 15:04"))
		}
		if article.Author != "" {
			fmt.Fprintf(&b, " - %s", article.Author)
		}
		b.WriteByte('\n')
	}
	if len(articles) > limit {
		fmt.Fprintf(&b, "\nShowing the first %d articles. StructuredContent contains the full result.", limit)
	}

	return b.String()
}

func formatGetArticleContent(article *portal.Article) string {
	var b strings.Builder
	fmt.Fprintf(&b, "# %s\n\n", article.Title)
	fmt.Fprintf(&b, "article_id: %s\n", article.ArticleID)
	if article.Author != "" {
		fmt.Fprintf(&b, "author: %s\n", article.Author)
	}
	if !article.PublishStart.IsZero() {
		fmt.Fprintf(&b, "publish_start: %s\n", article.PublishStart.Format("2006-01-02 15:04:05"))
	}
	if !article.PublishEnd.IsZero() {
		fmt.Fprintf(&b, "publish_end: %s\n", article.PublishEnd.Format("2006-01-02 15:04:05"))
	}
	if article.Content != "" {
		fmt.Fprintf(&b, "\n%s", article.Content)
	}
	return b.String()
}

func ListArticles(ctx context.Context, request *mcp.CallToolRequest, input *portal.ListArticlesOptions) (*mcp.CallToolResult, *ListArticlesOutput, error) {
	pc, err := loggedInPortalClient.get(ctx)
	if err != nil {
		return nil, nil, err
	}

	articles, err := pc.ListArticles(ctx, input)
	if err != nil {
		return nil, nil, err
	}

	return textResult(formatListArticlesContent(articles)), &ListArticlesOutput{
		Articles: articles,
		Count:    len(articles),
	}, nil
}

func GetArticle(ctx context.Context, request *mcp.CallToolRequest, input *GetArticleInput) (*mcp.CallToolResult, *GetArticleOutput, error) {
	if input == nil {
		input = &GetArticleInput{}
	}

	pc, err := loggedInPortalClient.get(ctx)
	if err != nil {
		return nil, nil, err
	}

	article, err := pc.GetArticle(ctx, input.ArticleID, &portal.GetArticleOptions{History: input.History})
	if err != nil {
		return nil, nil, err
	}

	return textResult(formatGetArticleContent(article)), &GetArticleOutput{Article: article}, nil
}
