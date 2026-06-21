package handler

import (
	"log/slog"
	"net/http"

	"github.com/agentmemory/agentmemory/internal/mcp"
	"github.com/jackc/pgx/v5/pgxpool"
	sdkmcp "github.com/modelcontextprotocol/go-sdk/mcp"
)

// NewMCPHandler creates an HTTP handler for the Model Context Protocol endpoint.
// It sets up a StreamableHTTP handler at /v1/mcp that services MCP client connections.
//
// The getServer function creates a fresh MCP server for each new session,
// registering all agentmemory tools. This is stateless — each session gets
// a fully independent MCP server instance.
//
// Server info:
//   - name: "agentmemory-v2"
//   - version: "2.0.0"
func NewMCPHandler(pool *pgxpool.Pool) http.Handler {
	getServer := func(r *http.Request) *sdkmcp.Server {
		if pool == nil {
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

		// Register all agentmemory tools
		mcp.RegisterAllTools(mcpServer, pool)

		return mcpServer
	}

	// Create StreamableHTTP handler with JSON response mode
	handler := sdkmcp.NewStreamableHTTPHandler(getServer, &sdkmcp.StreamableHTTPOptions{
		JSONResponse: true,
		Logger:       slog.Default(),
	})

	return handler
}
