package cmd

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"strconv"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/spf13/cobra"
	"github.com/yz4230/uec-portal-mcp/pkg/tools"
)

var serveFlags struct {
	port  int
	stdin bool
	http  bool
}

type serveMode string

const (
	serveModeHTTP  serveMode = "http"
	serveModeStdin serveMode = "stdin"

	defaultHTTPPort = 8080
	portEnvName     = "PORT"
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

func validateHTTPPort(port int) error {
	if port < 1 || port > 65535 {
		return fmt.Errorf("port must be a number between 1 and 65535, got %d", port)
	}
	return nil
}

func resolveHTTPPort(port int) (int, error) {
	portText := strings.TrimSpace(os.Getenv(portEnvName))
	if portText == "" {
		return port, validateHTTPPort(port)
	}

	envPort, err := strconv.Atoi(portText)
	if err != nil {
		return 0, fmt.Errorf("%s must be a port number between 1 and 65535, got %q", portEnvName, portText)
	}

	if err := validateHTTPPort(envPort); err != nil {
		return 0, fmt.Errorf("%s must be a port number between 1 and 65535, got %q", portEnvName, portText)
	}

	return envPort, nil
}

func runMCPHTTP(ctx context.Context, server *mcp.Server, port int) error {
	handler := mcp.NewStreamableHTTPHandler(func(r *http.Request) *mcp.Server { return server }, nil)
	httpServer := &http.Server{
		Addr:    fmt.Sprintf(":%d", port),
		Handler: handler,
	}

	go func() {
		<-ctx.Done()
		if err := httpServer.Shutdown(context.Background()); err != nil {
			slog.Warn("Failed to shut down MCP HTTP server", "error", err)
		}
	}()

	slog.Info("Starting MCP HTTP server", "port", port)
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
			port, err := resolveHTTPPort(serveFlags.port)
			if err != nil {
				return err
			}
			return runMCPHTTP(cmd.Context(), server, port)
		default:
			return fmt.Errorf("unknown serve mode %q", mode)
		}
	},
}

func init() {
	rootCmd.AddCommand(serveCmd)

	serveCmd.Flags().BoolVar(&serveFlags.stdin, "stdin", false, "Serve MCP over stdin/stdout")
	serveCmd.Flags().BoolVar(&serveFlags.http, "http", false, "Serve MCP over streamable HTTP")
	serveCmd.Flags().IntVarP(&serveFlags.port, "port", "p", defaultHTTPPort, "Port to listen on in --http mode")
}
