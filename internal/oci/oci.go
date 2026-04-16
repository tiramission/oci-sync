// Package oci provides push and pull operations for OCI artifacts using oras-go v2.
// Authentication is handled automatically via Docker credential store (~/.docker/config.json).
package oci

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"github.com/opencontainers/go-digest"
	specs "github.com/opencontainers/image-spec/specs-go"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
	"oras.land/oras-go/v2"
	"oras.land/oras-go/v2/registry/remote"
	"oras.land/oras-go/v2/registry/remote/auth"
	"oras.land/oras-go/v2/registry/remote/credentials"
	"oras.land/oras-go/v2/registry/remote/retry"
)

const (
	// AnnotationEncrypted marks whether the content is encrypted.
	AnnotationEncrypted = "io.oci-sync.encrypted"
	// AnnotationVersion records the tool version used to push.
	AnnotationVersion = "io.oci-sync.version"
	// Version is the current tool version.
	Version = "0.1.0"

	// mediaTypeLayer is used for arbitrary binary data layers.
	mediaTypeLayer = "application/octet-stream"
)

// ociManifest mirrors ocispec.Manifest but uses specs.Versioned to set schemaVersion correctly.
type ociManifest struct {
	specs.Versioned
	MediaType   string               `json:"mediaType"`
	Config      ocispec.Descriptor   `json:"config"`
	Layers      []ocispec.Descriptor `json:"layers"`
	Annotations map[string]string    `json:"annotations,omitempty"`
}

// Push pushes data as an OCI artifact to the given reference.
// ref must be in the format <registry>/<repository>:<tag>.
// encrypted indicates whether data has been encrypted.
func Push(ctx context.Context, data []byte, ref string, encrypted bool) error {
	repo, err := newRepository(ctx, ref)
	if err != nil {
		return err
	}

	// Create the layer descriptor
	layerDesc := ocispec.Descriptor{
		MediaType: mediaTypeLayer,
		Digest:    digest.FromBytes(data),
		Size:      int64(len(data)),
	}

	// Build manifest annotations
	annotations := map[string]string{
		AnnotationVersion: Version,
	}
	if encrypted {
		annotations[AnnotationEncrypted] = "true"
	} else {
		annotations[AnnotationEncrypted] = "false"
	}

	// Build empty config
	configBytes := emptyConfigBytes()
	configDesc := ocispec.Descriptor{
		MediaType: ocispec.MediaTypeImageConfig,
		Digest:    digest.FromBytes(configBytes),
		Size:      int64(len(configBytes)),
	}

	// Build OCI manifest
	manifest := ociManifest{
		Versioned:   specs.Versioned{SchemaVersion: 2},
		MediaType:   ocispec.MediaTypeImageManifest,
		Config:      configDesc,
		Layers:      []ocispec.Descriptor{layerDesc},
		Annotations: annotations,
	}
	manifestBytes, err := json.Marshal(manifest)
	if err != nil {
		return fmt.Errorf("marshal manifest: %w", err)
	}

	// Push layer
	if err := repo.Push(ctx, layerDesc, bytes.NewReader(data)); err != nil {
		return fmt.Errorf("push layer: %w", err)
	}

	// Push config (empty)
	if err := repo.Push(ctx, configDesc, bytes.NewReader(configBytes)); err != nil {
		return fmt.Errorf("push config: %w", err)
	}

	// Push manifest with tag
	manifestDesc := ocispec.Descriptor{
		MediaType: ocispec.MediaTypeImageManifest,
		Digest:    digest.FromBytes(manifestBytes),
		Size:      int64(len(manifestBytes)),
	}
	if err := repo.PushReference(ctx, manifestDesc, bytes.NewReader(manifestBytes), repo.Reference.Reference); err != nil {
		return fmt.Errorf("push manifest: %w", err)
	}

	return nil
}

// PullResult contains the pulled data and its metadata.
type PullResult struct {
	Data      []byte
	Encrypted bool
}

// Pull fetches an OCI artifact from the given reference.
func Pull(ctx context.Context, ref string) (*PullResult, error) {
	repo, err := newRepository(ctx, ref)
	if err != nil {
		return nil, err
	}

	// Fetch manifest bytes by tag
	_, manifestBytes, err := oras.FetchBytes(ctx, repo, repo.Reference.Reference, oras.DefaultFetchBytesOptions)
	if err != nil {
		return nil, fmt.Errorf("fetch manifest: %w", err)
	}

	var manifest ociManifest
	if err := json.Unmarshal(manifestBytes, &manifest); err != nil {
		return nil, fmt.Errorf("unmarshal manifest: %w", err)
	}

	if len(manifest.Layers) == 0 {
		return nil, fmt.Errorf("manifest has no layers")
	}

	// Fetch the first (and only) layer by digest
	layerDesc := manifest.Layers[0]
	rc, err := repo.Blobs().Fetch(ctx, layerDesc)
	if err != nil {
		return nil, fmt.Errorf("fetch layer blob: %w", err)
	}
	defer rc.Close()

	layerBytes, err := io.ReadAll(rc)
	if err != nil {
		return nil, fmt.Errorf("read layer: %w", err)
	}

	encrypted := manifest.Annotations[AnnotationEncrypted] == "true"
	return &PullResult{
		Data:      layerBytes,
		Encrypted: encrypted,
	}, nil
}

// newRepository creates an authenticated remote repository client.
// Authentication is loaded from ~/.docker/config.json (Docker credential store).
func newRepository(ctx context.Context, ref string) (*remote.Repository, error) {
	repo, err := remote.NewRepository(ref)
	if err != nil {
		return nil, fmt.Errorf("parse reference %q: %w", ref, err)
	}

	// Load credentials from Docker config (~/.docker/config.json)
	credStore, err := credentials.NewStoreFromDocker(credentials.StoreOptions{
		AllowPlaintextPut: false,
	})
	if err != nil {
		return nil, fmt.Errorf("load docker credential store: %w", err)
	}

	repo.Client = &auth.Client{
		Client:     retry.DefaultClient,
		Cache:      auth.DefaultCache,
		Credential: credentials.Credential(credStore),
	}

	return repo, nil
}

// emptyConfigBytes returns the bytes for an empty OCI config.
func emptyConfigBytes() []byte {
	return []byte("{}")
}

// Delete removes an OCI artifact from the remote registry.
func Delete(ctx context.Context, ref string) error {
	repo, err := newRepository(ctx, ref)
	if err != nil {
		return err
	}

	// Resolve the reference to a descriptor (this gets the digest)
	desc, err := repo.Resolve(ctx, repo.Reference.Reference)
	if err != nil {
		return fmt.Errorf("resolve tag/digest: %w", err)
	}

	// Delete the manifest by descriptor
	if err := repo.Delete(ctx, desc); err != nil {
		return fmt.Errorf("delete artifact: %w", err)
	}

	return nil
}

// ArtifactInfo represents metadata of an artifact pushed by oci-sync.
type ArtifactInfo struct {
	FullName  string `json:"fullName" yaml:"fullName"`
	Repo      string `json:"repo" yaml:"repo"`
	Tag       string `json:"tag" yaml:"tag"`
	Digest    string `json:"digest" yaml:"digest"`
	Encrypted bool   `json:"encrypted" yaml:"encrypted"`
	Version   string `json:"version" yaml:"version"`
}

// List retrieves all oci-sync artifacts in the specified repository or registry.
func List(ctx context.Context, ref string) ([]ArtifactInfo, error) {
	// 1. Try treating as a repository first (contains '/')
	if strings.Contains(ref, "/") {
		registry, _ := splitRegistry(ref)
		repoObj, err := newRepository(ctx, ref)
		if err != nil {
			return nil, err
		}
		return listRepoTags(ctx, registry, repoObj)
	}

	// 2. Treat as a registry host
	reg, err := newRegistry(ctx, ref)
	if err != nil {
		return nil, err
	}

	var results []ArtifactInfo
	err = reg.Repositories(ctx, "", func(repos []string) error {
		for _, repoName := range repos {
			fullRepoRef := ref + "/" + repoName
			repoObj, err := newRepository(ctx, fullRepoRef)
			if err != nil {
				continue
			}
			infos, err := listRepoTags(ctx, ref, repoObj)
			if err == nil {
				results = append(results, infos...)
			}
		}
		return nil
	})

	if err != nil {
		return nil, fmt.Errorf("list repositories failed: %w", err)
	}

	return results, nil
}

// listRepoTags is a helper to list all tags in a given repository.
func listRepoTags(ctx context.Context, registry string, repo *remote.Repository) ([]ArtifactInfo, error) {
	var results []ArtifactInfo
	repoName := repo.Reference.Repository

	err := repo.Tags(ctx, "", func(tags []string) error {
		for _, tag := range tags {
			desc, err := repo.Resolve(ctx, tag)
			if err != nil {
				continue
			}

			rc, err := repo.Fetch(ctx, desc)
			if err != nil {
				continue
			}
			data, err := io.ReadAll(rc)
			rc.Close()
			if err != nil {
				continue
			}

			var manifest ocispec.Manifest
			if err := json.Unmarshal(data, &manifest); err != nil {
				continue
			}

			if val, ok := manifest.Annotations[AnnotationVersion]; ok {
				encStr := manifest.Annotations[AnnotationEncrypted]
				results = append(results, ArtifactInfo{
					FullName:  registry + "/" + repoName + ":" + tag,
					Repo:      repoName,
					Tag:       tag,
					Digest:    desc.Digest.String(),
					Encrypted: encStr == "true",
					Version:   val,
				})
			}
		}
		return nil
	})
	return results, err
}

// splitRegistry splits a reference into registry and repository parts.
func splitRegistry(ref string) (registry, repo string) {
	parts := strings.SplitN(ref, "/", 2)
	if len(parts) == 2 {
		return parts[0], parts[1]
	}
	return ref, ""
}

func newRegistry(ctx context.Context, host string) (*remote.Registry, error) {
	reg, err := remote.NewRegistry(host)
	if err != nil {
		return nil, fmt.Errorf("parse registry %q: %w", host, err)
	}

	credStore, err := credentials.NewStoreFromDocker(credentials.StoreOptions{
		AllowPlaintextPut: false,
	})
	if err != nil {
		return nil, fmt.Errorf("load docker credential store: %w", err)
	}

	reg.Client = &auth.Client{
		Client:     retry.DefaultClient,
		Cache:      auth.DefaultCache,
		Credential: credentials.Credential(credStore),
	}

	return reg, nil
}
