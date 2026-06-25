package tools_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/yz4230/uec-portal-mcp/pkg/tools"
)

func TestMCPServer_ListAndGetArticlesTools(t *testing.T) {
	server := mcp.NewServer(&mcp.Implementation{
		Name:    "uec-portal-mcp-server-test",
		Version: "0.1.0",
	}, nil)
	mcp.AddTool(server, tools.ListArticlesTool, tools.ListArticles)
	mcp.AddTool(server, tools.GetArticleTool, tools.GetArticle)

	handler := mcp.NewStreamableHTTPHandler(func(r *http.Request) *mcp.Server {
		return server
	}, nil)
	httpServer := httptest.NewServer(handler)
	t.Cleanup(httpServer.Close)

	ctx := context.Background()
	client := mcp.NewClient(&mcp.Implementation{
		Name:    "uec-portal-mcp-client-test",
		Version: "0.1.0",
	}, nil)
	session, err := client.Connect(ctx, &mcp.StreamableClientTransport{Endpoint: httpServer.URL}, nil)
	if err != nil {
		t.Fatalf("connect MCP client: %v", err)
	}
	t.Cleanup(func() {
		_ = session.Close()
	})

	toolsResult, err := session.ListTools(ctx, nil)
	if err != nil {
		t.Fatalf("list tools: %v", err)
	}
	toolNames := map[string]bool{}
	for _, tool := range toolsResult.Tools {
		toolNames[tool.Name] = true
	}
	for _, name := range []string{"list_articles", "get_article"} {
		if !toolNames[name] {
			t.Fatalf("tool %q was not registered; got %v", name, toolNames)
		}
	}

	listResult, err := session.CallTool(ctx, &mcp.CallToolParams{
		Name:      "list_articles",
		Arguments: map[string]any{},
	})
	if err != nil {
		t.Fatalf("call list_articles: %v", err)
	}
	if listResult.IsError {
		t.Fatalf("list_articles returned MCP tool error: %#v", listResult.Content)
	}
	var listOutput tools.ListArticlesOutput
	decodeStructuredContent(t, listResult.StructuredContent, &listOutput)
	if listOutput.Count == 0 || len(listOutput.Articles) == 0 {
		t.Fatal("list_articles returned no articles")
	}
	firstArticle := listOutput.Articles[0]
	if firstArticle.ArticleID == "" {
		t.Fatal("list_articles returned first article without article_id")
	}
	listText := textContent(t, listResult)
	if !strings.Contains(listText, "Retrieved") || !strings.Contains(listText, firstArticle.ArticleID) {
		t.Fatalf("list_articles content = %q, want summary containing first article ID %q", listText, firstArticle.ArticleID)
	}

	getResult, err := session.CallTool(ctx, &mcp.CallToolParams{
		Name: "get_article",
		Arguments: map[string]any{
			"article_id": firstArticle.ArticleID,
		},
	})
	if err != nil {
		t.Fatalf("call get_article: %v", err)
	}
	if getResult.IsError {
		t.Fatalf("get_article returned MCP tool error: %#v", getResult.Content)
	}
	var getOutput tools.GetArticleOutput
	decodeStructuredContent(t, getResult.StructuredContent, &getOutput)
	if getOutput.Article == nil {
		t.Fatal("get_article returned nil article")
	}
	if getOutput.Article.ArticleID != firstArticle.ArticleID {
		t.Fatalf("get_article article_id = %q, want %q", getOutput.Article.ArticleID, firstArticle.ArticleID)
	}
	if getOutput.Article.Content == "" {
		t.Fatal("get_article returned empty content")
	}
	getText := textContent(t, getResult)
	if !strings.Contains(getText, getOutput.Article.Title) || !strings.Contains(getText, leadingRunes(getOutput.Article.Content, 20)) {
		t.Fatal("get_article content does not include article title and body")
	}
}

func decodeStructuredContent(t *testing.T, content any, target any) {
	t.Helper()

	data, err := json.Marshal(content)
	if err != nil {
		t.Fatalf("marshal structured content: %v", err)
	}
	if err := json.Unmarshal(data, target); err != nil {
		t.Fatalf("unmarshal structured content: %v", err)
	}
}

func textContent(t *testing.T, result *mcp.CallToolResult) string {
	t.Helper()

	if len(result.Content) == 0 {
		t.Fatal("tool result returned no content")
	}
	text, ok := result.Content[0].(*mcp.TextContent)
	if !ok {
		t.Fatalf("tool result content type = %T, want *mcp.TextContent", result.Content[0])
	}
	return text.Text
}

func leadingRunes(text string, count int) string {
	runes := []rune(strings.TrimSpace(text))
	if len(runes) <= count {
		return string(runes)
	}
	return string(runes[:count])
}
