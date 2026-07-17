package tools

import (
	"context"
	"fmt"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/yz4230/uec-portal-mcp/pkg/portal"
)

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

type GetArticleInput struct {
	ArticleID string `json:"article_id" jsonschema:"Article ID returned by list_articles"`
	History   bool   `json:"history,omitempty" jsonschema:"Set true when retrieving an article ID from search/history results"`
}

type GetArticleOutput struct {
	Article *portal.Article `json:"article" jsonschema:"Full article detail including title, body, author, and publication dates"`
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

func (h *Handler) GetArticle(ctx context.Context, request *mcp.CallToolRequest, input *GetArticleInput) (*mcp.CallToolResult, *GetArticleOutput, error) {
	if input == nil {
		input = &GetArticleInput{}
	}

	article, err := h.portalClient.GetArticle(ctx, input.ArticleID, &portal.GetArticleOptions{History: input.History})
	if err != nil {
		return nil, nil, err
	}

	return textResult(formatGetArticleContent(article)), &GetArticleOutput{Article: article}, nil
}
