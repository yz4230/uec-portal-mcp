package portal

import (
	"strconv"
	"testing"
	"time"
)

func TestBuildListArticlesForm_DefaultsToCurrentList(t *testing.T) {
	got, err := buildListArticlesForm(nil)
	if err != nil {
		t.Fatalf("buildListArticlesForm() error = %v", err)
	}

	if got.Get("method") != "getNoticeList" {
		t.Fatalf("method = %q, want getNoticeList", got.Get("method"))
	}
	if got.Get("type") != "99" {
		t.Fatalf("type = %q, want 99", got.Get("type"))
	}
	if got.Get("list") != "1" {
		t.Fatalf("list = %q, want 1", got.Get("list"))
	}
	if got.Has("keyword") || got.Has("page") || got.Has("history") {
		t.Fatalf("default form unexpectedly contains search parameters: %v", got)
	}
}

func TestBuildListArticlesForm_SearchOptions(t *testing.T) {
	got, err := buildListArticlesForm(&ListArticlesOptions{
		Page:     2,
		Keyword:  "履修",
		Year:     2026,
		Category: "010",
		Type:     "99",
	})
	if err != nil {
		t.Fatalf("buildListArticlesForm() error = %v", err)
	}

	for key, want := range map[string]string{
		"method":  "getNoticeList",
		"type":    "99",
		"cate":    "010",
		"gadget":  "0",
		"history": "1",
		"keyword": "履修",
		"year":    "2026",
		"page":    "2",
		"list":    "2",
	} {
		if got.Get(key) != want {
			t.Fatalf("%s = %q, want %q", key, got.Get(key), want)
		}
	}
}

func TestBuildListArticlesForm_KeywordUsesCurrentYearAndFirstPage(t *testing.T) {
	got, err := buildListArticlesForm(&ListArticlesOptions{Keyword: "履修"})
	if err != nil {
		t.Fatalf("buildListArticlesForm() error = %v", err)
	}

	if got.Get("page") != "1" {
		t.Fatalf("page = %q, want 1", got.Get("page"))
	}
	if got.Get("year") != strconv.Itoa(time.Now().Year()) {
		t.Fatalf("year = %q, want current year", got.Get("year"))
	}
}
