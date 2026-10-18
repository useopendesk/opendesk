// Command opendesk runs the opendesk API and workers.
package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/spf13/cobra"

	"github.com/useopendesk/opendesk/internal/app"
	"github.com/useopendesk/opendesk/internal/config"
	"github.com/useopendesk/opendesk/internal/db"
	"github.com/useopendesk/opendesk/internal/version"
)

func main() {
	root := &cobra.Command{Use: "opendesk", Short: "The open-source helpdesk", SilenceUsage: true}

	serve := &cobra.Command{
		Use:   "serve",
		Short: "Run the API and/or workers (OPENDESK_ROLE=all|api|worker)",
		RunE: func(cmd *cobra.Command, _ []string) error {
			cfg, err := config.Load()
			if err != nil {
				return err
			}
			if role, _ := cmd.Flags().GetString("role"); role != "" {
				cfg.Role = role
			}
			log := app.NewLogger(cfg)
			ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
			defer stop()
			if os.Getenv("AUTO_MIGRATE") != "false" {
				if err := db.Migrate(ctx, cfg.DatabaseURLDirect); err != nil {
					return err
				}
			}
			d, cleanup, err := app.Build(ctx, cfg, log)
			if err != nil {
				return err
			}
			defer cleanup()
			return app.Serve(ctx, d)
		},
	}
	serve.Flags().String("role", "", "override OPENDESK_ROLE")

	migrateCmd := &cobra.Command{
		Use:   "migrate",
		Short: "Apply database migrations",
		RunE: func(*cobra.Command, []string) error {
			cfg, err := config.Load()
			if err != nil {
				return err
			}
			if err := db.Migrate(context.Background(), cfg.DatabaseURLDirect); err != nil {
				return err
			}
			fmt.Println("migrations applied")
			return nil
		},
	}

	health := &cobra.Command{
		Use:   "healthcheck",
		Short: "Exit 0 when the local server answers /healthz (for container health checks)",
		RunE: func(*cobra.Command, []string) error {
			addr := os.Getenv("HTTP_ADDR")
			if addr == "" || addr[0] == ':' {
				addr = "127.0.0.1" + addr
				if addr == "127.0.0.1" {
					addr += ":8080"
				}
			}
			c := &http.Client{Timeout: 3 * time.Second}
			req, _ := http.NewRequestWithContext(context.Background(), http.MethodGet, "http://"+addr+"/healthz", nil)
			resp, err := c.Do(req)
			if err != nil {
				return err
			}
			defer resp.Body.Close()
			if resp.StatusCode != http.StatusOK {
				return fmt.Errorf("status %d", resp.StatusCode)
			}
			return nil
		},
	}

	ver := &cobra.Command{Use: "version", Run: func(*cobra.Command, []string) { fmt.Println(version.Version, version.Commit) }}

	root.AddCommand(serve, migrateCmd, health, ver, adminCommand(), updatesCommand())
	if err := root.Execute(); err != nil {
		os.Exit(1)
	}
}
