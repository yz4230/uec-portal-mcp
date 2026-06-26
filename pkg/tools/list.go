package tools

import (
	"context"
	"fmt"
	"strings"
	"sync"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/yz4230/uec-portal-mcp/pkg/portal"
)

const maxArticleListContentItems = 20

var ListArticlesTool = &mcp.Tool{
	Name:        "list_articles",
	Title:       "List Portal Articles",
	Description: "List UEC Portal bulletin board articles. Supports pagination and keyword search.",
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
	Description: "Get a UEC Portal bulletin board article by article_id returned from list_articles.",
	Annotations: &mcp.ToolAnnotations{
		ReadOnlyHint:    true,
		DestructiveHint: new(false),
		IdempotentHint:  true,
		OpenWorldHint:   new(true),
	},
}

type ListArticlesOutput struct {
	Articles []*portal.ArticleHeading `json:"articles" jsonschema:"Article headings returned from the UEC Portal bulletin board"`
	Count    int                      `json:"count" jsonschema:"Number of articles returned"`
}

type GetArticleInput struct {
	ArticleID string `json:"article_id" jsonschema:"Article ID returned by list_articles"`
	History   bool   `json:"history,omitempty" jsonschema:"Set true when retrieving an article from history/search results"`
}

type GetArticleOutput struct {
	Article *portal.Article `json:"article" jsonschema:"Article detail"`
}

var loggedInPortalClient = sync.OnceValues(func() (*portal.PortalClient, error) {
	auth, err := portal.LoadAuthConfig()
	if err != nil {
		return nil, err
	}

	pc := portal.NewPortalClient(auth)
	if err := pc.Login(context.Background()); err != nil {
		return nil, err
	}
	return pc, nil
})

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
		fmt.Fprintf(&b, "%d. [%s] %s", i+1, article.ArticleID, strings.TrimSpace(article.Title))
		if !article.PublishStart.IsZero() {
			fmt.Fprintf(&b, " (%s)", article.PublishStart.Format("2006-01-02 15:04"))
		}
		if strings.TrimSpace(article.Author) != "" {
			fmt.Fprintf(&b, " - %s", strings.TrimSpace(article.Author))
		}
		b.WriteByte('\n')
	}
	if len(articles) > limit {
		fmt.Fprintf(&b, "\nShowing the first %d articles. StructuredContent contains the full result.", limit)
	}

	return strings.TrimSpace(b.String())
}

func formatGetArticleContent(article *portal.Article) string {
	var b strings.Builder
	fmt.Fprintf(&b, "# %s\n\n", strings.TrimSpace(article.Title))
	fmt.Fprintf(&b, "article_id: %s\n", article.ArticleID)
	if strings.TrimSpace(article.Author) != "" {
		fmt.Fprintf(&b, "author: %s\n", strings.TrimSpace(article.Author))
	}
	if !article.PublishStart.IsZero() {
		fmt.Fprintf(&b, "publish_start: %s\n", article.PublishStart.Format("2006-01-02 15:04:05"))
	}
	if !article.PublishEnd.IsZero() {
		fmt.Fprintf(&b, "publish_end: %s\n", article.PublishEnd.Format("2006-01-02 15:04:05"))
	}
	if strings.TrimSpace(article.Content) != "" {
		fmt.Fprintf(&b, "\n%s", strings.TrimSpace(article.Content))
	}
	return strings.TrimSpace(b.String())
}

func ListArticles(ctx context.Context, request *mcp.CallToolRequest, input *portal.ListArticlesOptions) (*mcp.CallToolResult, *ListArticlesOutput, error) {
	pc, err := loggedInPortalClient()
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

	pc, err := loggedInPortalClient()
	if err != nil {
		return nil, nil, err
	}

	article, err := pc.GetArticle(ctx, input.ArticleID, &portal.GetArticleOptions{History: input.History})
	if err != nil {
		return nil, nil, err
	}

	return textResult(formatGetArticleContent(article)), &GetArticleOutput{Article: article}, nil
}
