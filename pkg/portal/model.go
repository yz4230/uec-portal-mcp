package portal

import "time"

type ArticleHeading struct {
	ArticleID    string    `json:"article_id"`
	Author       string    `json:"author"`
	Title        string    `json:"title"`
	Read         bool      `json:"read"`
	Category     string    `json:"category"`
	PublishStart time.Time `json:"publish_start"`
	PublishEnd   time.Time `json:"publish_end"`
}

type Article struct {
	ArticleID    string    `json:"article_id"`
	Author       string    `json:"author"`
	Title        string    `json:"title"`
	Content      string    `json:"content"`
	PublishStart time.Time `json:"publish_start"`
	PublishEnd   time.Time `json:"publish_end"`
}

type ListArticlesOptions struct {
	Page    int    `json:"page,omitempty" jsonschema:"1-based article list page number; defaults to 1"`
	Keyword string `json:"keyword,omitempty" jsonschema:"Search text for matching portal bulletin board articles"`
	Year    int    `json:"year,omitempty" jsonschema:"Publication year to filter by, such as 2026"`
}

type GetArticleOptions struct {
	History bool `json:"history,omitempty"`
}
