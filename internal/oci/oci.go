// Package oci provides push and pull operations for OCI artifacts using oras-go v2.
// Authentication supports config file (per-registry) and Docker credential store fallback.
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
	"github.com/tiramission/oci-sync/internal/config"
	"github.com/tiramission/oci-sync/internal/version"
	"oras.land/oras-go/v2"
	"oras.land/oras-go/v2/registry/remote"
	"oras.land/oras-go/v2/registry/remote/auth"
	"oras.land/oras-go/v2/registry/remote/credentials"
	"oras.land/oras-go/v2/registry/remote/retry"
)

const (
	AnnotationEncrypted = "io.oci-sync.encrypted"
	AnnotationVersion   = "io.oci-sync.version"
	mediaTypeLayer      = "application/octet-stream"
)

type ociManifest struct {
	specs.Versioned
	MediaType   string               `json:"mediaType"`
	Config      ocispec.Descriptor   `json:"config"`
	Layers      []ocispec.Descriptor `json:"layers"`
	Annotations map[string]string    `json:"annotations,omitempty"`
}

func Push(ctx context.Context, data []byte, ref string, encrypted bool) error {
	repo, err := newRepository(ctx, ref)
	if err != nil {
		return err
	}

	layerDesc := ocispec.Descriptor{
		MediaType: mediaTypeLayer,
		Digest:    digest.FromBytes(data),
		Size:      int64(len(data)),
	}

	annotations := map[string]string{
		AnnotationVersion: version.Version,
	}
	if encrypted {
		annotations[AnnotationEncrypted] = "true"
	} else {
		annotations[AnnotationEncrypted] = "false"
	}

	configBytes := emptyConfigBytes()
	configDesc := ocispec.Descriptor{
		MediaType: ocispec.MediaTypeImageConfig,
		Digest:    digest.FromBytes(configBytes),
		Size:      int64(len(configBytes)),
	}

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

	if err := repo.Push(ctx, layerDesc, bytes.NewReader(data)); err != nil {
		return fmt.Errorf("push layer: %w", err)
	}

	if err := repo.Push(ctx, configDesc, bytes.NewReader(configBytes)); err != nil {
		return fmt.Errorf("push config: %w", err)
	}

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

type PullResult struct {
	Data      []byte
	Encrypted bool
}

func IsEncrypted(ctx context.Context, ref string) (bool, error) {
	repo, err := newRepository(ctx, ref)
	if err != nil {
		return false, err
	}

	_, manifestBytes, err := oras.FetchBytes(ctx, repo, repo.Reference.Reference, oras.DefaultFetchBytesOptions)
	if err != nil {
		return false, fmt.Errorf("fetch manifest: %w", err)
	}

	var manifest ociManifest
	if err := json.Unmarshal(manifestBytes, &manifest); err != nil {
		return false, fmt.Errorf("unmarshal manifest: %w", err)
	}

	return manifest.Annotations[AnnotationEncrypted] == "true", nil
}

func Pull(ctx context.Context, ref string) (*PullResult, error) {
	repo, err := newRepository(ctx, ref)
	if err != nil {
		return nil, err
	}

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

func newRepository(ctx context.Context, ref string) (*remote.Repository, error) {
	repo, err := remote.NewRepository(ref)
	if err != nil {
		return nil, fmt.Errorf("parse reference %q: %w", ref, err)
	}

	authClient := &auth.Client{
		Client: retry.DefaultClient,
		Cache:  auth.DefaultCache,
	}

	host := repo.Reference.Host()
	if regAuth, ok := config.GetRegistryAuth(host); ok && regAuth.Username != "" && regAuth.Password != "" {
		authClient.Credential = auth.CredentialFunc(func(ctx context.Context, reg string) (auth.Credential, error) {
			return auth.Credential{
				Username: regAuth.Username,
				Password: regAuth.Password,
			}, nil
		})
	} else {
		credStore, err := credentials.NewStoreFromDocker(credentials.StoreOptions{
			AllowPlaintextPut: false,
		})
		if err != nil {
			return nil, fmt.Errorf("load docker credential store: %w", err)
		}
		authClient.Credential = credentials.Credential(credStore)
	}

	repo.Client = authClient

	return repo, nil
}

func emptyConfigBytes() []byte {
	return []byte("{}")
}

func Delete(ctx context.Context, ref string) error {
	repo, err := newRepository(ctx, ref)
	if err != nil {
		return err
	}

	desc, err := repo.Resolve(ctx, repo.Reference.Reference)
	if err != nil {
		return fmt.Errorf("resolve tag/digest: %w", err)
	}

	if err := repo.Delete(ctx, desc); err != nil {
		return fmt.Errorf("delete artifact: %w", err)
	}

	return nil
}

type ArtifactInfo struct {
	FullName  string `json:"fullName" yaml:"fullName"`
	Repo      string `json:"repo" yaml:"repo"`
	Tag       string `json:"tag" yaml:"tag"`
	Digest    string `json:"digest" yaml:"digest"`
	Encrypted bool   `json:"encrypted" yaml:"encrypted"`
	Version   string `json:"version" yaml:"version"`
	Size      int64  `json:"size" yaml:"size"`
}

func List(ctx context.Context, ref string) ([]ArtifactInfo, error) {
	if strings.Contains(ref, "/") {
		registry, _ := splitRegistry(ref)
		repoObj, err := newRepository(ctx, ref)
		if err != nil {
			return nil, err
		}
		return listRepoTags(ctx, registry, repoObj)
	}

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
				var size int64
				if len(manifest.Layers) > 0 {
					size = manifest.Layers[0].Size
				}
				results = append(results, ArtifactInfo{
					FullName:  registry + "/" + repoName + ":" + tag,
					Repo:      repoName,
					Tag:       tag,
					Digest:    desc.Digest.String(),
					Encrypted: encStr == "true",
					Version:   val,
					Size:      size,
				})
			}
		}
		return nil
	})
	return results, err
}

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

	authClient := &auth.Client{
		Client: retry.DefaultClient,
		Cache:  auth.DefaultCache,
	}

	if regAuth, ok := config.GetRegistryAuth(host); ok && regAuth.Username != "" && regAuth.Password != "" {
		authClient.Credential = auth.CredentialFunc(func(ctx context.Context, reg string) (auth.Credential, error) {
			return auth.Credential{
				Username: regAuth.Username,
				Password: regAuth.Password,
			}, nil
		})
	} else {
		credStore, err := credentials.NewStoreFromDocker(credentials.StoreOptions{
			AllowPlaintextPut: false,
		})
		if err != nil {
			return nil, fmt.Errorf("load docker credential store: %w", err)
		}
		authClient.Credential = credentials.Credential(credStore)
	}

	reg.Client = authClient

	return reg, nil
}
