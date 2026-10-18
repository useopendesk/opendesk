package main

import (
	"context"

	"github.com/spf13/cobra"

	"github.com/useopendesk/opendesk/internal/admin"
	"github.com/useopendesk/opendesk/internal/db"
)

// updatesCommand is how opendesk-ctl tells the instance about automatic updates
// (contracts/admin.md "Updates"), so the admin panel shows whether they are on and how
// the last run went.
func updatesCommand() *cobra.Command {
	cmd := &cobra.Command{Use: "updates", Short: "Automatic update status, as opendesk-ctl reports it"}

	report := &cobra.Command{
		Use:   "report",
		Short: "Record whether automatic updates are on, and the result of a run",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			enabled, _ := cmd.Flags().GetBool("enabled")
			result, _ := cmd.Flags().GetString("result")
			detail, _ := cmd.Flags().GetString("detail")
			return withDB(func(ctx context.Context, d *db.DB) error {
				return admin.ReportAutoUpdate(ctx, d.Pool, enabled, result, detail)
			})
		},
	}
	report.Flags().Bool("enabled", false, "automatic updates are on")
	report.Flags().String("result", "", "the result of a run: current, updated, skipped or failed (empty: only --enabled)")
	report.Flags().String("detail", "", "one line about the run")

	cmd.AddCommand(report)
	return cmd
}
