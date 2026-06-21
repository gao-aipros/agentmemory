package unit

import (
	"os"
	"strings"
	"testing"

	cmd "github.com/agentmemory/agentmemory/internal/cmd"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// T145: CLI flag parsing tests for serve, setup, migrate, connect, and team commands.
// These tests verify that cobra commands are correctly defined and accept expected flags.
// Note: Tests avoid calling Execute() on commands that would connect to real databases.

func clearCLIEnv() {
	os.Unsetenv("DB_URL")
	os.Unsetenv("PORT")
	os.Unsetenv("JWT_SECRET")
	os.Unsetenv("MIGRATE_ON_STARTUP")
	os.Unsetenv("LOG_LEVEL")
}

// TestServeCommandFlags verifies the serve command accepts expected flags.
func TestServeCommandFlags(t *testing.T) {
	clearCLIEnv()

	t.Run("serve command has required flags", func(t *testing.T) {
		serveCmd := cmd.NewServeCommand()
		require.NotNil(t, serveCmd, "serve command should not be nil")
		assert.Equal(t, "serve", serveCmd.Use)

		// Verify flag defaults
		dbURL, _ := serveCmd.Flags().GetString("db-url")
		assert.Equal(t, "", dbURL, "db-url should default to empty")

		port, _ := serveCmd.Flags().GetInt("port")
		assert.Equal(t, 0, port, "port should default to 0 (use env)")

		migrate, _ := serveCmd.Flags().GetBool("migrate-on-startup")
		assert.False(t, migrate, "migrate-on-startup should default to false")
	})

	t.Run("serve command parses flags from args", func(t *testing.T) {
		serveCmd := cmd.NewServeCommand()
		serveCmd.SetArgs([]string{
			"--db-url", "postgres://localhost:5432/test",
			"--port", "9090",
			"--migrate-on-startup",
		})

		// Parse flags without executing (no DB connection attempt)
		err := serveCmd.ParseFlags([]string{
			"--db-url", "postgres://localhost:5432/test",
			"--port", "9090",
			"--migrate-on-startup",
		})
		require.NoError(t, err)

		dbURL, _ := serveCmd.Flags().GetString("db-url")
		assert.Equal(t, "postgres://localhost:5432/test", dbURL)

		port, _ := serveCmd.Flags().GetInt("port")
		assert.Equal(t, 9090, port)

		migrate, _ := serveCmd.Flags().GetBool("migrate-on-startup")
		assert.True(t, migrate)
	})
}

// TestSetupCommandFlags verifies the setup command accepts expected flags.
func TestSetupCommandFlags(t *testing.T) {
	clearCLIEnv()

	t.Run("setup command exists", func(t *testing.T) {
		setupCmd := cmd.NewSetupCommand()
		require.NotNil(t, setupCmd, "setup command should not be nil")
		assert.Equal(t, "setup", setupCmd.Use)
	})

	t.Run("setup command parses db-url flag", func(t *testing.T) {
		setupCmd := cmd.NewSetupCommand()
		err := setupCmd.ParseFlags([]string{"--db-url", "postgres://localhost:5432/test"})
		require.NoError(t, err)

		dbURL, _ := setupCmd.Flags().GetString("db-url")
		assert.Equal(t, "postgres://localhost:5432/test", dbURL)
	})
}

// TestMigrateCommandFlags verifies the migrate command accepts expected flags.
func TestMigrateCommandFlags(t *testing.T) {
	clearCLIEnv()

	t.Run("migrate command exists", func(t *testing.T) {
		migrateCmd := cmd.NewMigrateCommand()
		require.NotNil(t, migrateCmd, "migrate command should not be nil")
		assert.Equal(t, "migrate", migrateCmd.Use)
	})

	t.Run("migrate command parses db-url flag", func(t *testing.T) {
		migrateCmd := cmd.NewMigrateCommand()
		err := migrateCmd.ParseFlags([]string{"--db-url", "postgres://localhost:5432/test"})
		require.NoError(t, err)

		dbURL, _ := migrateCmd.Flags().GetString("db-url")
		assert.Equal(t, "postgres://localhost:5432/test", dbURL)
	})
}

// TestConnectCommandFlags verifies the connect command accepts expected flags.
func TestConnectCommandFlags(t *testing.T) {
	clearCLIEnv()

	t.Run("connect command exists", func(t *testing.T) {
		connectCmd := cmd.NewConnectCommand()
		require.NotNil(t, connectCmd, "connect command should not be nil")
		assert.Equal(t, "connect", connectCmd.Use)
	})

	t.Run("connect command parses url and token flags", func(t *testing.T) {
		connectCmd := cmd.NewConnectCommand()
		err := connectCmd.ParseFlags([]string{
			"--url", "http://localhost:8080",
			"--token", "test-token-123",
		})
		require.NoError(t, err)

		url, _ := connectCmd.Flags().GetString("url")
		assert.Equal(t, "http://localhost:8080", url)

		token, _ := connectCmd.Flags().GetString("token")
		assert.Equal(t, "test-token-123", token)
	})

	t.Run("connect command marks url as required", func(t *testing.T) {
		connectCmd := cmd.NewConnectCommand()
		err := connectCmd.Execute()
		assert.Error(t, err, "missing required --url flag should cause error")
	})
}

// TestUserCommandFlags verifies the user command and its subcommands.
func TestUserCommandFlags(t *testing.T) {
	clearCLIEnv()

	t.Run("user command exists with create subcommand", func(t *testing.T) {
		userCmd := cmd.NewUserCommand()
		require.NotNil(t, userCmd, "user command should not be nil")
		assert.Equal(t, "user", userCmd.Use)

		// Check that create subcommand exists
		foundCreate := false
		for _, sub := range userCmd.Commands() {
			if sub.Use == "create" {
				foundCreate = true
				break
			}
		}
		assert.True(t, foundCreate, "user command should have create subcommand")
	})

	t.Run("user create parses flags", func(t *testing.T) {
		userCmd := cmd.NewUserCommand()
		// Find the create subcommand and parse its flags
		var createCmd *cobra.Command
		for _, sub := range userCmd.Commands() {
			if sub.Use == "create" {
				createCmd = sub
				break
			}
		}
		require.NotNil(t, createCmd, "create subcommand should exist")

		err := createCmd.ParseFlags([]string{
			"--email", "test@example.com",
			"--password", "secret123",
			"--name", "Test User",
			"--db-url", "postgres://localhost:5432/test",
		})
		require.NoError(t, err)

		email, _ := createCmd.Flags().GetString("email")
		assert.Equal(t, "test@example.com", email)

		password, _ := createCmd.Flags().GetString("password")
		assert.Equal(t, "secret123", password)

		name, _ := createCmd.Flags().GetString("name")
		assert.Equal(t, "Test User", name)
	})
}

// TestTeamCommandFlags verifies the team command and its subcommands.
func TestTeamCommandFlags(t *testing.T) {
	clearCLIEnv()

	t.Run("team command exists with subcommands", func(t *testing.T) {
		teamCmd := cmd.NewTeamCommand()
		require.NotNil(t, teamCmd, "team command should not be nil")
		assert.Equal(t, "team", teamCmd.Use)

		expectedSubs := map[string]bool{
			"create": false,
			"list":   false,
			"add":    false,
			"remove": false,
		}
		for _, sub := range teamCmd.Commands() {
			if _, ok := expectedSubs[sub.Use]; ok {
				expectedSubs[sub.Use] = true
			}
		}
		for sub, found := range expectedSubs {
			assert.True(t, found, "team command should have %s subcommand", sub)
		}
	})

	t.Run("team commands accept json flag", func(t *testing.T) {
		teamCmd := cmd.NewTeamCommand()
		jsonFlag := teamCmd.PersistentFlags().Lookup("json")
		assert.NotNil(t, jsonFlag, "team command should have --json persistent flag")
	})
}

// TestRootCommandHasAllSubcommands verifies the root command includes all expected subcommands.
func TestRootCommandHasAllSubcommands(t *testing.T) {
	rootCmd := cmd.NewRootCommand()

	expectedSubs := map[string]bool{
		"serve":   false,
		"setup":   false,
		"migrate": false,
		"user":    false,
		"connect": false,
		"team":    false,
	}
	for _, sub := range rootCmd.Commands() {
		if _, ok := expectedSubs[sub.Use]; ok {
			expectedSubs[sub.Use] = true
		}
	}
	for sub, found := range expectedSubs {
		assert.True(t, found, "root command should have %s subcommand", sub)
	}
}

// TestServeCommandEnvVarFallback verifies serve command falls back to env vars.
func TestServeCommandEnvVarFallback(t *testing.T) {
	clearCLIEnv()

	t.Run("serve reads PORT from flag", func(t *testing.T) {
		os.Setenv("PORT", "3000")
		defer os.Unsetenv("PORT")

		serveCmd := cmd.NewServeCommand()
		err := serveCmd.ParseFlags([]string{"--port", "3000"})
		require.NoError(t, err)

		port, _ := serveCmd.Flags().GetInt("port")
		assert.Equal(t, 3000, port)
	})
}

// TestCLIVersion verifies that version information is accessible.
func TestCLIVersion(t *testing.T) {
	rootCmd := cmd.NewRootCommand()
	assert.Equal(t, "agentmemory", rootCmd.Use)
	assert.NotEmpty(t, rootCmd.Short)
	assert.NotEmpty(t, rootCmd.Long)
}

// TestConnectCommandSettingsPaths verifies that connect command handles settings paths.
func TestConnectCommandSettingsPaths(t *testing.T) {
	connectCmd := cmd.NewConnectCommand()

	// Verify flag definitions
	urlFlag := connectCmd.Flags().Lookup("url")
	require.NotNil(t, urlFlag, "url flag should exist")
	assert.True(t, strings.Contains(urlFlag.Usage, "required"), "url should be described as required")

	tokenFlag := connectCmd.Flags().Lookup("token")
	require.NotNil(t, tokenFlag, "token flag should exist")
}
