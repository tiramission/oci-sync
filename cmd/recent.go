package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/pterm/pterm"
	"github.com/spf13/cobra"
	"github.com/tiramission/oci-sync/internal/cache"
	"gopkg.in/yaml.v3"
)

func newRecentCmd() *cobra.Command {
	var limit int
	var format string
	var clear bool

	cmd := &cobra.Command{
		Use:   "recent",
		Short: "Show recent activity history",
		Long: `Display recent oci-sync operations including push, pull, delete, and label activities.
Activities are stored locally and persist across sessions.

Examples:
  oci-sync recent
  oci-sync recent --limit 10
  oci-sync recent --format json
  oci-sync recent --clear`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if clear {
				return clearActivities()
			}
			return runRecent(cmd.Context(), limit, format)
		},
	}

	cmd.Flags().IntVarP(&limit, "limit", "n", 20, "maximum number of activities to show")
	cmd.Flags().StringVarP(&format, "format", "f", "table", "output format (table, json, yaml)")
	cmd.Flags().BoolVar(&clear, "clear", false, "clear all activity history")
	return cmd
}

func runRecent(ctx context.Context, limit int, format string) error {
	activities, err := cache.GetRecentActivities(limit)
	if err != nil {
		return fmt.Errorf("failed to load activities: %w", err)
	}

	if len(activities) == 0 {
		fmt.Println("No recent activities")
		return nil
	}

	switch format {
	case "json":
		data, err := json.MarshalIndent(activities, "", "  ")
		if err != nil {
			return fmt.Errorf("marshal json: %w", err)
		}
		fmt.Println(string(data))
	case "yaml":
		data, err := yaml.Marshal(activities)
		if err != nil {
			return fmt.Errorf("marshal yaml: %w", err)
		}
		fmt.Print(string(data))
	default:
		fmt.Println()
		data := pterm.TableData{
			{"TIME", "TYPE", "REMOTE REF", "LOCAL PATH", "LABELS", "STATUS"},
		}

		for _, a := range activities {
			status := "✓ success"
			if !a.Success {
				status = "✗ failed"
			}
			localPath := "-"
			if a.LocalPath != "" {
				localPath = a.LocalPath
			}
			labels := "-"
			if len(a.Labels) > 0 {
				labels = joinLabels(a.Labels)
			}
			data = append(data, []string{
				formatTime(a.Timestamp),
				string(a.Type),
				a.RemoteRef,
				localPath,
				labels,
				status,
			})
		}

		output, _ := pterm.DefaultTable.
			WithHasHeader(true).
			WithData(data).
			WithBoxed(true).
			WithSeparator(" │ ").
			Srender()
		fmt.Print(output)

		pterm.Println()
		fmt.Printf("  Total: %d activity(s)\n", len(activities))
	}

	return nil
}

func clearActivities() error {
	if err := cache.ClearActivities(); err != nil {
		return fmt.Errorf("failed to clear activities: %w", err)
	}
	fmt.Println("Activity history cleared")
	return nil
}

func formatTime(t time.Time) string {
	return t.Format("2006-01-02 15:04:05")
}

func joinLabels(labels []string) string {
	if len(labels) == 0 {
		return "-"
	}
	if len(labels) <= 2 {
		var result strings.Builder
		for i, l := range labels {
			if i > 0 {
				result.WriteString(", ")
			}
			result.WriteString(l)
		}
		return result.String()
	}
	return fmt.Sprintf("%s, %s... +%d", labels[0], labels[1], len(labels)-2)
}
