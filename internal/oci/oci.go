// Package oci provides push and pull operations for OCI artifacts using oras-go v2.
// Authentication is handled automatically via Docker credential store (~/.docker/config.json).
package oci

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"

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
	MediaType   string                `json:"mediaType"`
	Config      ocispec.Descriptor    `json:"config"`
	Layers      []ocispec.Descriptor  `json:"layers"`
	Annotations map[string]string     `json:"annotations,omitempty"`
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
