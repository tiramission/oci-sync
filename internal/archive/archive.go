// Package archive provides tar.gz packing and unpacking functionality.
package archive

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// Pack packs the given path (file or directory) into a tar.gz archive
// and returns the compressed bytes.
func Pack(srcPath string) ([]byte, error) {
	srcPath = filepath.Clean(srcPath)
	info, err := os.Stat(srcPath)
	if err != nil {
		return nil, fmt.Errorf("stat %s: %w", srcPath, err)
	}

	var buf bytes.Buffer
	gw := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gw)

	if info.IsDir() {
		err = packDir(tw, srcPath, filepath.Base(srcPath))
	} else {
		err = packFile(tw, srcPath, filepath.Base(srcPath))
	}
	if err != nil {
		return nil, err
	}

	if err := tw.Close(); err != nil {
		return nil, fmt.Errorf("close tar writer: %w", err)
	}
	if err := gw.Close(); err != nil {
		return nil, fmt.Errorf("close gzip writer: %w", err)
	}
	return buf.Bytes(), nil
}

func packDir(tw *tar.Writer, srcDir, baseDir string) error {
	return filepath.Walk(srcDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		// compute relative header name under baseDir
		rel, err := filepath.Rel(srcDir, path)
		if err != nil {
			return err
		}
		headerName := filepath.Join(baseDir, rel)
		if info.IsDir() {
			headerName += "/"
		}

		hdr, err := tar.FileInfoHeader(info, "")
		if err != nil {
			return fmt.Errorf("create tar header for %s: %w", path, err)
		}
		hdr.Name = headerName

		if err := tw.WriteHeader(hdr); err != nil {
			return fmt.Errorf("write tar header for %s: %w", path, err)
		}

		if info.IsDir() {
			return nil
		}

		f, err := os.Open(path)
		if err != nil {
			return fmt.Errorf("open %s: %w", path, err)
		}
		defer f.Close()

		if _, err := io.Copy(tw, f); err != nil {
			return fmt.Errorf("write %s to tar: %w", path, err)
		}
		return nil
	})
}

func packFile(tw *tar.Writer, srcFile, name string) error {
	info, err := os.Stat(srcFile)
	if err != nil {
		return fmt.Errorf("stat %s: %w", srcFile, err)
	}

	hdr, err := tar.FileInfoHeader(info, "")
	if err != nil {
		return fmt.Errorf("create tar header: %w", err)
	}
	hdr.Name = name

	if err := tw.WriteHeader(hdr); err != nil {
		return fmt.Errorf("write tar header: %w", err)
	}

	f, err := os.Open(srcFile)
	if err != nil {
		return fmt.Errorf("open %s: %w", srcFile, err)
	}
	defer f.Close()

	if _, err := io.Copy(tw, f); err != nil {
		return fmt.Errorf("copy file content: %w", err)
	}
	return nil
}

// Unpack extracts the tar.gz archive bytes into destPath.
// If destPath does not exist, it will be created.
func Unpack(data []byte, destPath string) error {
	absDestPath, err := filepath.Abs(destPath)
	if err != nil {
		return fmt.Errorf("get absolute dest path: %w", err)
	}

	if err := os.MkdirAll(absDestPath, 0o755); err != nil {
		return fmt.Errorf("create dest dir %s: %w", absDestPath, err)
	}

	gr, err := gzip.NewReader(bytes.NewReader(data))
	if err != nil {
		return fmt.Errorf("create gzip reader: %w", err)
	}
	defer gr.Close()

	tr := tar.NewReader(gr)
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("read tar entry: %w", err)
		}

		// Security: prevent path traversal
		targetPath := filepath.Join(absDestPath, hdr.Name)
		if !strings.HasPrefix(targetPath, absDestPath+string(os.PathSeparator)) && targetPath != absDestPath {
			return fmt.Errorf("illegal file path in archive: %s", hdr.Name)
		}

		switch hdr.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(targetPath, hdr.FileInfo().Mode()); err != nil {
				return fmt.Errorf("create dir %s: %w", targetPath, err)
			}
		case tar.TypeReg:
			if err := os.MkdirAll(filepath.Dir(targetPath), 0o755); err != nil {
				return fmt.Errorf("create parent dir for %s: %w", targetPath, err)
			}
			f, err := os.OpenFile(targetPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, hdr.FileInfo().Mode())
			if err != nil {
				return fmt.Errorf("create file %s: %w", targetPath, err)
			}
			if _, err := io.Copy(f, tr); err != nil {
				f.Close()
				return fmt.Errorf("write file %s: %w", targetPath, err)
			}
			f.Close()
		default:
			// skip unsupported types (symlinks, devices, etc.)
		}
	}
	return nil
}
