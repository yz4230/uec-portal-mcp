package cmd

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/spf13/cobra"
	"github.com/yz4230/uec-portal-mcp/pkg/tools"
)

var serveFlags struct {
	addr  string
	stdin bool
	http  bool
}

type serveMode string

const (
	serveModeHTTP  serveMode = "http"
	serveModeStdin serveMode = "stdin"
)

func newMCPServer() *mcp.Server {
	server := mcp.NewServer(&mcp.Implementation{
		Title:   "UEC Portal MCP Server",
		Name:    "uec-portal-mcp-server",
		Version: "0.1.0",
	}, nil)

	mcp.AddTool(server, tools.ListArticlesTool, tools.ListArticles)
	mcp.AddTool(server, tools.GetArticleTool, tools.GetArticle)

	return server
}

func chooseServeMode(stdin, http bool) (serveMode, error) {
	if stdin && http {
		return "", fmt.Errorf("use either --stdin or --http, not both")
	}
	if stdin {
		return serveModeStdin, nil
	}
	return serveModeHTTP, nil
}

func runMCPHTTP(ctx context.Context, server *mcp.Server, addr string) error {
	handler := mcp.NewStreamableHTTPHandler(func(r *http.Request) *mcp.Server { return server }, nil)
	httpServer := &http.Server{
		Addr:    addr,
		Handler: handler,
	}

	go func() {
		<-ctx.Done()
		if err := httpServer.Shutdown(context.Background()); err != nil {
			slog.Warn("Failed to shut down MCP HTTP server", "error", err)
		}
	}()

	slog.Info("Starting MCP HTTP server", "addr", addr)
	if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		return err
	}
	return nil
}

// serveCmd represents the serve command
var serveCmd = &cobra.Command{
	Use:   "serve",
	Short: "Serve the MCP server",
	RunE: func(cmd *cobra.Command, args []string) error {
		mode, err := chooseServeMode(serveFlags.stdin, serveFlags.http)
		if err != nil {
			return err
		}

		server := newMCPServer()
		switch mode {
		case serveModeStdin:
			slog.Info("Starting MCP stdio server")
			return server.Run(cmd.Context(), &mcp.StdioTransport{})
		case serveModeHTTP:
			return runMCPHTTP(cmd.Context(), server, serveFlags.addr)
		default:
			return fmt.Errorf("unknown serve mode %q", mode)
		}
	},
}

func init() {
	rootCmd.AddCommand(serveCmd)

	serveCmd.Flags().BoolVar(&serveFlags.stdin, "stdin", false, "Serve MCP over stdin/stdout")
	serveCmd.Flags().BoolVar(&serveFlags.http, "http", false, "Serve MCP over streamable HTTP")
	serveCmd.Flags().StringVarP(&serveFlags.addr, "addr", "a", ":8080", "Address to listen on in --http mode")
}
