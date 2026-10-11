package cmd

import (
	"context"
	"fmt"
	"io"
	"os"

	"github.com/tiramission/oci-sync/internal/archive"
	"github.com/tiramission/oci-sync/internal/crypto"
	"github.com/tiramission/oci-sync/internal/oci"
)

// Streaming push and pull stage the artifact layer on disk instead of
// holding it in memory.
//
// The staging is not an optimisation, it is a requirement: a registry
// addresses a blob by the digest of its contents, so the digest must be
// known before the upload begins. Anything whose digest is not yet known —
// a freshly packed tar.gz, a just-encrypted stream — cannot be uploaded
// until it has been written out somewhere and read back. A file is that
// somewhere, and it costs disk rather than RAM.
//
// Every stage past the download reads the staged file as a stream, so peak
// memory is one chunk (crypto.ChunkSize) no matter how large the artifact
// is.

// stageTempFile creates an empty file to stage an artifact layer in. The
// caller removes it; the name is prefixed so a crash leaves something
// identifiable rather than an anonymous blob in /tmp.
func stageTempFile(prefix string) (string, error) {
	f, err := os.CreateTemp("", prefix+"-*.tmp")
	if err != nil {
		return "", fmt.Errorf("create staging file: %w", err)
	}
	path := f.Name()
	if err := f.Close(); err != nil {
		os.Remove(path)
		return "", fmt.Errorf("create staging file: %w", err)
	}
	return path, nil
}

// stageWriter packs localPath as tar.gz into a staging file, encrypting it
// on the way through when a passphrase is given. It returns the staged
// path and whether the content is encrypted.
//
// Both steps stream, so packing a large tree never builds the archive in
// memory.
func stageWriter(localPath, passphrase string) (path string, encrypted bool, err error) {
	staged, err := stageTempFile("oci-sync-push")
	if err != nil {
		return "", false, err
	}

	f, err := os.Create(staged)
	if err != nil {
		os.Remove(staged)
		return "", false, fmt.Errorf("create staging file %s: %w", staged, err)
	}
	defer f.Close()

	encrypted = passphrase != ""

	// The chain is pack -> (encrypt) -> file. Each stage writes into the
	// next, so the only thing held in memory is one chunk at a time.
	if encrypted {
		ew, err := crypto.NewEncryptPassphrase(f, passphrase)
		if err != nil {
			os.Remove(staged)
			return "", false, err
		}
		if err := packInto(ew, localPath); err != nil {
			ew.Close()
			os.Remove(staged)
			return "", false, err
		}
		// Close writes the stream header and terminator; without it the
		// file is not a readable stream.
		if err := ew.Close(); err != nil {
			os.Remove(staged)
			return "", false, err
		}
	} else if err := packInto(f, localPath); err != nil {
		os.Remove(staged)
		return "", false, err
	}

	if err := f.Close(); err != nil {
		os.Remove(staged)
		return "", false, fmt.Errorf("close staging file %s: %w", staged, err)
	}
	return staged, encrypted, nil
}

// packInto writes localPath as a tar.gz archive into w.
func packInto(w io.Writer, localPath string) error {
	return archive.Pack(w, localPath)
}

// pullToStaged downloads ref's layer into the staging file path.
func pullToStaged(ctx context.Context, path, remotePath string, report func(PullStage)) error {
	_, err := oci.PullToFile(ctx, path, remotePath, func(done, total int64) {
		report(PullStage{Phase: "download", Done: done, Total: total})
	})
	return err
}

// openDecrypted opens the staged layer for reading, decrypting it on the
// way through when it is encrypted. The caller closes the result.
func openDecrypted(path, passphrase string, encrypted bool) (io.ReadCloser, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open staging file %s: %w", path, err)
	}

	if !encrypted {
		return f, nil
	}

	dr, err := crypto.NewDecryptPassphrase(f, passphrase)
	if err != nil {
		f.Close()
		return nil, fmt.Errorf("decryption failed: %w", err)
	}
	return &decryptReadCloser{Reader: dr, file: f}, nil
}

// decryptReadCloser closes the underlying file along with the reader, which
// crypto.DecryptReader does not own.
type decryptReadCloser struct {
	io.Reader
	file *os.File
}

func (d *decryptReadCloser) Close() error { return d.file.Close() }

func fileSize(path string) (int64, error) {
	fi, err := os.Stat(path)
	if err != nil {
		return 0, fmt.Errorf("stat staging file %s: %w", path, err)
	}
	return fi.Size(), nil
}
