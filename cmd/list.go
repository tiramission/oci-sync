package cmd

import (
	"context"
	"encoding/json"
	"fmt"

	"charm.land/log/v2"
	"github.com/pterm/pterm"
	"github.com/spf13/cobra"
	"github.com/tiramission/oci-sync/internal/oci"
	"gopkg.in/yaml.v3"
)

func newListCmd() *cobra.Command {
	var remote string
	var format string
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List oci-sync artifacts in an OCI registry repository",
		Long: `List all valid artifacts previously pushed by oci-sync in the specified repository.

Example:
  oci-sync list --remote registry-1.docker.io/myuser/myrepo`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runList(cmd.Context(), remote, format)
		},
	}

	cmd.Flags().StringVarP(&remote, "remote", "r", "", "remote OCI registry reference (format: <registry>/<repository> or <registry>)")
	cmd.Flags().StringVarP(&format, "format", "f", "table", "output format (table, json, yaml)")
	cmd.MarkFlagRequired("remote")

	return cmd
}

func runList(ctx context.Context, repoPath string, format string) error {
	log.Info("Fetching tags from registry...", "repo", repoPath)
	artifacts, err := oci.List(ctx, repoPath)
	if err != nil {
		return fmt.Errorf("list failed: %w", err)
	}

	if len(artifacts) == 0 {
		log.Info("No oci-sync artifacts found in repository", "repo", repoPath)
		return nil
	}

	switch format {
	case "json":
		data, err := json.MarshalIndent(artifacts, "", "  ")
		if err != nil {
			return fmt.Errorf("marshal json: %w", err)
		}
		fmt.Println(string(data))
	case "yaml":
		data, err := yaml.Marshal(artifacts)
		if err != nil {
			return fmt.Errorf("marshal yaml: %w", err)
		}
		fmt.Print(string(data))
	default:
		fmt.Println()
		fmt.Printf("  Repository: %s\n\n", repoPath)

		data := pterm.TableData{
			{"REPO", "TAG", "ENCRYPTED", "VERSION", "SIZE", "DIGEST"},
		}

		for _, a := range artifacts {
			encStr := "yes"
			if !a.Encrypted {
				encStr = "no"
			}
			digestShort := a.Digest
			if len(digestShort) > 32 {
				digestShort = digestShort[:32] + "..."
			}
			data = append(data, []string{a.Repo, a.Tag, encStr, a.Version, formatBytes(int(a.Size)), digestShort})
		}

		output, _ := pterm.DefaultTable.
			WithHasHeader(true).
			WithData(data).
			WithBoxed(true).
			WithSeparator(" │ ").
			Srender()
		fmt.Print(output)

		pterm.Println()
		fmt.Printf("  Total: %d artifact(s)\n", len(artifacts))
	}

	return nil
}
