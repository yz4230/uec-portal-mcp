package tools

import (
	"context"
	"fmt"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/yz4230/uec-portal-mcp/pkg/portal"
)

const maxArticleListContentItems = 20

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

type ListArticlesOutput struct {
	Articles []*portal.ArticleHeading `json:"articles" jsonschema:"Article headings with article_id, title, author, category, read state, and publication dates"`
	Count    int                      `json:"count" jsonschema:"Number of article headings returned"`
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

func (h *Handler) ListArticles(ctx context.Context, request *mcp.CallToolRequest, input *portal.ListArticlesOptions) (*mcp.CallToolResult, *ListArticlesOutput, error) {
	articles, err := h.portalClient.ListArticles(ctx, input)
	if err != nil {
		return nil, nil, err
	}

	return textResult(formatListArticlesContent(articles)), &ListArticlesOutput{
		Articles: articles,
		Count:    len(articles),
	}, nil
}
