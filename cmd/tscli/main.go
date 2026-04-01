package main

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/google/uuid"
	"github.com/spf13/cobra"
	"github.com/vlsi/troubleshooting-cli/internal/app"
	"github.com/vlsi/troubleshooting-cli/internal/domain"
	"github.com/vlsi/troubleshooting-cli/internal/mcp"
	"github.com/vlsi/troubleshooting-cli/internal/storage"
)

func main() {
	if err := rootCmd().Execute(); err != nil {
		os.Exit(1)
	}
}

func newService(dbPath string) (*app.Service, func(), error) {
	if dbPath == "" {
		var err error
		dbPath, err = storage.DefaultDBPath()
		if err != nil {
			return nil, nil, err
		}
	}
	store, err := storage.NewSQLiteStore(dbPath)
	if err != nil {
		return nil, nil, err
	}
	svc := app.NewService(store, func() string { return uuid.New().String() })
	return svc, func() { store.Close() }, nil
}

func printJSON(v any) {
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	enc.Encode(v)
}

func rootCmd() *cobra.Command {
	var dbPath string
	root := &cobra.Command{
		Use:   "tscli",
		Short: "Local troubleshooting session manager",
	}
	root.PersistentFlags().StringVar(&dbPath, "db", "", "Database path (default: ~/.troubleshooting/sessions.db)")
	root.AddCommand(sessionCmd(&dbPath))
	root.AddCommand(mcpCmd(&dbPath))
	return root
}

func sessionCmd(dbPath *string) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "session",
		Short: "Manage investigation sessions",
	}
	cmd.AddCommand(
		startCmd(dbPath),
		getStateCmd(dbPath),
		addFindingCmd(dbPath),
		addHypothesisCmd(dbPath),
		updateHypothesisCmd(dbPath),
		rankHypothesesCmd(dbPath),
		recommendCmd(dbPath),
		summaryCmd(dbPath),
		timelineCmd(dbPath),
		closeCmd(dbPath),
	)
	return cmd
}

func startCmd(dbPath *string) *cobra.Command {
	var title, service, env, incident string
	cmd := &cobra.Command{
		Use:   "start",
		Short: "Start a new investigation session",
		RunE: func(cmd *cobra.Command, args []string) error {
			svc, cleanup, err := newService(*dbPath)
			if err != nil {
				return err
			}
			defer cleanup()
			sess, err := svc.StartSession(title, service, env, incident, nil)
			if err != nil {
				return err
			}
			printJSON(sess)
			return nil
		},
	}
	cmd.Flags().StringVar(&title, "title", "", "Session title (required)")
	cmd.Flags().StringVar(&service, "service", "", "Service name (required)")
	cmd.Flags().StringVar(&env, "env", "", "Environment (required)")
	cmd.Flags().StringVar(&incident, "incident", "", "Incident hint")
	cmd.MarkFlagRequired("title")
	cmd.MarkFlagRequired("service")
	cmd.MarkFlagRequired("env")
	return cmd
}

func getStateCmd(dbPath *string) *cobra.Command {
	var id string
	cmd := &cobra.Command{
		Use:   "get-state",
		Short: "Get full session state",
		RunE: func(cmd *cobra.Command, args []string) error {
			svc, cleanup, err := newService(*dbPath)
			if err != nil {
				return err
			}
			defer cleanup()
			state, err := svc.GetState(id)
			if err != nil {
				return err
			}
			printJSON(state)
			return nil
		},
	}
	cmd.Flags().StringVar(&id, "id", "", "Session ID (required)")
	cmd.MarkFlagRequired("id")
	return cmd
}

func addFindingCmd(dbPath *string) *cobra.Command {
	var sessionID, kind, summary, details, importance string
	var tags []string
	var evidenceJSON string
	cmd := &cobra.Command{
		Use:   "add-finding",
		Short: "Add a finding to a session",
		RunE: func(cmd *cobra.Command, args []string) error {
			svc, cleanup, err := newService(*dbPath)
			if err != nil {
				return err
			}
			defer cleanup()
			var evidence []domain.Evidence
			if evidenceJSON != "" {
				if err := json.Unmarshal([]byte(evidenceJSON), &evidence); err != nil {
					return fmt.Errorf("invalid evidence JSON: %w", err)
				}
			}
			f, err := svc.AddFinding(sessionID, kind, summary, details, importance, tags, evidence)
			if err != nil {
				return err
			}
			printJSON(f)
			return nil
		},
	}
	cmd.Flags().StringVar(&sessionID, "session", "", "Session ID (required)")
	cmd.Flags().StringVar(&kind, "kind", "observation", "Finding kind")
	cmd.Flags().StringVar(&summary, "summary", "", "Short summary (required)")
	cmd.Flags().StringVar(&details, "details", "", "Detailed notes")
	cmd.Flags().StringVar(&importance, "importance", "", "Importance level")
	cmd.Flags().StringSliceVar(&tags, "tags", nil, "Tags")
	cmd.Flags().StringVar(&evidenceJSON, "evidence", "", `Evidence refs as JSON array, e.g. '[{"type":"log","pointer":"/var/log/app.log","snippet":"OOM"}]'`)
	cmd.MarkFlagRequired("session")
	cmd.MarkFlagRequired("summary")
	return cmd
}

func addHypothesisCmd(dbPath *string) *cobra.Command {
	var sessionID, statement, impact string
	var confidence float64
	var hasConfidence bool
	var nextChecks []string
	cmd := &cobra.Command{
		Use:   "add-hypothesis",
		Short: "Add a hypothesis to a session",
		RunE: func(cmd *cobra.Command, args []string) error {
			svc, cleanup, err := newService(*dbPath)
			if err != nil {
				return err
			}
			defer cleanup()
			var confPtr *float64
			if hasConfidence {
				confPtr = &confidence
			}
			h, err := svc.AddHypothesis(sessionID, statement, impact, confPtr, nextChecks)
			if err != nil {
				return err
			}
			printJSON(h)
			return nil
		},
	}
	cmd.Flags().StringVar(&sessionID, "session", "", "Session ID (required)")
	cmd.Flags().StringVar(&statement, "statement", "", "Hypothesis statement (required)")
	cmd.Flags().StringVar(&impact, "impact", "", "Expected impact")
	cmd.Flags().Float64Var(&confidence, "confidence", 0, "Confidence 0.0-1.0")
	cmd.Flags().StringSliceVar(&nextChecks, "next-checks", nil, "Suggested next checks")
	cmd.MarkFlagRequired("session")
	cmd.MarkFlagRequired("statement")
	cmd.PreRunE = func(cmd *cobra.Command, args []string) error {
		hasConfidence = cmd.Flags().Changed("confidence")
		return nil
	}
	return cmd
}

func updateHypothesisCmd(dbPath *string) *cobra.Command {
	var id, status string
	var confidence float64
	var hasConfidence bool
	var support, contradict, nextChecks []string
	cmd := &cobra.Command{
		Use:   "update-hypothesis",
		Short: "Update a hypothesis",
		RunE: func(cmd *cobra.Command, args []string) error {
			svc, cleanup, err := newService(*dbPath)
			if err != nil {
				return err
			}
			defer cleanup()
			var statusPtr *domain.HypothesisStatus
			if status != "" {
				s := domain.HypothesisStatus(status)
				statusPtr = &s
			}
			var confPtr *float64
			if hasConfidence {
				confPtr = &confidence
			}
			h, err := svc.UpdateHypothesis(id, statusPtr, confPtr, support, contradict, nextChecks)
			if err != nil {
				return err
			}
			printJSON(h)
			return nil
		},
	}
	cmd.Flags().StringVar(&id, "id", "", "Hypothesis ID (required)")
	cmd.Flags().StringVar(&status, "status", "", "New status")
	cmd.Flags().Float64Var(&confidence, "confidence", 0, "Confidence 0.0-1.0")
	cmd.Flags().StringSliceVar(&support, "support", nil, "Supporting finding IDs")
	cmd.Flags().StringSliceVar(&contradict, "contradict", nil, "Contradicting finding IDs")
	cmd.Flags().StringSliceVar(&nextChecks, "next-checks", nil, "Additional next checks")
	cmd.MarkFlagRequired("id")
	cmd.PreRunE = func(cmd *cobra.Command, args []string) error {
		hasConfidence = cmd.Flags().Changed("confidence")
		return nil
	}
	return cmd
}

func rankHypothesesCmd(dbPath *string) *cobra.Command {
	var sessionID string
	cmd := &cobra.Command{
		Use:   "rank-hypotheses",
		Short: "Rank hypotheses for a session",
		RunE: func(cmd *cobra.Command, args []string) error {
			svc, cleanup, err := newService(*dbPath)
			if err != nil {
				return err
			}
			defer cleanup()
			ranked, err := svc.RankHypotheses(sessionID)
			if err != nil {
				return err
			}
			printJSON(ranked)
			return nil
		},
	}
	cmd.Flags().StringVar(&sessionID, "session", "", "Session ID (required)")
	cmd.MarkFlagRequired("session")
	return cmd
}

func recommendCmd(dbPath *string) *cobra.Command {
	var sessionID string
	var n int
	cmd := &cobra.Command{
		Use:   "recommend-next-step",
		Short: "Get recommended next steps",
		RunE: func(cmd *cobra.Command, args []string) error {
			svc, cleanup, err := newService(*dbPath)
			if err != nil {
				return err
			}
			defer cleanup()
			recs, err := svc.RecommendNextSteps(sessionID, n)
			if err != nil {
				return err
			}
			printJSON(recs)
			return nil
		},
	}
	cmd.Flags().StringVar(&sessionID, "session", "", "Session ID (required)")
	cmd.Flags().IntVarP(&n, "count", "n", 3, "Max recommendations")
	cmd.MarkFlagRequired("session")
	return cmd
}

func summaryCmd(dbPath *string) *cobra.Command {
	var sessionID, mode string
	cmd := &cobra.Command{
		Use:   "generate-summary",
		Short: "Generate a markdown summary",
		RunE: func(cmd *cobra.Command, args []string) error {
			svc, cleanup, err := newService(*dbPath)
			if err != nil {
				return err
			}
			defer cleanup()
			md, err := svc.GenerateSummary(sessionID, mode)
			if err != nil {
				return err
			}
			fmt.Print(md)
			return nil
		},
	}
	cmd.Flags().StringVar(&sessionID, "session", "", "Session ID (required)")
	cmd.Flags().StringVar(&mode, "mode", "handoff", "Summary mode: handoff or postmortem-draft")
	cmd.MarkFlagRequired("session")
	return cmd
}

func timelineCmd(dbPath *string) *cobra.Command {
	var sessionID string
	cmd := &cobra.Command{
		Use:   "get-timeline",
		Short: "Get session timeline",
		RunE: func(cmd *cobra.Command, args []string) error {
			svc, cleanup, err := newService(*dbPath)
			if err != nil {
				return err
			}
			defer cleanup()
			events, err := svc.GetTimeline(sessionID)
			if err != nil {
				return err
			}
			printJSON(events)
			return nil
		},
	}
	cmd.Flags().StringVar(&sessionID, "session", "", "Session ID (required)")
	cmd.MarkFlagRequired("session")
	return cmd
}

func closeCmd(dbPath *string) *cobra.Command {
	var sessionID, status, outcome string
	cmd := &cobra.Command{
		Use:   "close",
		Short: "Close a session",
		RunE: func(cmd *cobra.Command, args []string) error {
			svc, cleanup, err := newService(*dbPath)
			if err != nil {
				return err
			}
			defer cleanup()
			validStatuses := map[string]domain.SessionStatus{
				"resolved":       domain.SessionResolved,
				"mitigated":      domain.SessionMitigated,
				"abandoned":      domain.SessionAbandoned,
				"needs-followup": domain.SessionFollowup,
			}
			s, ok := validStatuses[strings.ToLower(status)]
			if !ok {
				return fmt.Errorf("invalid status %q; use: resolved, mitigated, abandoned, needs-followup", status)
			}
			sess, err := svc.CloseSession(sessionID, s, outcome)
			if err != nil {
				return err
			}
			printJSON(sess)
			return nil
		},
	}
	cmd.Flags().StringVar(&sessionID, "session", "", "Session ID (required)")
	cmd.Flags().StringVar(&status, "status", "", "Final status (required)")
	cmd.Flags().StringVar(&outcome, "outcome", "", "Outcome description")
	cmd.MarkFlagRequired("session")
	cmd.MarkFlagRequired("status")
	return cmd
}

func mcpCmd(dbPath *string) *cobra.Command {
	return &cobra.Command{
		Use:   "mcp",
		Short: "Run MCP stdio server",
		Long:  "Start an MCP (Model Context Protocol) stdio server for AI agent integration.",
		RunE: func(cmd *cobra.Command, args []string) error {
			svc, cleanup, err := newService(*dbPath)
			if err != nil {
				return err
			}
			defer cleanup()
			mcp.NewServer(svc).Run(os.Stdin, os.Stdout)
			return nil
		},
	}
}
