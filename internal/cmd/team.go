package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/agentmemory/agentmemory/internal/config"
	"github.com/agentmemory/agentmemory/internal/service"
	"github.com/spf13/cobra"
)

// NewTeamCommand creates the `agentmemory team` command group.
func NewTeamCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "team",
		Short: "Manage teams",
		Long:  "Create, list, and manage teams and team members.",
	}

	cmd.PersistentFlags().Bool("json", false, "Output as JSON")

	cmd.AddCommand(newTeamCreateCommand())
	cmd.AddCommand(newTeamListCommand())
	cmd.AddCommand(newTeamAddCommand())
	cmd.AddCommand(newTeamRemoveCommand())

	return cmd
}

// teamOutput writes output either as formatted table or JSON.
func teamOutput(cmd *cobra.Command, data interface{}, headers []string, rows [][]string) error {
	useJSON, _ := cmd.Flags().GetBool("json")
	if useJSON {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(data)
	}

	// Table output
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, strings.Join(headers, "\t"))
	for _, row := range rows {
		fmt.Fprintln(w, strings.Join(row, "\t"))
	}
	return w.Flush()
}

func newTeamCreateCommand() *cobra.Command {
	var (
		dbURL             string
		teamName          string
		ownerID           string
		defaultVisibility string
	)

	cmd := &cobra.Command{
		Use:   "create",
		Short: "Create a new team",
		RunE: func(cmd *cobra.Command, args []string) error {
			if dbURL == "" {
				dbURL = os.Getenv("DB_URL")
			}
			if dbURL == "" {
				return fmt.Errorf("DB_URL is required; set via --db-url flag or DB_URL environment variable")
			}
			if teamName == "" {
				return fmt.Errorf("--name is required")
			}
			if ownerID == "" {
				return fmt.Errorf("--owner-id is required")
			}
			if defaultVisibility == "" {
				defaultVisibility = "member_choice"
			}

			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()

			pool, err := config.NewPool(ctx, dbURL)
			if err != nil {
				return fmt.Errorf("failed to connect to database: %w", err)
			}
			defer config.ClosePool(pool)

			teamSvc := service.NewTeamService(pool)
			team, err := teamSvc.CreateTeam(ctx, teamName, ownerID, defaultVisibility)
			if err != nil {
				slog.Error("failed to create team", "error", err)
				return fmt.Errorf("failed to create team: %w", err)
			}

			return teamOutput(cmd, team,
				[]string{"ID", "NAME", "OWNER", "VISIBILITY"},
				[][]string{{team.ID, team.Name, team.OwnerID, team.DefaultVisibility}},
			)
		},
	}

	cmd.Flags().StringVar(&dbURL, "db-url", "", "Database URL")
	cmd.Flags().StringVar(&teamName, "name", "", "Team name")
	cmd.Flags().StringVar(&ownerID, "owner-id", "", "Owner user ID")

	cmd.Flags().StringVar(&defaultVisibility, "default-visibility", "member_choice", "Default visibility (member_choice, team, public)")

	return cmd
}

func newTeamListCommand() *cobra.Command {
	var (
		limit   int
		dbURL   string
		ownerID string
	)

	cmd := &cobra.Command{
		Use:   "list",
		Short: "List teams",
		RunE: func(cmd *cobra.Command, args []string) error {
			if dbURL == "" {
				dbURL = os.Getenv("DB_URL")
			}
			if dbURL == "" {
				return fmt.Errorf("DB_URL is required; set via --db-url flag or DB_URL environment variable")
			}

			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()

			pool, err := config.NewPool(ctx, dbURL)
			if err != nil {
				return fmt.Errorf("failed to connect to database: %w", err)
			}
			defer config.ClosePool(pool)

			teamSvc := service.NewTeamService(pool)

			var teams interface{}
			var headers []string
			var rows [][]string

			if ownerID != "" {
				teamList, err := teamSvc.ListTeamsByOwner(ctx, ownerID, int32(limit))
				if err != nil {
					return fmt.Errorf("failed to list teams: %w", err)
				}
				teams = teamList
				headers = []string{"ID", "NAME", "OWNER", "VISIBILITY"}
				for _, t := range teamList {
					rows = append(rows, []string{t.ID, t.Name, t.OwnerID, t.DefaultVisibility})
				}
			} else {
				// List all teams — use ownerID="" to return all
				teamList, err := teamSvc.ListTeamsByOwner(ctx, "", int32(limit))
				if err != nil {
					return fmt.Errorf("failed to list teams: %w", err)
				}
				teams = teamList
				headers = []string{"ID", "NAME", "OWNER", "VISIBILITY"}
				for _, t := range teamList {
					rows = append(rows, []string{t.ID, t.Name, t.OwnerID, t.DefaultVisibility})
				}
			}

			return teamOutput(cmd, teams, headers, rows)
		},
	}

	cmd.Flags().StringVar(&dbURL, "db-url", "", "Database URL")
	cmd.Flags().StringVar(&ownerID, "owner-id", "", "Filter teams by owner user ID")

	cmd.Flags().IntVar(&limit, "limit", 1000, "Maximum number of teams to return")

	return cmd
}

func newTeamAddCommand() *cobra.Command {
	var (
		dbURL  string
		teamID string
		userID string
	)

	cmd := &cobra.Command{
		Use:   "add",
		Short: "Add a member to a team",
		RunE: func(cmd *cobra.Command, args []string) error {
			if dbURL == "" {
				dbURL = os.Getenv("DB_URL")
			}
			if dbURL == "" {
				return fmt.Errorf("DB_URL is required; set via --db-url flag or DB_URL environment variable")
			}
			if teamID == "" {
				return fmt.Errorf("--team-id is required")
			}
			if userID == "" {
				return fmt.Errorf("--user-id is required")
			}

			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()

			pool, err := config.NewPool(ctx, dbURL)
			if err != nil {
				return fmt.Errorf("failed to connect to database: %w", err)
			}
			defer config.ClosePool(pool)

			memberSvc := service.NewTeamMembersService(pool)
			if err := memberSvc.AddMember(ctx, teamID, userID); err != nil {
				return fmt.Errorf("failed to add team member: %w", err)
			}

			fmt.Printf("User %s added to team %s.\n", userID, teamID)
			return nil
		},
	}

	cmd.Flags().StringVar(&dbURL, "db-url", "", "Database URL")
	cmd.Flags().StringVar(&teamID, "team-id", "", "Team ID")
	cmd.Flags().StringVar(&userID, "user-id", "", "User ID to add")

	return cmd
}

func newTeamRemoveCommand() *cobra.Command {
	var (
		dbURL  string
		teamID string
		userID string
	)

	cmd := &cobra.Command{
		Use:   "remove",
		Short: "Remove a member from a team",
		RunE: func(cmd *cobra.Command, args []string) error {
			if dbURL == "" {
				dbURL = os.Getenv("DB_URL")
			}
			if dbURL == "" {
				return fmt.Errorf("DB_URL is required; set via --db-url flag or DB_URL environment variable")
			}
			if teamID == "" {
				return fmt.Errorf("--team-id is required")
			}
			if userID == "" {
				return fmt.Errorf("--user-id is required")
			}

			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()

			pool, err := config.NewPool(ctx, dbURL)
			if err != nil {
				return fmt.Errorf("failed to connect to database: %w", err)
			}
			defer config.ClosePool(pool)

			memberSvc := service.NewTeamMembersService(pool)
			if err := memberSvc.RemoveMember(ctx, teamID, userID); err != nil {
				return fmt.Errorf("failed to remove team member: %w", err)
			}

			fmt.Printf("User %s removed from team %s.\n", userID, teamID)
			return nil
		},
	}

	cmd.Flags().StringVar(&dbURL, "db-url", "", "Database URL")
	cmd.Flags().StringVar(&teamID, "team-id", "", "Team ID")
	cmd.Flags().StringVar(&userID, "user-id", "", "User ID to remove")

	return cmd
}
