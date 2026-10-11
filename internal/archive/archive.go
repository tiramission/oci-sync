// Package archive provides tar.gz packing and unpacking functionality.
//
// Both directions stream: Pack writes into an io.Writer and Unpack reads
// from an io.Reader, so neither holds a whole archive in memory. A caller
// that wants the bytes still has PackBytes/UnpackBytes, which simply wrap
// a buffer.
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

// Pack packs the given path (file or directory) as a tar.gz archive written
// to w. Nothing is buffered whole: each file is copied straight through.
func Pack(w io.Writer, srcPath string) error {
	srcPath = filepath.Clean(srcPath)
	info, err := os.Stat(srcPath)
	if err != nil {
		return fmt.Errorf("stat %s: %w", srcPath, err)
	}

	gw := gzip.NewWriter(w)
	tw := tar.NewWriter(gw)

	if info.IsDir() {
		err = packDir(tw, srcPath, filepath.Base(srcPath))
	} else {
		err = packFile(tw, srcPath, filepath.Base(srcPath))
	}
	if err != nil {
		return err
	}

	if err := tw.Close(); err != nil {
		return fmt.Errorf("close tar writer: %w", err)
	}
	if err := gw.Close(); err != nil {
		return fmt.Errorf("close gzip writer: %w", err)
	}
	return nil
}

// PackBytes packs srcPath and returns the compressed bytes. It buffers the
// whole archive, so prefer Pack for anything large.
func PackBytes(srcPath string) ([]byte, error) {
	var buf bytes.Buffer
	if err := Pack(&buf, srcPath); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func packDir(tw *tar.Writer, srcDir, baseDir string) error {
	return filepath.Walk(srcDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		// Follow symlinks: they are packed as ordinary files holding the
		// target's content. Unpack only materialises regular files and
		// directories, so storing link targets would make a round trip turn
		// every link into an empty file. tar.FileInfoHeader would also size
		// the entry to the length of the link path while the body is the
		// target content, which fails with "write too long".
		if info.Mode()&os.ModeSymlink != 0 {
			resolved, err := os.Stat(path)
			if err != nil {
				return fmt.Errorf("stat symlink %s: %w", path, err)
			}
			if resolved.IsDir() {
				// A link to a directory would duplicate the subtree under a
				// second name; skip it rather than silently nesting content.
				return nil
			}
			info = resolved
		}

		// compute relative header name under baseDir
		rel, err := filepath.Rel(srcDir, path)
		if err != nil {
			return err
		}
		// Tar member names are always slash-separated, regardless of the host
		// OS. filepath.Join yields backslashes on Windows, which would collapse
		// the whole tree into single flat names when unpacked elsewhere.
		headerName := filepath.ToSlash(filepath.Join(baseDir, rel))
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

		return writeFileTo(tw, path)
	})
}

// writeFileTo appends the contents of src to tw. It closes src before
// returning so callers packing large trees do not accumulate open handles.
func writeFileTo(tw *tar.Writer, src string) error {
	f, err := os.Open(src)
	if err != nil {
		return fmt.Errorf("open %s: %w", src, err)
	}
	defer f.Close()

	if _, err := io.Copy(tw, f); err != nil {
		return fmt.Errorf("write %s to tar: %w", src, err)
	}
	return nil
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
	hdr.Name = filepath.ToSlash(name)

	if err := tw.WriteHeader(hdr); err != nil {
		return fmt.Errorf("write tar header: %w", err)
	}

	return writeFileTo(tw, srcFile)
}

// Unpack extracts a tar.gz archive read from r into destPath.
// If destPath does not exist, it will be created.
//
// Entries are written as they arrive, so the archive is never held in
// memory. A partially written destination is left behind on error: an
// interrupted pull should not look like a successful one, and removing
// files the user may have had before would be worse.
func Unpack(r io.Reader, destPath string) error {
	absDestPath, err := filepath.Abs(destPath)
	if err != nil {
		return fmt.Errorf("get absolute dest path: %w", err)
	}

	if err := os.MkdirAll(absDestPath, 0o755); err != nil {
		return fmt.Errorf("create dest dir %s: %w", absDestPath, err)
	}

	gr, err := gzip.NewReader(r)
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
			// Skip links, devices, fifos. Pack never emits them (symlinks are
			// followed and stored as regular files), so this only guards
			// against archives from other producers — materialising a link
			// here would let an archive write outside destPath.
		}
	}
	return nil
}

// UnpackBytes extracts a tar.gz archive held in data. It is the in-memory
// counterpart of Unpack; prefer Unpack for anything large.
func UnpackBytes(data []byte, destPath string) error {
	return Unpack(bytes.NewReader(data), destPath)
}
