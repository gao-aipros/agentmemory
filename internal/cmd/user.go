package cmd

import (
	"bufio"
	"context"
	"fmt"
	"log/slog"
	"os"
	"strings"
	"time"

	"github.com/agentmemory/agentmemory/internal/config"
	"github.com/agentmemory/agentmemory/internal/service"
	"github.com/spf13/cobra"
)

// NewUserCommand creates the `agentmemory user` command group.
func NewUserCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "user",
		Short: "Manage users",
		Long:  "Create and manage AgentMemory user accounts.",
	}

	cmd.AddCommand(newUserCreateCommand())

	return cmd
}

// newUserCreateCommand creates the `agentmemory user create` subcommand.
func newUserCreateCommand() *cobra.Command {
	var (
		dbURL    string
		email    string
		password string
		name     string
	)

	cmd := &cobra.Command{
		Use:   "create",
		Short: "Create a new user",
		Long: `Create a new AgentMemory user account.

The email, password, and name can be set via flags.
If --password is not provided, you will be prompted securely.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if dbURL == "" {
				dbURL = os.Getenv("DB_URL")
			}
			if dbURL == "" {
				return fmt.Errorf("DB_URL is required; set via --db-url flag or DB_URL environment variable")
			}

			if email == "" {
				return fmt.Errorf("--email is required")
			}
			if name == "" {
				return fmt.Errorf("--name is required")
			}
			if password == "" {
				// Prompt for password if not provided via flag
				pwd, err := promptPassword()
				if err != nil {
					return fmt.Errorf("failed to read password: %w", err)
				}
				if pwd == "" {
					return fmt.Errorf("password is required")
				}
				password = pwd
			}

			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()

			pool, err := config.NewPool(ctx, dbURL)
			if err != nil {
				return fmt.Errorf("failed to connect to database: %w", err)
			}
			defer config.ClosePool(pool)

			userSvc := service.NewUserService(pool)
			user, err := userSvc.CreateUser(ctx, email, password, name)
			if err != nil {
				slog.Error("failed to create user", "error", err)
				return fmt.Errorf("failed to create user: %w", err)
			}

			fmt.Printf("User created successfully.\n")
			fmt.Printf("  ID:    %s\n", user.ID)
			fmt.Printf("  Email: %s\n", user.Email)
			fmt.Printf("  Name:  %s\n", user.Name)

			return nil
		},
	}

	cmd.Flags().StringVar(&dbURL, "db-url", "", "Database URL (or set DB_URL env var)")
	cmd.Flags().StringVar(&email, "email", "", "User email")
	cmd.Flags().StringVar(&password, "password", "", "User password (will prompt if not provided)")
	cmd.Flags().StringVar(&name, "name", "", "User display name")

	return cmd
}

// promptPassword prompts the user for a password and reads the full line
// including internal spaces (unlike fmt.Scanln which truncates at whitespace).
func promptPassword() (string, error) {
	fmt.Print("Enter password: ")
	reader := bufio.NewReader(os.Stdin)
	input, err := reader.ReadString('\n')
	if err != nil {
		return "", fmt.Errorf("failed to read password: %w", err)
	}
	return strings.TrimRight(input, "\n"), nil
}
