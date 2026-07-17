package portal_test

import (
	"context"
	"testing"

	"github.com/yz4230/uec-portal-mcp/pkg/portal"
	"github.com/yz4230/uec-portal-mcp/pkg/uecauth"
)

func TestPortalClient_Login(t *testing.T) {
	credentials, err := uecauth.LoadCredentialsFromEnv()
	if err != nil {
		t.Fatalf("Failed to load auth config: %v", err)
	}
	authenticator, err := uecauth.New(credentials)
	if err != nil {
		t.Fatalf("Create authenticator: %v", err)
	}
	if _, err := authenticator.Authenticate(context.Background()); err != nil {
		t.Fatalf("Login failed: %v", err)
	}
}

func TestPortalClient_ListAndGetArticles(t *testing.T) {
	credentials, err := uecauth.LoadCredentialsFromEnv()
	if err != nil {
		t.Fatalf("Failed to load auth config: %v", err)
	}
	ctx := context.Background()
	authenticator, err := uecauth.New(credentials)
	if err != nil {
		t.Fatalf("Create authenticator: %v", err)
	}
	session, err := authenticator.Authenticate(ctx)
	if err != nil {
		t.Fatalf("Login failed: %v", err)
	}
	pc := portal.NewPortalClient(session)

	articles, err := pc.ListArticles(ctx, nil)
	if err != nil {
		t.Fatalf("ListArticles failed: %v", err)
	}
	if len(articles) == 0 {
		t.Fatal("ListArticles returned no articles")
	}
	t.Logf("current list returned %d articles", len(articles))
	firstArticle := articles[0]
	if firstArticle.ArticleID == "" {
		t.Fatal("first article has empty ArticleID")
	}
	if firstArticle.Title == "" {
		t.Fatal("first article has empty Title")
	}

	searched, err := pc.ListArticles(ctx, &portal.ListArticlesOptions{
		Keyword: firstArticle.Title,
		Year:    firstArticle.PublishStart.Year(),
	})
	if err != nil {
		t.Fatalf("ListArticles search failed: %v", err)
	}
	if len(searched) == 0 {
		t.Fatalf("ListArticles search returned no articles for title %q", firstArticle.Title)
	}
	t.Logf("search for first title returned %d articles", len(searched))

	page2, err := pc.ListArticles(ctx, &portal.ListArticlesOptions{
		Page: 2,
		Year: firstArticle.PublishStart.Year(),
	})
	if err != nil {
		t.Fatalf("ListArticles page 2 failed: %v", err)
	}
	if len(page2) == 0 {
		t.Fatal("ListArticles page 2 returned no articles")
	}
	t.Logf("page 2 returned %d articles", len(page2))

	detail, err := pc.GetArticle(ctx, firstArticle.ArticleID, &portal.GetArticleOptions{History: true})
	if err != nil {
		t.Fatalf("GetArticle failed: %v", err)
	}
	t.Logf("detail returned article_id=%s title=%q content_bytes=%d", detail.ArticleID, detail.Title, len(detail.Content))
	if detail.ArticleID != firstArticle.ArticleID {
		t.Fatalf("detail ArticleID = %q, want %q", detail.ArticleID, firstArticle.ArticleID)
	}
	if detail.Title == "" {
		t.Fatal("detail has empty Title")
	}
	if detail.Content == "" {
		t.Fatal("detail has empty Content")
	}
}
