package cmd

import (
	"errors"
	"log/slog"
	"net/http"
	"os"
	"slices"
	"strings"

	"github.com/yz4230/mcpauth"
)

// Environment variables configuring OAuth for the HTTP server. OAuth is on
// when MCP_PASSWORD and MCP_PUBLIC_URL are both set.
const (
	passwordEnv     = "MCP_PASSWORD"
	publicURLEnv    = "MCP_PUBLIC_URL"
	stateFileEnv    = "MCP_STATE_FILE"
	redirectURIsEnv = "MCP_REDIRECT_URIS"
)

// withAuth protects h with mcpauth as configured by the environment, or
// returns h unchanged when neither MCP_PASSWORD nor MCP_PUBLIC_URL is set.
func withAuth(h http.Handler) (http.Handler, error) {
	password, publicURL := os.Getenv(passwordEnv), os.Getenv(publicURLEnv)
	if password == "" && publicURL == "" {
		slog.Warn("serving without authentication; set " + passwordEnv + " and " + publicURLEnv + " to enable OAuth")
		return h, nil
	}
	if password == "" || publicURL == "" {
		return nil, errors.New(passwordEnv + " and " + publicURLEnv + " must be set together")
	}
	auth, err := mcpauth.New(mcpauth.Config{
		PublicURL:           publicURL,
		Password:            password,
		StateFile:           os.Getenv(stateFileEnv),
		AllowedRedirectURIs: append(slices.Clone(mcpauth.DefaultRedirectURIs), splitList(os.Getenv(redirectURIsEnv))...),
	})
	if err != nil {
		return nil, err
	}
	return auth.Middleware(h), nil
}

// splitList splits a comma-separated list, dropping empty items.
func splitList(s string) []string {
	var list []string
	for item := range strings.SplitSeq(s, ",") {
		if item = strings.TrimSpace(item); item != "" {
			list = append(list, item)
		}
	}
	return list
}
