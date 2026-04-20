package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"charm.land/log/v2"
	"github.com/pterm/pterm"
	"github.com/spf13/cobra"
	"github.com/tiramission/oci-sync/internal/oci"
	"gopkg.in/yaml.v3"
)

func newListCmd() *cobra.Command {
	var remote string
	var format string
	var labels []string

	cmd := &cobra.Command{
		Use:   "list",
		Short: "List oci-sync artifacts in an OCI registry repository",
		Long: `List all valid artifacts previously pushed by oci-sync in the specified repository.

Example:
  oci-sync list --remote registry-1.docker.io/myuser/myrepo
  oci-sync list --remote registry-1.docker.io/myuser/myrepo --label app=myapp`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runList(cmd.Context(), remote, format, labels)
		},
	}

	cmd.Flags().StringVarP(&remote, "remote", "r", "", "remote OCI registry reference (format: <registry>/<repository> or <registry>)")
	cmd.Flags().StringVarP(&format, "format", "f", "table", "output format (table, json, yaml)")
	cmd.Flags().StringArrayVar(&labels, "label", []string{}, "filter by labels (key=value, can be repeated)")
	cmd.MarkFlagRequired("remote")

	return cmd
}

func runList(ctx context.Context, repoPath string, format string, filterLabels []string) error {
	log.Info("Fetching tags from registry...", "repo", repoPath)
	artifacts, err := oci.List(ctx, repoPath)
	if err != nil {
		return fmt.Errorf("list failed: %w", err)
	}

	if len(artifacts) == 0 {
		log.Info("No oci-sync artifacts found in repository", "repo", repoPath)
		return nil
	}

	if len(filterLabels) > 0 {
		filtered := make([]oci.ArtifactInfo, 0)
		for _, a := range artifacts {
			match := true
			for _, l := range filterLabels {
				parts := strings.SplitN(l, "=", 2)
				if len(parts) == 2 {
					if v, ok := a.Labels[parts[0]]; !ok || v != parts[1] {
						match = false
						break
					}
				} else {
					if v, ok := a.Labels[parts[0]]; !ok || v == "" {
						match = false
						break
					}
				}
			}
			if match {
				filtered = append(filtered, a)
			}
		}
		artifacts = filtered
	}

	if len(artifacts) == 0 {
		log.Info("No matching artifacts found")
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
			{"REPO", "TAG", "ENCRYPTED", "VERSION", "SIZE", "DIGEST", "LABELS"},
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
			labelStr := ""
			if len(a.Labels) > 0 {
				pairs := make([]string, 0, len(a.Labels))
				for k, v := range a.Labels {
					pairs = append(pairs, k+"="+v)
				}
				labelStr = strings.Join(pairs, ",")
			}
			data = append(data, []string{a.Repo, a.Tag, encStr, a.Version, formatBytes(int(a.Size)), digestShort, labelStr})
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
