package crypto_test

import (
	"bytes"
	"crypto/rand"
	"encoding/binary"
	"errors"
	"io"
	"strings"
	"testing"

	"github.com/tiramission/oci-sync/internal/crypto"
)

const passphrase = "my-strong-passphrase"

// streamHeaderSize is len(magic) + saltSize. The reader rejects anything
// that does not start with those bytes, so the tests that corrupt a
// stream need to slice past exactly this much.
const streamHeaderSize = 36

// tagLen is the GCM tag appended to every sealed chunk.
const tagLen = 16

// seal encrypts plaintext through the streaming writer and returns the
// finished stream. Every test goes through this rather than a one-shot
// helper, because the writer only produces a readable stream once Close
// has written the header and the terminator.
func seal(t *testing.T, plaintext []byte, pass string) []byte {
	t.Helper()
	var buf bytes.Buffer
	w, err := crypto.NewEncryptPassphrase(&buf, pass)
	if err != nil {
		t.Fatalf("NewEncryptPassphrase: %v", err)
	}
	if _, err := w.Write(plaintext); err != nil {
		t.Fatalf("Write: %v", err)
	}
	if err := w.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	return buf.Bytes()
}

func open(t *testing.T, stream []byte, pass string) ([]byte, error) {
	t.Helper()
	r, err := crypto.NewDecryptPassphrase(bytes.NewReader(stream), pass)
	if err != nil {
		return nil, err
	}
	return io.ReadAll(r)
}

// TestRoundTripAcrossChunkBoundaries pins the sizes that decide how many
// chunks a stream has: nothing at all, exactly one chunk (no remainder),
// and one byte over. An off-by-one in the buffer or the terminator shows
// up at a boundary and nowhere else, so all three have to be covered.
func TestRoundTripAcrossChunkBoundaries(t *testing.T) {
	cases := []struct {
		name string
		size int
	}{
		{"empty", 0},
		{"one byte", 1},
		{"chunk minus one", crypto.ChunkSize - 1},
		{"exactly one chunk", crypto.ChunkSize},
		{"one chunk plus one", crypto.ChunkSize + 1},
		{"three chunks", crypto.ChunkSize*3 + 17},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			original := make([]byte, tc.size)
			if _, err := rand.Read(original); err != nil {
				t.Fatal(err)
			}

			stream := seal(t, original, passphrase)
			if tc.size >= 64 && bytes.Contains(stream, original[:64]) {
				t.Fatal("stream contains the plaintext verbatim")
			}

			got, err := open(t, stream, passphrase)
			if err != nil {
				t.Fatalf("decrypt: %v", err)
			}
			if !bytes.Equal(got, original) {
				t.Errorf("round trip mismatch for %d bytes", tc.size)
			}
		})
	}
}

// TestReaderYieldsPlaintextInSmallPieces checks the reader against a caller
// that copies in awkward sizes. A reader that only worked when handed the
// full chunk at once would pass the round trip above and still break
// archive.Unpack, which reads in 32 KiB blocks.
func TestReaderYieldsPlaintextInSmallPieces(t *testing.T) {
	original := make([]byte, crypto.ChunkSize+500)
	if _, err := rand.Read(original); err != nil {
		t.Fatal(err)
	}

	r, err := crypto.NewDecryptPassphrase(bytes.NewReader(seal(t, original, passphrase)), passphrase)
	if err != nil {
		t.Fatal(err)
	}

	var got bytes.Buffer
	buf := make([]byte, 32*1024)
	for {
		n, err := r.Read(buf)
		got.Write(buf[:n])
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatalf("Read: %v", err)
		}
		if n == 0 {
			t.Fatal("Read returned 0, nil and made no progress")
		}
	}
	if !bytes.Equal(got.Bytes(), original) {
		t.Error("streamed plaintext mismatch")
	}
}

func TestDecryptWrongPassphrase(t *testing.T) {
	stream := seal(t, []byte("some data to encrypt"), "correct-pass")

	_, err := open(t, stream, "wrong-pass")
	if err == nil {
		t.Fatal("expected an error when decrypting with the wrong passphrase")
	}
	if !strings.Contains(err.Error(), "wrong passphrase") {
		t.Errorf("error should point at the passphrase, got %v", err)
	}
}

// TestErrNotChunkedOnForeignStream keeps "this is some other format"
// separate from "this is ours but damaged": a caller handed an old
// one-shot ciphertext needs to be told that, not that the file is corrupt.
func TestErrNotChunkedOnForeignStream(t *testing.T) {
	_, err := open(t, []byte("this is not an oci-sync stream at all"), passphrase)
	if !errors.Is(err, crypto.ErrNotChunked) {
		t.Errorf("err = %v, want ErrNotChunked", err)
	}
}

func TestDecryptTooShort(t *testing.T) {
	if _, err := open(t, []byte("tooshort"), "pass"); err == nil {
		t.Fatal("expected an error for a stream shorter than the header")
	}
}

func TestEncryptDifferentEachTime(t *testing.T) {
	data := []byte("same plaintext")

	enc1 := seal(t, data, "same-pass")
	enc2 := seal(t, data, "same-pass")

	// Random salt, so the derived keys differ and with them the streams.
	if bytes.Equal(enc1, enc2) {
		t.Error("two streams of the same plaintext under the same passphrase should differ")
	}
}

func TestNewEncryptWriterRejectsBadParameters(t *testing.T) {
	key := make([]byte, 32)
	if _, err := crypto.NewEncryptWriter(io.Discard, key, make([]byte, 8)); err == nil {
		t.Error("expected an error for a short salt")
	}
	if _, err := crypto.NewEncryptWriter(io.Discard, make([]byte, 16), make([]byte, 32)); err == nil {
		t.Error("expected an error for a non-256-bit key")
	}
}

// TestCloseIsNotIdempotent documents why: a second Close would append a
// second terminator and leave a stream the reader cannot interpret, so
// the writer refuses instead of writing a corrupt file.
func TestCloseIsNotIdempotent(t *testing.T) {
	w, err := crypto.NewEncryptPassphrase(io.Discard, passphrase)
	if err != nil {
		t.Fatal(err)
	}
	if err := w.Close(); err != nil {
		t.Fatalf("first Close: %v", err)
	}
	if err := w.Close(); err == nil {
		t.Error("second Close should report an error")
	}
	if _, err := w.Write([]byte("late")); err == nil {
		t.Error("Write after Close should report an error")
	}
}

// TestReorderedChunksAreRejected pins the AAD binding. Each chunk's
// additional data includes its index, so swapping two same-length
// ciphertexts must fail authentication rather than silently decrypt to
// scrambled plaintext.
func TestReorderedChunksAreRejected(t *testing.T) {
	original := make([]byte, crypto.ChunkSize*2)
	if _, err := rand.Read(original); err != nil {
		t.Fatal(err)
	}
	stream := seal(t, original, passphrase)

	// Layout: header | len | body | len | body | term. Both bodies are the
	// same length because both plaintexts filled a chunk exactly, so the
	// two ciphertexts can be exchanged without touching any offset.
	bodyLen := crypto.ChunkSize + tagLen
	body0 := streamHeaderSize + 4
	body1 := body0 + 4 + bodyLen

	swapped := make([]byte, len(stream))
	copy(swapped, stream)
	copy(swapped[body0:body0+bodyLen], stream[body1:body1+bodyLen])
	copy(swapped[body1:body1+bodyLen], stream[body0:body0+bodyLen])

	_, err := open(t, swapped, passphrase)
	if err == nil {
		t.Fatal("reordering two chunks should fail authentication")
	}
	// A bare "wrong passphrase?" hint would be misleading here: the key is
	// right, so the failure has to be reported as a decryption failure.
	if !strings.Contains(err.Error(), "decrypt chunk") {
		t.Errorf("error should report an authentication failure, got %v", err)
	}
}

// TestTruncatedStreamIsReported covers the three ways a stream can end
// early. Each must be an error rather than a short read the caller could
// mistake for a complete artifact.
func TestTruncatedStreamIsReported(t *testing.T) {
	original := make([]byte, crypto.ChunkSize+64)
	if _, err := rand.Read(original); err != nil {
		t.Fatal(err)
	}
	stream := seal(t, original, passphrase)

	t.Run("no terminator", func(t *testing.T) {
		// Drop the trailing zero length. Everything before it decrypts,
		// so this is the case that would return silently truncated data.
		if _, err := open(t, stream[:len(stream)-4], passphrase); err == nil {
			t.Error("a stream without a terminator should be reported as truncated")
		}
	})

	t.Run("short chunk body", func(t *testing.T) {
		// Keep the header and the first length prefix, cut into the body.
		if _, err := open(t, stream[:streamHeaderSize+4+64], passphrase); err == nil {
			t.Error("a chunk with a missing body should be reported")
		}
	})

	t.Run("truncated header", func(t *testing.T) {
		if _, err := open(t, stream[:streamHeaderSize-1], passphrase); err == nil {
			t.Error("a stream cut inside the header should be reported")
		}
	})
}

// TestOversizedChunkLengthIsRejected guards the allocation in fill: a
// corrupt length must be refused rather than believed, or a four-byte
// field in a damaged file could ask for gigabytes.
func TestOversizedChunkLengthIsRejected(t *testing.T) {
	stream := seal(t, []byte("small"), passphrase)

	var buf bytes.Buffer
	buf.Write(stream[:streamHeaderSize]) // magic + salt
	var prefix [4]byte
	binary.BigEndian.PutUint32(prefix[:], uint32(crypto.ChunkSize+16+1))
	buf.Write(prefix[:])

	_, err := open(t, buf.Bytes(), passphrase)
	if err == nil {
		t.Fatal("an oversized chunk length should be rejected")
	}
	if !strings.Contains(err.Error(), "maximum") {
		t.Errorf("error should mention the maximum, got %v", err)
	}
}

// TestTamperedCiphertextIsRejected is the basic integrity guarantee: one
// flipped byte anywhere in the stream fails authentication.
func TestTamperedCiphertextIsRejected(t *testing.T) {
	stream := seal(t, []byte("secret data: hello, oci-sync!"), passphrase)

	tampered := make([]byte, len(stream))
	copy(tampered, stream)
	tampered[len(tampered)-8] ^= 0x01

	if _, err := open(t, tampered, passphrase); err == nil {
		t.Error("a flipped ciphertext byte should fail authentication")
	}
}
