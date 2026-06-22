package handler

import (
	"log/slog"
	"net/http"

	"github.com/agentmemory/agentmemory/internal/mcp"
	sdkmcp "github.com/modelcontextprotocol/go-sdk/mcp"
)

// NewMCPHandler creates an HTTP handler for the Model Context Protocol endpoint.
// It sets up a StreamableHTTP handler at /v1/mcp that services MCP client connections.
//
// The getServer function creates a fresh MCP server for each new session,
// registering all agentmemory tools. The same ServiceBundle is shared across
// all sessions — services are stateless wrappers around the DB pool and LLM.
//
// Server info:
//   - name: "agentmemory-v2"
//   - version: "2.0.0"
func NewMCPHandler(bundle *mcp.ServiceBundle) http.Handler {
	getServer := func(r *http.Request) *sdkmcp.Server {
		if bundle == nil || bundle.Pool == nil {
			slog.Warn("MCP request received but database pool is nil")
			return nil
		}

		// Create a new MCP server for this session
		mcpServer := sdkmcp.NewServer(
			&sdkmcp.Implementation{
				Name:    "agentmemory-v2",
				Version: "2.0.0",
			},
			&sdkmcp.ServerOptions{
				Logger: slog.Default(),
			},
		)

		// Register all agentmemory tools using the shared ServiceBundle
		mcp.RegisterAllTools(mcpServer, bundle)

		return mcpServer
	}

	// Create StreamableHTTP handler with JSON response mode
	handler := sdkmcp.NewStreamableHTTPHandler(getServer, &sdkmcp.StreamableHTTPOptions{
		JSONResponse: true,
		Logger:       slog.Default(),
	})

	return handler
}
