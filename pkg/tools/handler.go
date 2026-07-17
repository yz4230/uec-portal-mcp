package tools

import (
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/yz4230/uec-portal-mcp/pkg/portal"
)

type Handler struct {
	portalClient *portal.PortalClient
}

func NewHandler(portalClient *portal.PortalClient) *Handler {
	return &Handler{portalClient: portalClient}
}

func textResult(text string) *mcp.CallToolResult {
	return &mcp.CallToolResult{
		Content: []mcp.Content{
			&mcp.TextContent{Text: text},
		},
	}
}
