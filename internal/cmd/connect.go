package cmd

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"
)

// MCPConfig represents the MCP server configuration for a host agent's settings file.
type MCPConfig struct {
	MCPServers map[string]MCPServerConfig `json:"mcpServers"`
}

// MCPServerConfig represents a single MCP server entry.
type MCPServerConfig struct {
	Type    string            `json:"type"`
	URL     string            `json:"url"`
	Headers map[string]string `json:"headers,omitempty"`
}

// NewConnectCommand creates the `agentmemory connect` command.
func NewConnectCommand() *cobra.Command {
	var (
		serverURL string
		token     string
	)

	cmd := &cobra.Command{
		Use:   "connect",
		Short: "Configure a host coding agent to use AgentMemory",
		Long: `Configure a host coding agent (Claude Code, Codex) to connect to AgentMemory.

Writes the MCP server configuration to the host agent's settings file.
Supported agents:
  - Claude Code: ~/.claude/settings.json (or project .claude/settings.json)
  - Codex:       ~/.codex/settings.json

The --url and --token flags are required. The token should be a session token (st_ prefix).`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if serverURL == "" {
				return fmt.Errorf("--url is required (e.g., http://localhost:8080)")
			}
			if token == "" {
				token = os.Getenv("AGENTMEMORY_TOKEN")
			}
			if token == "" {
				return fmt.Errorf("--token is required (or set AGENTMEMORY_TOKEN env var)")
			}

			// Build MCP config
			mcpConfig := MCPConfig{
				MCPServers: map[string]MCPServerConfig{
					"agentmemory": {
						Type: "http",
						URL:  serverURL + "/v1/mcp",
						Headers: map[string]string{
							"Authorization": "Bearer " + token,
						},
					},
				},
			}

			// Write to Claude Code settings
			homeDir, err := os.UserHomeDir()
			if err != nil {
				return fmt.Errorf("failed to determine home directory: %w", err)
			}

			configWritten := false

			// Check for Claude Code
			claudeSettingsPath := filepath.Join(homeDir, ".claude", "settings.json")
			if err := writeMCPConfig(claudeSettingsPath, mcpConfig); err != nil {
				slog.Debug("failed to write Claude Code config", "path", claudeSettingsPath, "error", err)
			} else {
				fmt.Printf("Claude Code config written: %s\n", claudeSettingsPath)
				configWritten = true
			}

			// Check for Codex
			codexDir := filepath.Join(homeDir, ".codex")
			if _, err := os.Stat(codexDir); err == nil {
				codexSettingsPath := filepath.Join(codexDir, "settings.json")
				if err := writeMCPConfig(codexSettingsPath, mcpConfig); err != nil {
					slog.Debug("failed to write Codex config", "path", codexSettingsPath, "error", err)
				} else {
					fmt.Printf("Codex config written: %s\n", codexSettingsPath)
					configWritten = true
				}
			}

			// Also write to project-level .claude/settings.json if in a project directory
			cwd, _ := os.Getwd()
			projectClaudePath := filepath.Join(cwd, ".claude", "settings.json")
			if _, err := os.Stat(filepath.Join(cwd, ".claude")); err == nil {
				if err := writeMCPConfig(projectClaudePath, mcpConfig); err != nil {
					slog.Debug("failed to write project Claude Code config", "path", projectClaudePath, "error", err)
				} else {
					fmt.Printf("Project Claude Code config written: %s\n", projectClaudePath)
					configWritten = true
				}
			}

			if !configWritten {
				// Fallback: always write to ~/.claude/settings.json even if it doesn't exist yet
				os.MkdirAll(filepath.Join(homeDir, ".claude"), 0755)
				if err := writeMCPConfig(claudeSettingsPath, mcpConfig); err != nil {
					return fmt.Errorf("failed to write config: %w", err)
				}
				fmt.Printf("Claude Code config written: %s\n", claudeSettingsPath)
			}

			fmt.Println("\nConnect complete. Restart your coding agent to use AgentMemory.")
			return nil
		},
	}

	cmd.Flags().StringVar(&serverURL, "url", "", "AgentMemory server URL (required, e.g., http://localhost:8080)")
	cmd.Flags().StringVar(&token, "token", "", "Session token (st_...) or set AGENTMEMORY_TOKEN")
	cmd.MarkFlagRequired("url")

	return cmd
}

// writeMCPConfig reads an existing settings file, merges the MCP config, and writes it back.
func writeMCPConfig(path string, newConfig MCPConfig) error {
	// Read existing config if it exists
	var existing map[string]interface{}
	data, err := os.ReadFile(path)
	if err == nil {
		if err := json.Unmarshal(data, &existing); err != nil {
			// If file exists but can't be parsed, start fresh
			existing = make(map[string]interface{})
		}
	} else {
		existing = make(map[string]interface{})
	}

	// Merge the MCP servers config
	if existingMCPServers, ok := existing["mcpServers"].(map[string]interface{}); ok {
		// Merge into existing mcpServers
		for k, v := range newConfig.MCPServers {
			existingMCPServers[k] = v
		}
		existing["mcpServers"] = existingMCPServers
	} else {
		existing["mcpServers"] = newConfig.MCPServers
	}

	// Ensure directory exists
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create directory %s: %w", dir, err)
	}

	// Write the merged config
	out, err := json.MarshalIndent(existing, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal config: %w", err)
	}

	if err := os.WriteFile(path, out, 0644); err != nil {
		return fmt.Errorf("failed to write config: %w", err)
	}

	return nil
}
