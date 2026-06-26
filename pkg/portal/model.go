package portal

import "time"

type ArticleHeading struct {
	ArticleID    string    `json:"article_id"`
	Author       string    `json:"author"`
	Title        string    `json:"title"`
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
	Page     int    `json:"page,omitempty"`
	Keyword  string `json:"keyword,omitempty"`
	Year     int    `json:"year,omitempty"`
	Category string `json:"category,omitempty"`
	Type     string `json:"type,omitempty"`
}

type GetArticleOptions struct {
	History bool `json:"history,omitempty"`
}
