package oci

import (
	"archive/tar"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/go-containerregistry/pkg/authn"
	"github.com/google/go-containerregistry/pkg/name"
	v1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/google/go-containerregistry/pkg/v1/empty"
	"github.com/google/go-containerregistry/pkg/v1/mutate"
	"github.com/google/go-containerregistry/pkg/v1/remote"
	"github.com/google/go-containerregistry/pkg/v1/tarball"
)

type Descriptor struct {
	MediaType   string            `json:"mediaType"`
	Digest      string            `json:"digest"`
	Size        int64             `json:"size"`
	Annotations map[string]string `json:"annotations,omitempty"`
}

type OCIManifest struct {
	SchemaVersion int          `json:"schemaVersion"`
	MediaType     string       `json:"mediaType"`
	Config        Descriptor   `json:"config"`
	Layers        []Descriptor `json:"layers"`
}

func Push(srcDir, ref string) error {
	scheme, image, tag, localRoot, err := parseRef(ref)
	if err != nil {
		return err
	}
	if scheme == "file" {
		return pushLocalFileLayout(srcDir, localRoot, image, tag)
	}
	if isRegistryRef(image) {
		return pushRemote(srcDir, image, tag)
	}
	return pushLocalStore(srcDir, image, tag)
}

func Pull(ref, destDir string) error {
	scheme, image, tag, localRoot, err := parseRef(ref)
	if err != nil {
		return err
	}
	if scheme == "file" {
		return pullLocalFileLayout(localRoot, image, tag, destDir)
	}
	if isRegistryRef(image) {
		return pullRemote(image, tag, destDir)
	}
	return pullLocalStore(image, tag, destDir)
}

func pushLocalToRoot(srcDir, root, image, tag string) error {
	info, err := os.Stat(srcDir)
	if err != nil {
		return err
	}
	if !info.IsDir() {
		return fmt.Errorf("source must be directory: %s", srcDir)
	}
	if tag == "" {
		tag = "latest"
	}
	if root == "~/.oci-sync/store" {
		root, err = storeBasePath()
		if err != nil {
			return err
		}
		root = filepath.Join(root, filepath.FromSlash(image), tag)
	}
	if err := os.MkdirAll(root, 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(root, "oci-layout"), []byte("{\"imageLayoutVersion\": \"1.0.0\"}\n"), 0o644); err != nil {
		return err
	}
	blobDir := filepath.Join(root, "blobs", "sha256")
	if err := os.MkdirAll(blobDir, 0o755); err != nil {
		return err
	}
	layerPath, _, err := createLayerFromDir(srcDir, root)
	if err != nil {
		return err
	}
	layerData, err := os.ReadFile(layerPath)
	if err != nil {
		return err
	}
	layerDigest, layerSize := digestBytes(layerData)
	layerFile := filepath.Join(blobDir, strings.TrimPrefix(layerDigest, "sha256:")+".tar")
	if err := os.WriteFile(layerFile, layerData, 0o644); err != nil {
		return err
	}
	config := map[string]interface{}{
		"created": time.Now().UTC().Format(time.RFC3339Nano),
		"os":      "linux",
		"arch":    "amd64",
		"source":  srcDir,
	}
	confBytes, err := json.Marshal(config)
	if err != nil {
		return err
	}
	confDigest, confSize := digestBytes(confBytes)
	configFile := filepath.Join(blobDir, strings.TrimPrefix(confDigest, "sha256:")+".json")
	if err := os.WriteFile(configFile, confBytes, 0o644); err != nil {
		return err
	}
	manifest := OCIManifest{
		SchemaVersion: 2,
		MediaType:     "application/vnd.oci.image.manifest.v1+json",
		Config: Descriptor{
			MediaType: "application/vnd.oci.image.config.v1+json",
			Digest:    confDigest,
			Size:      confSize,
		},
		Layers: []Descriptor{{
			MediaType: "application/vnd.oci.image.layer.v1.tar",
			Digest:    layerDigest,
			Size:      layerSize,
		}},
	}
	manifestBytes, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return err
	}
	manifestDigest, manifestSize := digestBytes(manifestBytes)
	manifestPath := filepath.Join(blobDir, strings.TrimPrefix(manifestDigest, "sha256:")+".json")
	if err := os.WriteFile(manifestPath, manifestBytes, 0o644); err != nil {
		return err
	}
	index := map[string]interface{}{
		"schemaVersion": 2,
		"manifests": []interface{}{
			map[string]interface{}{
				"mediaType":   "application/vnd.oci.image.manifest.v1+json",
				"digest":      manifestDigest,
				"size":        manifestSize,
				"annotations": map[string]interface{}{"org.opencontainers.image.ref.name": fmt.Sprintf("%s:%s", image, tag)},
			},
		},
	}
	indexBytes, err := json.MarshalIndent(index, "", "  ")
	if err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(root, "index.json"), indexBytes, 0o644); err != nil {
		return err
	}
	fmt.Printf("saved oci-layout => %s\n", filepath.Join(root, "oci-layout"))
	return nil
}

func pushLocalStore(srcDir, image, tag string) error {
	return pushLocalToRoot(srcDir, "~/.oci-sync/store", image, tag)
}

func pullLocalStore(image, tag, destDir string) error {
	return pullLocalFromRoot("~/.oci-sync/store", image, tag, destDir)
}

func pushLocalFileLayout(srcDir, root, image, tag string) error {
	return pushLocalToRoot(srcDir, root, image, tag)
}

func pullLocalFileLayout(root, image, tag, destDir string) error {
	return pullLocalFromRoot(root, image, tag, destDir)
}

func pullLocalFromRoot(root, image, tag, destDir string) error {
	if tag == "" {
		tag = "latest"
	}
	if root == "~/.oci-sync/store" {
		var err error
		root, err = storeBasePath()
		if err != nil {
			return err
		}
		root = filepath.Join(root, filepath.FromSlash(image), tag)
	}
	indexPath := filepath.Join(root, "index.json")
	indexBytes, err := os.ReadFile(indexPath)
	if err != nil {
		return fmt.Errorf("failed to read index: %w", err)
	}
	var index struct {
		SchemaVersion int `json:"schemaVersion"`
		Manifests     []struct {
			Digest string `json:"digest"`
		} `json:"manifests"`
	}
	if err := json.Unmarshal(indexBytes, &index); err != nil {
		return fmt.Errorf("failed to parse index: %w", err)
	}
	if len(index.Manifests) == 0 {
		return errors.New("index has no manifests")
	}
	manifestDigest := index.Manifests[0].Digest
	if !strings.HasPrefix(manifestDigest, "sha256:") {
		return fmt.Errorf("unsupported manifest digest: %s", manifestDigest)
	}
	manifestPath := filepath.Join(root, "blobs", "sha256", strings.TrimPrefix(manifestDigest, "sha256:")+".json")
	manifestBytes, err := os.ReadFile(manifestPath)
	if err != nil {
		return fmt.Errorf("failed to read manifest: %w", err)
	}
	var manifest OCIManifest
	if err := json.Unmarshal(manifestBytes, &manifest); err != nil {
		return fmt.Errorf("failed to parse manifest: %w", err)
	}
	if len(manifest.Layers) == 0 {
		return errors.New("manifest has no layers")
	}
	layer := manifest.Layers[0]
	if !strings.HasPrefix(layer.Digest, "sha256:") {
		return fmt.Errorf("unsupported digest format: %s", layer.Digest)
	}
	digestName := strings.TrimPrefix(layer.Digest, "sha256:")
	layerPath := filepath.Join(root, "blobs", "sha256", digestName+".tar")
	if _, err := os.Stat(layerPath); err != nil {
		return fmt.Errorf("layer file not found: %w", err)
	}
	if err := os.MkdirAll(destDir, 0o755); err != nil {
		return err
	}
	return extractTar(layerPath, destDir)
}

func pushRemote(srcDir, image, tag string) error {
	info, err := os.Stat(srcDir)
	if err != nil {
		return err
	}
	if !info.IsDir() {
		return fmt.Errorf("source must be directory: %s", srcDir)
	}
	tmpDir, err := os.MkdirTemp("", "oci-sync-registry")
	if err != nil {
		return err
	}
	defer os.RemoveAll(tmpDir)
	layerPath, _, err := createLayerFromDir(srcDir, tmpDir)
	if err != nil {
		return err
	}
	layer, err := tarball.LayerFromFile(layerPath)
	if err != nil {
		return err
	}
	img := empty.Image
	img, err = mutate.AppendLayers(img, layer)
	if err != nil {
		return err
	}
	diffID, err := layer.DiffID()
	if err != nil {
		return err
	}
	cfg := &v1.ConfigFile{
		Created:      v1.Time{Time: time.Now().UTC()},
		Architecture: "amd64",
		OS:           "linux",
		RootFS: v1.RootFS{
			Type:    "layers",
			DiffIDs: []v1.Hash{diffID},
		},
		History: []v1.History{{Created: v1.Time{Time: time.Now().UTC()}, CreatedBy: "oci-sync"}},
	}
	img, err = mutate.ConfigFile(img, cfg)
	if err != nil {
		return err
	}
	refStr := fmt.Sprintf("%s:%s", image, tag)
	remoteTag, err := name.NewTag(refStr, name.WeakValidation, name.Insecure)
	if err != nil {
		return err
	}
	tc := &http.Transport{Proxy: http.ProxyFromEnvironment}
	if err := remote.Write(remoteTag, img, remote.WithAuth(authn.Anonymous), remote.WithTransport(tc)); err != nil {
		return err
	}
	fmt.Printf("pushed remote image %s\n", refStr)
	return nil
}

func pullRemote(image, tag, destDir string) error {
	refStr := fmt.Sprintf("%s:%s", image, tag)
	remoteTag, err := name.NewTag(refStr, name.WeakValidation, name.Insecure)
	if err != nil {
		return err
	}
	tc := &http.Transport{Proxy: http.ProxyFromEnvironment}
	img, err := remote.Image(remoteTag, remote.WithAuth(authn.Anonymous), remote.WithTransport(tc))
	if err != nil {
		return err
	}
	layers, err := img.Layers()
	if err != nil {
		return err
	}
	if len(layers) == 0 {
		return errors.New("remote image has no layers")
	}
	if err := os.MkdirAll(destDir, 0o755); err != nil {
		return err
	}
	layer := layers[0]
	r, err := layer.Uncompressed()
	if err != nil {
		return err
	}
	defer r.Close()
	tarPath := filepath.Join(destDir, ".oci-sync-layer.tmp.tar")
	f, err := os.Create(tarPath)
	if err != nil {
		return err
	}
	if _, err := io.Copy(f, r); err != nil {
		f.Close()
		return err
	}
	f.Close()
	defer os.Remove(tarPath)
	return extractTar(tarPath, destDir)
}

func parseRef(ref string) (scheme, image, tag, localRoot string, err error) {
	if strings.HasPrefix(ref, "oci://") {
		image, tag, err = parseOCIRef(ref)
		if err != nil {
			return "", "", "", "", err
		}
		return "oci", image, tag, "", nil
	}
	if strings.HasPrefix(ref, "file://") {
		body := strings.TrimPrefix(ref, "file://")
		if body == "" {
			return "", "", "", "", errors.New("invalid file reference")
		}
		idx := strings.Index(body, ":")
		if idx < 0 {
			return "", "", "", "", errors.New("file reference must be file://<path-dir>:<image-name>")
		}
		localRoot = body[:idx]
		imagePart := body[idx+1:]
		if localRoot == "" || imagePart == "" {
			return "", "", "", "", errors.New("invalid file reference, expected file://<path-dir>:<image-name>")
		}
		image, tag, err = parseOCIRef("oci://" + imagePart)
		if err != nil {
			return "", "", "", "", err
		}
		return "file", image, tag, localRoot, nil
	}
	return "", "", "", "", fmt.Errorf("unsupported ref scheme (use oci:// or file://): %s", ref)
}

func parseOCIRef(ref string) (string, string, error) {
	const prefix = "oci://"
	if !strings.HasPrefix(ref, prefix) {
		return "", "", fmt.Errorf("invalid oci ref (must start with oci://): %s", ref)
	}
	refBody := strings.TrimPrefix(ref, prefix)
	if refBody == "" {
		return "", "", errors.New("empty oci reference")
	}
	slashPos := strings.LastIndex(refBody, "/")
	colonPos := strings.LastIndex(refBody, ":")
	var image, tag string
	if colonPos > 0 && colonPos > slashPos {
		image = refBody[:colonPos]
		tag = refBody[colonPos+1:]
		if tag == "" {
			return "", "", errors.New("tag cannot be empty")
		}
	} else {
		image = refBody
		tag = "latest"
	}
	if image == "" {
		return "", "", errors.New("image name empty")
	}
	return image, tag, nil
}

func isRegistryRef(image string) bool {
	parts := strings.SplitN(image, "/", 2)
	host := parts[0]
	if strings.Contains(host, ".") || strings.Contains(host, ":") || host == "localhost" {
		return true
	}
	return false
}

func pushFile(srcDir, targetPath string) error {
	info, err := os.Stat(srcDir)
	if err != nil {
		return err
	}
	if !info.IsDir() {
		return fmt.Errorf("source must be directory: %s", srcDir)
	}
	if err := copyDir(srcDir, targetPath); err != nil {
		return err
	}
	fmt.Printf("pushed file path %s -> %s\n", srcDir, targetPath)
	return nil
}

func pullFile(srcPath, destDir string) error {
	if err := copyDir(srcPath, destDir); err != nil {
		return err
	}
	fmt.Printf("pulled file path %s -> %s\n", srcPath, destDir)
	return nil
}

func copyDir(src, dst string) error {
	info, err := os.Stat(src)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(dst, info.Mode()); err != nil {
		return err
	}
	return filepath.Walk(src, func(path string, fi os.FileInfo, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		target := filepath.Join(dst, rel)
		if fi.IsDir() {
			return os.MkdirAll(target, fi.Mode())
		}
		if fi.Mode().IsRegular() {
			data, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			return os.WriteFile(target, data, fi.Mode())
		}
		return nil
	})
}

func storeBasePath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	path := filepath.Join(home, ".oci-sync", "store")
	if err := os.MkdirAll(path, 0o755); err != nil {
		return "", err
	}
	return path, nil
}

func createLayerFromDir(srcDir, imageDir string) (string, Descriptor, error) {
	tmpFile, err := os.CreateTemp("", "oci-sync-layer-*.tar")
	if err != nil {
		return "", Descriptor{}, err
	}
	tmpPath := tmpFile.Name()
	tarWriter := tar.NewWriter(tmpFile)
	root := filepath.Base(srcDir)
	err = filepath.Walk(srcDir, func(path string, fi os.FileInfo, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		rel, err := filepath.Rel(srcDir, path)
		if err != nil {
			return err
		}
		if rel == "." {
			rel = root
		} else {
			rel = filepath.Join(root, rel)
		}
		header, err := tar.FileInfoHeader(fi, "")
		if err != nil {
			return err
		}
		header.Name = filepath.ToSlash(rel)
		if err := tarWriter.WriteHeader(header); err != nil {
			return err
		}
		if fi.Mode().IsRegular() {
			f, err := os.Open(path)
			if err != nil {
				return err
			}
			defer f.Close()
			if _, err := io.Copy(tarWriter, f); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		tarWriter.Close()
		tmpFile.Close()
		os.Remove(tmpPath)
		return "", Descriptor{}, err
	}
	tarWriter.Close()
	tmpFile.Close()
	data, err := os.ReadFile(tmpPath)
	if err != nil {
		os.Remove(tmpPath)
		return "", Descriptor{}, err
	}
	digest, size := digestBytes(data)
	layerDir := filepath.Join(imageDir, "layers")
	if err := os.MkdirAll(layerDir, 0o755); err != nil {
		os.Remove(tmpPath)
		return "", Descriptor{}, err
	}
	layerFile := filepath.Join(layerDir, strings.TrimPrefix(digest, "sha256:")+".tar")
	if err := os.WriteFile(layerFile, data, 0o644); err != nil {
		os.Remove(tmpPath)
		return "", Descriptor{}, err
	}
	os.Remove(tmpPath)
	desc := Descriptor{MediaType: "application/vnd.oci.image.layer.v1.tar", Digest: digest, Size: size, Annotations: map[string]string{"org.opencontainers.image.title": "data"}}
	return layerFile, desc, nil
}

func digestBytes(b []byte) (string, int64) {
	h := sha256.Sum256(b)
	return "sha256:" + hex.EncodeToString(h[:]), int64(len(b))
}

func extractTar(tarPath, destDir string) error {
	f, err := os.Open(tarPath)
	if err != nil {
		return err
	}
	defer f.Close()
	tr := tar.NewReader(f)
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}
		target := filepath.Join(destDir, filepath.FromSlash(hdr.Name))
		switch hdr.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(target, os.FileMode(hdr.Mode)); err != nil {
				return err
			}
		case tar.TypeReg, tar.TypeRegA:
			if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
				return err
			}
			out, err := os.OpenFile(target, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, os.FileMode(hdr.Mode))
			if err != nil {
				return err
			}
			if _, err := io.Copy(out, tr); err != nil {
				out.Close()
				return err
			}
			out.Close()
		case tar.TypeSymlink:
			if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
				return err
			}
			if err := os.Symlink(hdr.Linkname, target); err != nil {
				return err
			}
		default:
			// skip unsupported
		}
	}
	return nil
}
