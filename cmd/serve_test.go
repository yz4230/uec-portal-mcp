package cmd

import (
	"context"
	"os/exec"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestChooseServeMode(t *testing.T) {
	tests := []struct {
		name    string
		stdin   bool
		http    bool
		want    serveMode
		wantErr bool
	}{
		{
			name: "default http",
			want: serveModeHTTP,
		},
		{
			name:  "stdin",
			stdin: true,
			want:  serveModeStdin,
		},
		{
			name: "http",
			http: true,
			want: serveModeHTTP,
		},
		{
			name:    "both",
			stdin:   true,
			http:    true,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := chooseServeMode(tt.stdin, tt.http)
			if tt.wantErr {
				if err == nil {
					t.Fatal("chooseServeMode() error = nil, want error")
				}
				return
			}
			if err != nil {
				t.Fatalf("chooseServeMode() error = %v", err)
			}
			if got != tt.want {
				t.Fatalf("chooseServeMode() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestServeStdioListsTools(t *testing.T) {
	ctx := context.Background()
	command := exec.Command("go", "run", ".", "serve", "--stdin")
	command.Dir = ".."

	client := mcp.NewClient(&mcp.Implementation{
		Name:    "uec-portal-mcp-stdio-test-client",
		Version: "0.1.0",
	}, nil)
	session, err := client.Connect(ctx, &mcp.CommandTransport{Command: command}, nil)
	if err != nil {
		t.Fatalf("connect stdio server: %v", err)
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
}
