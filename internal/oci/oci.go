// Package oci provides push and pull operations for OCI artifacts using oras-go v2.
// Authentication supports config file (per-registry) and Docker credential store fallback.
package oci

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"maps"
	"strconv"
	"strings"
	"sync"

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

	// systemAnnotationPrefix marks annotations owned by oci-sync. User labels
	// never carry it, so it also separates labels from metadata in List.
	systemAnnotationPrefix = "io.oci-sync."
)

type ociManifest struct {
	specs.Versioned
	MediaType   string               `json:"mediaType"`
	Config      ocispec.Descriptor   `json:"config"`
	Layers      []ocispec.Descriptor `json:"layers"`
	Annotations map[string]string    `json:"annotations,omitempty"`
}

func Push(ctx context.Context, data []byte, ref string, encrypted bool, labels map[string]string) error {
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
	// User labels go in first and the system keys overwrite them, so a label
	// can never claim a different encryption state or version than reality.
	maps.Copy(annotations, labels)
	annotations[AnnotationVersion] = version.Version
	annotations[AnnotationEncrypted] = strconv.FormatBool(encrypted)

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

// Pull fetches the artifact layer for ref. When onRead is non-nil it is
// called after every read chunk with the cumulative downloaded bytes and the
// total layer size, enabling progress reporting.
func Pull(ctx context.Context, ref string, onRead func(done, total int64)) (*PullResult, error) {
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

	var buf bytes.Buffer
	pw := &progressWriter{w: &buf, total: layerDesc.Size, on: onRead}
	if _, err := io.Copy(pw, rc); err != nil {
		return nil, fmt.Errorf("read layer: %w", err)
	}

	encrypted := manifest.Annotations[AnnotationEncrypted] == "true"
	return &PullResult{
		Data:      buf.Bytes(),
		Encrypted: encrypted,
	}, nil
}

type progressWriter struct {
	w     io.Writer
	total int64
	done  int64
	on    func(done, total int64)
}

func (p *progressWriter) Write(b []byte) (int, error) {
	n, err := p.w.Write(b)
	p.done += int64(n)
	if p.on != nil {
		p.on(p.done, p.total)
	}
	return n, err
}

// newAuthClient builds the auth client for host. Credentials from the
// oci-sync config win; otherwise the Docker credential store is used, so
// `docker login` keeps working.
func newAuthClient(host string) (*auth.Client, error) {
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
		return authClient, nil
	}

	credStore, err := credentials.NewStoreFromDocker(credentials.StoreOptions{
		AllowPlaintextPut: false,
	})
	if err != nil {
		return nil, fmt.Errorf("load docker credential store: %w", err)
	}
	authClient.Credential = credentials.Credential(credStore)
	return authClient, nil
}

func newRepository(ctx context.Context, ref string) (*remote.Repository, error) {
	repo, err := remote.NewRepository(ref)
	if err != nil {
		return nil, fmt.Errorf("parse reference %q: %w", ref, err)
	}

	authClient, err := newAuthClient(repo.Reference.Host())
	if err != nil {
		return nil, err
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

func UpdateAnnotations(ctx context.Context, ref string, updates map[string]string, removeKeys []string) error {
	repo, err := newRepository(ctx, ref)
	if err != nil {
		return err
	}

	desc, err := repo.Resolve(ctx, repo.Reference.Reference)
	if err != nil {
		return fmt.Errorf("resolve tag/digest: %w", err)
	}

	manifestRC, err := repo.Fetch(ctx, desc)
	if err != nil {
		return fmt.Errorf("fetch manifest: %w", err)
	}
	defer manifestRC.Close()

	manifestBytes, err := io.ReadAll(manifestRC)
	if err != nil {
		return fmt.Errorf("read manifest: %w", err)
	}

	var manifest ociManifest
	if err := json.Unmarshal(manifestBytes, &manifest); err != nil {
		return fmt.Errorf("unmarshal manifest: %w", err)
	}
	encrypted := manifest.Annotations[AnnotationEncrypted] == "true"

	if manifest.Annotations == nil {
		manifest.Annotations = make(map[string]string)
	}

	maps.Copy(manifest.Annotations, updates)
	for _, k := range removeKeys {
		delete(manifest.Annotations, k)
	}
	// System annotations are authoritative and survive every label edit:
	// rewriting the encrypted flag would make pull skip decryption, and
	// dropping the version would hide the artifact from List.
	manifest.Annotations[AnnotationEncrypted] = strconv.FormatBool(encrypted)
	if v := manifest.Annotations[AnnotationVersion]; v == "" {
		manifest.Annotations[AnnotationVersion] = version.Version
	}

	newManifestBytes, err := json.Marshal(manifest)
	if err != nil {
		return fmt.Errorf("marshal manifest: %w", err)
	}

	newDesc := ocispec.Descriptor{
		MediaType: ocispec.MediaTypeImageManifest,
		Digest:    digest.FromBytes(newManifestBytes),
		Size:      int64(len(newManifestBytes)),
	}

	if err := repo.Push(ctx, newDesc, bytes.NewReader(newManifestBytes)); err != nil {
		return fmt.Errorf("push manifest: %w", err)
	}

	if err := repo.Tag(ctx, newDesc, repo.Reference.Reference); err != nil {
		return fmt.Errorf("update tag: %w", err)
	}

	return nil
}

type ArtifactInfo struct {
	FullName  string            `json:"fullName" yaml:"fullName"`
	Repo      string            `json:"repo" yaml:"repo"`
	Tag       string            `json:"tag" yaml:"tag"`
	Digest    string            `json:"digest" yaml:"digest"`
	Encrypted bool              `json:"encrypted" yaml:"encrypted"`
	Version   string            `json:"version" yaml:"version"`
	Size      int64             `json:"size" yaml:"size"`
	Labels    map[string]string `json:"labels,omitempty" yaml:"labels,omitempty"`
}

// listConcurrency bounds in-flight registry requests. Registries rate-limit
// aggressively, so this trades a little speed for not getting throttled.
const listConcurrency = 8

// List returns every oci-sync artifact under ref. A ref containing a "/"
// is treated as a single repository; a bare host lists every repository the
// registry exposes.
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

	// Collect the repository names first, then fan out: doing the fan-out
	// inside the pagination callback would serialise every repo behind one
	// slow listing.
	var repoNames []string
	if err := reg.Repositories(ctx, "", func(repos []string) error {
		repoNames = append(repoNames, repos...)
		return nil
	}); err != nil {
		return nil, fmt.Errorf("list repositories failed: %w", err)
	}

	results, errs := fanOut(len(repoNames), func(i int) ([]ArtifactInfo, error) {
		repoObj, err := newRepository(ctx, ref+"/"+repoNames[i])
		if err != nil {
			return nil, err
		}
		return listRepoTags(ctx, ref, repoObj)
	})

	if len(results) == 0 && len(errs) > 0 {
		return nil, fmt.Errorf("list repositories failed: %w", errors.Join(errs...))
	}
	return results, nil
}

// listRepoTags resolves every tag of one repository concurrently. Each tag
// needs a Resolve followed by a Fetch, so a serial walk costs two round
// trips per tag.
func listRepoTags(ctx context.Context, registry string, repo *remote.Repository) ([]ArtifactInfo, error) {
	var tags []string
	if err := repo.Tags(ctx, "", func(page []string) error {
		tags = append(tags, page...)
		return nil
	}); err != nil {
		return nil, err
	}

	repoName := repo.Reference.Repository
	results, errs := fanOut(len(tags), func(i int) ([]ArtifactInfo, error) {
		info, err := artifactInfo(ctx, registry, repoName, repo, tags[i])
		if err != nil || info == nil {
			return nil, err
		}
		return []ArtifactInfo{*info}, nil
	})

	if len(results) == 0 && len(errs) > 0 {
		return nil, errors.Join(errs...)
	}
	return results, nil
}

// artifactInfo returns the ArtifactInfo for one tag, or nil when the tag is
// simply not an oci-sync artifact (a foreign image, or a manifest we cannot
// parse). Those are normal, not errors.
func artifactInfo(ctx context.Context, registry, repoName string, repo *remote.Repository, tag string) (*ArtifactInfo, error) {
	desc, err := repo.Resolve(ctx, tag)
	if err != nil {
		return nil, fmt.Errorf("resolve %s:%s: %w", repoName, tag, err)
	}

	rc, err := repo.Fetch(ctx, desc)
	if err != nil {
		return nil, fmt.Errorf("fetch %s:%s: %w", repoName, tag, err)
	}
	defer rc.Close()
	data, err := io.ReadAll(rc)
	if err != nil {
		return nil, fmt.Errorf("read %s:%s: %w", repoName, tag, err)
	}

	var manifest ocispec.Manifest
	if err := json.Unmarshal(data, &manifest); err != nil {
		// Not a manifest we understand; treat as a foreign artifact.
		return nil, nil
	}

	val, ok := manifest.Annotations[AnnotationVersion]
	if !ok {
		return nil, nil // not pushed by oci-sync
	}

	var size int64
	if len(manifest.Layers) > 0 {
		size = manifest.Layers[0].Size
	}
	labels := make(map[string]string)
	for k, v := range manifest.Annotations {
		if !strings.HasPrefix(k, systemAnnotationPrefix) {
			labels[k] = v
		}
	}

	return &ArtifactInfo{
		FullName:  registry + "/" + repoName + ":" + tag,
		Repo:      repoName,
		Tag:       tag,
		Digest:    desc.Digest.String(),
		Encrypted: manifest.Annotations[AnnotationEncrypted] == "true",
		Version:   val,
		Size:      size,
		Labels:    labels,
	}, nil
}

// fanOut runs fn for indexes [0,n) with bounded concurrency and returns the
// concatenated results in input order plus every non-nil error. Callers
// decide whether a partial result is usable.
func fanOut(n int, fn func(i int) ([]ArtifactInfo, error)) ([]ArtifactInfo, []error) {
	if n == 0 {
		return nil, nil
	}

	out := make([][]ArtifactInfo, n)
	errs := make([]error, n)

	sem := make(chan struct{}, listConcurrency)
	var wg sync.WaitGroup
	for i := range n {
		wg.Add(1)
		go func() {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			out[i], errs[i] = fn(i)
		}()
	}
	wg.Wait()

	var results []ArtifactInfo
	var kept []error
	for i := range n {
		results = append(results, out[i]...)
		if errs[i] != nil {
			kept = append(kept, errs[i])
		}
	}
	return results, kept
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

	authClient, err := newAuthClient(host)
	if err != nil {
		return nil, err
	}
	reg.Client = authClient

	return reg, nil
}
