//go:build e2e

package main_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/yz4230/uec-portal-mcp/pkg/tools"
)

func TestE2EStdio(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	var stderr bytes.Buffer
	command := exec.Command(buildServer(t), "serve", "--stdin")
	command.Stderr = &stderr

	client := newMCPClient()
	session, err := client.Connect(ctx, &mcp.CommandTransport{Command: command}, nil)
	if err != nil {
		t.Fatalf("connect to stdio MCP server: %v\nserver stderr:\n%s", err, stderr.String())
	}
	t.Cleanup(func() {
		if err := session.Close(); err != nil {
			t.Errorf("close stdio MCP session: %v", err)
		}
	})

	exercisePortalTools(t, ctx, session)
}

func TestE2EHTTP(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	port := availablePort(t)
	var stderr bytes.Buffer
	command := exec.Command(buildServer(t), "serve", "--http", "--port", strconv.Itoa(port))
	command.Stderr = &stderr
	if err := command.Start(); err != nil {
		t.Fatalf("start HTTP MCP server: %v", err)
	}
	serverDone := make(chan error, 1)
	go func() { serverDone <- command.Wait() }()
	t.Cleanup(func() {
		if command.Process != nil {
			_ = command.Process.Signal(os.Interrupt)
		}
		select {
		case <-serverDone:
		case <-time.After(10 * time.Second):
			if command.Process != nil {
				_ = command.Process.Kill()
			}
			<-serverDone
		}
	})

	endpoint := fmt.Sprintf("http://127.0.0.1:%d", port)
	var session *mcp.ClientSession
	var err error
	for ctx.Err() == nil {
		select {
		case processErr := <-serverDone:
			t.Fatalf("HTTP MCP server exited before connection: %v\nserver stderr:\n%s", processErr, stderr.String())
		default:
		}

		session, err = newMCPClient().Connect(ctx, &mcp.StreamableClientTransport{Endpoint: endpoint}, nil)
		if err == nil {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	if err != nil {
		t.Fatalf("connect to HTTP MCP server: %v\nserver stderr:\n%s", err, stderr.String())
	}
	t.Cleanup(func() {
		if err := session.Close(); err != nil {
			t.Errorf("close HTTP MCP session: %v", err)
		}
	})

	exercisePortalTools(t, ctx, session)
}

func newMCPClient() *mcp.Client {
	return mcp.NewClient(&mcp.Implementation{
		Name:    "uec-portal-mcp-e2e-client",
		Version: "0.1.0",
	}, nil)
}

func exercisePortalTools(t *testing.T, ctx context.Context, session *mcp.ClientSession) {
	t.Helper()

	listed, err := session.ListTools(ctx, nil)
	if err != nil {
		t.Fatalf("list tools: %v", err)
	}
	names := make(map[string]bool, len(listed.Tools))
	for _, tool := range listed.Tools {
		names[tool.Name] = true
	}
	for _, want := range []string{"list_articles", "get_article"} {
		if !names[want] {
			t.Fatalf("tool %q is not registered; got %v", want, names)
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
		t.Fatalf("list_articles returned tool error: %s", resultText(listResult))
	}
	var listOutput tools.ListArticlesOutput
	decodeStructuredContent(t, listResult.StructuredContent, &listOutput)
	if listOutput.Count == 0 || len(listOutput.Articles) == 0 {
		t.Fatal("list_articles returned no articles")
	}
	first := listOutput.Articles[0]
	if first == nil || first.ArticleID == "" || first.Title == "" {
		t.Fatalf("list_articles returned invalid first article: %#v", first)
	}

	getResult, err := session.CallTool(ctx, &mcp.CallToolParams{
		Name: "get_article",
		Arguments: map[string]any{
			"article_id": first.ArticleID,
		},
	})
	if err != nil {
		t.Fatalf("call get_article: %v", err)
	}
	if getResult.IsError {
		t.Fatalf("get_article returned tool error: %s", resultText(getResult))
	}
	var getOutput tools.GetArticleOutput
	decodeStructuredContent(t, getResult.StructuredContent, &getOutput)
	if getOutput.Article == nil {
		t.Fatal("get_article returned no article")
	}
	if getOutput.Article.ArticleID != first.ArticleID {
		t.Fatalf("get_article article_id = %q, want %q", getOutput.Article.ArticleID, first.ArticleID)
	}
	if strings.TrimSpace(getOutput.Article.Content) == "" {
		t.Fatal("get_article returned empty content")
	}

	t.Logf("verified %d listed articles and article %s (%q)", listOutput.Count, first.ArticleID, first.Title)
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

func resultText(result *mcp.CallToolResult) string {
	var texts []string
	for _, content := range result.Content {
		if text, ok := content.(*mcp.TextContent); ok {
			texts = append(texts, text.Text)
		}
	}
	return strings.Join(texts, "\n")
}

func availablePort(t *testing.T) int {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("reserve HTTP port: %v", err)
	}
	defer listener.Close()
	return listener.Addr().(*net.TCPAddr).Port
}

func buildServer(t *testing.T) string {
	t.Helper()
	binary := t.TempDir() + "/uec-portal-mcp"
	command := exec.Command("go", "build", "-o", binary, ".")
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("build MCP server: %v\n%s", err, output)
	}
	return binary
}
