// Package crypto provides streaming AES-256-GCM encryption and decryption
// with scrypt-based key derivation from a passphrase.
//
// Unlike a one-shot AEAD over the whole message, EncryptWriter seals
// fixed-size chunks independently, so neither side ever holds more than one
// chunk in memory. That is what lets push and pull work on blobs larger
// than RAM.
//
// # Stream format
//
//	"OCS1" | salt(32B) | ( u32be ciphertextLen | ciphertext+tag )*
//
// A chunk length of zero ends the stream, so the reader never has to guess
// which chunk was the last one. Each chunk carries a 16-byte GCM tag.
//
// # Nonces
//
// Every chunk of a stream gets a distinct nonce, derived from its index
// alone. Reusing a nonce under one key breaks GCM, but a key is derived
// from a random 32-byte salt, so keys — and therefore nonce spaces — never
// repeat between streams. Within one stream the index is unique, which is
// what the rule actually requires.
package crypto

import (
	"bufio"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/binary"
	"errors"
	"fmt"
	"io"

	"golang.org/x/crypto/scrypt"
)

const (
	saltSize = 32
	keySize  = 32 // AES-256

	// magic identifies the stream format, so the reader can tell a chunked
	// stream from anything else instead of guessing from its length.
	magic = "OCS1"

	// headerSize is len(magic) + saltSize.
	headerSize = 4 + saltSize

	// lenSize is the per-chunk big-endian length prefix.
	lenSize = 4

	// tagSize is the GCM authentication tag appended to every chunk.
	tagSize = 16

	// scrypt parameters (N=2^15, r=8, p=1) — OWASP recommended minimum
	scryptN = 1 << 15
	scryptR = 8
	scryptP = 1
)

// ChunkSize is the plaintext bytes sealed as one AEAD message. It bounds
// memory on both sides: EncryptWriter and DecryptReader hold one chunk at
// most. At 1 MiB the per-chunk overhead (a 4-byte length plus a 16-byte
// tag) stays far below 0.01%.
const ChunkSize = 1 << 20

// ErrNotChunked reports a stream that does not carry the format magic. It
// is distinct from a parse error so the caller can tell "this is some
// other format" from "this is ours but damaged".
var ErrNotChunked = errors.New("not an oci-sync chunked stream")

// EncryptWriter seals everything written to it into a chunked stream.
// Close must be called: it writes the header, flushes the final chunk and
// appends the terminator. Nothing before that is a readable stream,
// because the header carries the salt the chunks are keyed with.
type EncryptWriter struct {
	w       *bufio.Writer
	gcm     cipher.AEAD
	salt    []byte
	header  []byte
	index   uint32
	started bool
	closed  bool

	plain  [ChunkSize]byte
	cipher [ChunkSize + tagSize]byte
	n      int // plaintext bytes buffered in plain
}

// NewEncryptWriter returns a writer that encrypts into w. key must be 32
// bytes and salt the 32 bytes it was derived from; the salt is stored in
// the stream header so a reader can derive the same key.
func NewEncryptWriter(w io.Writer, key, salt []byte) (*EncryptWriter, error) {
	if len(salt) != saltSize {
		return nil, fmt.Errorf("salt must be %d bytes, got %d", saltSize, len(salt))
	}
	gcm, err := newGCM(key)
	if err != nil {
		return nil, err
	}

	header := make([]byte, headerSize)
	copy(header[:4], magic)
	copy(header[4:], salt)

	return &EncryptWriter{
		w:      bufio.NewWriterSize(w, ChunkSize),
		gcm:    gcm,
		salt:   salt,
		header: header,
	}, nil
}

// NewEncryptPassphrase derives a key from passphrase with a fresh random
// salt and returns a writer ready to encrypt into w.
func NewEncryptPassphrase(w io.Writer, passphrase string) (*EncryptWriter, error) {
	salt := make([]byte, saltSize)
	if _, err := io.ReadFull(rand.Reader, salt); err != nil {
		return nil, fmt.Errorf("generate salt: %w", err)
	}
	key, err := deriveKey(passphrase, salt)
	if err != nil {
		return nil, err
	}
	return NewEncryptWriter(w, key, salt)
}

// Write buffers p and seals a chunk whenever a full one is ready. It never
// returns an error for a short write of p into the internal buffer; the
// error surfaces when the sealed chunk cannot be written out.
func (e *EncryptWriter) Write(p []byte) (int, error) {
	if e.closed {
		return 0, errors.New("crypto: write after close")
	}
	written := 0
	for len(p) > 0 {
		n := copy(e.plain[e.n:], p)
		e.n += n
		p = p[n:]
		written += n
		if e.n == ChunkSize {
			if err := e.flush(); err != nil {
				return written, err
			}
		}
	}
	return written, nil
}

// Close writes the header, seals any buffered plaintext and appends the
// terminator. It is not idempotent: a second call reports an error rather
// than appending a second terminator, because such a stream cannot be
// read back.
func (e *EncryptWriter) Close() error {
	if e.closed {
		return errors.New("crypto: close of a closed writer")
	}
	if err := e.start(); err != nil {
		return err
	}
	if e.n > 0 {
		if err := e.flush(); err != nil {
			return err
		}
	}
	// A zero length marks the end, so the reader knows the plaintext stops
	// here rather than inferring it from EOF.
	var term [lenSize]byte
	if _, err := e.w.Write(term[:]); err != nil {
		return fmt.Errorf("write terminator: %w", err)
	}
	if err := e.w.Flush(); err != nil {
		return fmt.Errorf("flush stream: %w", err)
	}
	e.closed = true
	return nil
}

// Salt returns the salt this stream is encrypted under.
func (e *EncryptWriter) Salt() []byte { return e.salt }

func (e *EncryptWriter) start() error {
	if e.started {
		return nil
	}
	if _, err := e.w.Write(e.header); err != nil {
		return fmt.Errorf("write stream header: %w", err)
	}
	e.started = true
	return nil
}

func (e *EncryptWriter) flush() error {
	if err := e.start(); err != nil {
		return err
	}

	sealed := e.gcm.Seal(e.cipher[:0], chunkNonce(e.index), e.plain[:e.n], aad(e.index))

	var prefix [lenSize]byte
	binary.BigEndian.PutUint32(prefix[:], uint32(len(sealed)))
	if _, err := e.w.Write(prefix[:]); err != nil {
		return fmt.Errorf("write chunk %d length: %w", e.index, err)
	}
	if _, err := e.w.Write(sealed); err != nil {
		return fmt.Errorf("write chunk %d: %w", e.index, err)
	}

	e.index++
	e.n = 0
	return nil
}

// DecryptReader reads a chunked stream produced by EncryptWriter and
// yields its plaintext. Each chunk is authenticated before any of its
// plaintext is handed out, so tampering fails before the caller can act on
// wrong bytes.
type DecryptReader struct {
	r    *bufio.Reader
	gcm  cipher.AEAD
	salt []byte

	plain [ChunkSize]byte
	off   int // read position within plain
	avail int // plaintext bytes left in plain

	index  uint32
	closed bool
}

// NewDecryptPassphrase reads a chunked stream from r and derives the key
// from passphrase and the stream's own salt.
func NewDecryptPassphrase(r io.Reader, passphrase string) (*DecryptReader, error) {
	br := bufio.NewReaderSize(r, ChunkSize)

	header := make([]byte, headerSize)
	if _, err := io.ReadFull(br, header); err != nil {
		return nil, fmt.Errorf("read stream header: %w", err)
	}
	if string(header[:4]) != magic {
		return nil, ErrNotChunked
	}
	salt := header[4:]

	key, err := deriveKey(passphrase, salt)
	if err != nil {
		return nil, err
	}
	gcm, err := newGCM(key)
	if err != nil {
		return nil, err
	}

	return &DecryptReader{r: br, gcm: gcm, salt: salt}, nil
}

// Salt returns the salt the stream was encrypted under.
func (d *DecryptReader) Salt() []byte { return d.salt }

func (d *DecryptReader) Read(p []byte) (int, error) {
	if len(p) == 0 {
		return 0, nil
	}
	for d.avail == 0 {
		if d.closed {
			return 0, io.EOF
		}
		if err := d.fill(); err != nil {
			return 0, err
		}
	}

	n := copy(p, d.plain[d.off:d.off+d.avail])
	d.off += n
	d.avail -= n
	return n, nil
}

func (d *DecryptReader) fill() error {
	var prefix [lenSize]byte
	if _, err := io.ReadFull(d.r, prefix[:]); err != nil {
		if err == io.EOF {
			// A stream that ends without a terminator is truncated: the
			// writer always appends one, so EOF here means bytes are
			// missing and the plaintext we have is incomplete.
			return fmt.Errorf("stream ends without a terminator after %d chunks", d.index)
		}
		return fmt.Errorf("read chunk %d length: %w", d.index, err)
	}

	size := binary.BigEndian.Uint32(prefix[:])
	if size == 0 {
		d.closed = true
		return io.EOF
	}
	// A chunk can never exceed one full chunk plus its tag, so an
	// oversized length means the stream is corrupt rather than that this
	// build allocated for a huge buffer.
	if size > ChunkSize+tagSize {
		return fmt.Errorf("chunk %d claims %d bytes, more than the %d maximum", d.index, size, ChunkSize+tagSize)
	}

	buf := make([]byte, size)
	if _, err := io.ReadFull(d.r, buf); err == io.ErrUnexpectedEOF {
		return fmt.Errorf("chunk %d is truncated: got %d bytes, want %d", d.index, len(buf), size)
	} else if err != nil {
		return fmt.Errorf("read chunk %d: %w", d.index, err)
	}

	plain, err := d.gcm.Open(d.plain[:0], chunkNonce(d.index), buf, aad(d.index))
	if err != nil {
		return fmt.Errorf("decrypt chunk %d: %w (wrong passphrase?)", d.index, err)
	}

	d.off = 0
	d.avail = len(plain)
	d.index++
	return nil
}

// chunkNonce derives the nonce for a chunk from its index. The key behind
// it is unique to one stream, so indices need not repeat across streams.
func chunkNonce(index uint32) []byte {
	var nonce [12]byte
	binary.BigEndian.PutUint32(nonce[8:], index)
	return nonce[:]
}

// aad binds a chunk's position to its plaintext, so a chunk cannot be
// moved to another position or another stream without failing
// authentication.
func aad(index uint32) []byte {
	return binary.BigEndian.AppendUint32([]byte(magic), index)
}

func newGCM(key []byte) (cipher.AEAD, error) {
	if len(key) != keySize {
		return nil, fmt.Errorf("key must be %d bytes, got %d", keySize, len(key))
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("create AES cipher: %w", err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("create GCM: %w", err)
	}
	return gcm, nil
}

// deriveKey derives a 32-byte AES key from a passphrase and salt using scrypt.
func deriveKey(passphrase string, salt []byte) ([]byte, error) {
	key, err := scrypt.Key([]byte(passphrase), salt, scryptN, scryptR, scryptP, keySize)
	if err != nil {
		return nil, fmt.Errorf("scrypt key derivation: %w", err)
	}
	return key, nil
}
