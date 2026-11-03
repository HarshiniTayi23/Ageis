// Package seal handles key derivation, chunk addressing and authenticated
// encryption.
//
// Design:
//   - A user passphrase is stretched with PBKDF2-HMAC-SHA256 (600k iterations,
//     per-repository random salt) into a 32-byte master key.
//   - Two independent sub-keys are derived from the master key with HMAC:
//     one for chunk IDs and one for encryption (key separation).
//   - Chunk ID = HMAC-SHA256(idKey, plaintext). A keyed hash (not a bare
//     SHA-256) means someone who can read the bucket cannot confirm whether a
//     known file is in the backup by hashing it themselves.
//   - Each chunk is encrypted with AES-256-GCM. The chunk ID is passed as
//     associated data, so a ciphertext is cryptographically bound to its
//     address: an attacker cannot swap chunk A's bytes into chunk B's slot.
package seal

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/pbkdf2"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
)

const (
	KeySize    = 32
	SaltSize   = 16
	Iterations = 600_000 // OWASP 2023 recommendation for PBKDF2-HMAC-SHA256
)

var ErrTampered = errors.New("seal: ciphertext failed authentication (tampered or wrong key)")

// Keyring holds the derived sub-keys for a repository.
type Keyring struct {
	idKey  []byte
	encKey []byte
}

// NewSalt returns a fresh random salt for a new repository.
func NewSalt() ([]byte, error) {
	s := make([]byte, SaltSize)
	_, err := rand.Read(s)
	return s, err
}

// DeriveKeyring stretches a passphrase into a Keyring.
func DeriveKeyring(passphrase string, salt []byte) (*Keyring, error) {
	if passphrase == "" {
		return nil, errors.New("seal: empty passphrase")
	}
	master, err := pbkdf2.Key(sha256.New, passphrase, salt, Iterations, KeySize)
	if err != nil {
		return nil, err
	}
	return &Keyring{
		idKey:  mac(master, []byte("aegis/v1/chunk-id")),
		encKey: mac(master, []byte("aegis/v1/encrypt")),
	}, nil
}

func mac(key, msg []byte) []byte {
	h := hmac.New(sha256.New, key)
	h.Write(msg)
	return h.Sum(nil)
}

// ID returns the content address of a plaintext chunk.
func (k *Keyring) ID(plain []byte) string {
	return hex.EncodeToString(mac(k.idKey, plain))
}

// Seal encrypts plain and binds it to label (normally the object key).
// Output layout: nonce(12) || ciphertext || tag(16).
func (k *Keyring) Seal(label string, plain []byte) ([]byte, error) {
	aead, err := k.aead()
	if err != nil {
		return nil, err
	}
	nonce := make([]byte, aead.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return nil, err
	}
	return aead.Seal(nonce, nonce, plain, []byte(label)), nil
}

// Open decrypts and authenticates a blob produced by Seal.
func (k *Keyring) Open(label string, blob []byte) ([]byte, error) {
	aead, err := k.aead()
	if err != nil {
		return nil, err
	}
	ns := aead.NonceSize()
	if len(blob) < ns+aead.Overhead() {
		return nil, fmt.Errorf("%w: blob too short", ErrTampered)
	}
	plain, err := aead.Open(nil, blob[:ns], blob[ns:], []byte(label))
	if err != nil {
		return nil, ErrTampered
	}
	return plain, nil
}

func (k *Keyring) aead() (cipher.AEAD, error) {
	block, err := aes.NewCipher(k.encKey)
	if err != nil {
		return nil, err
	}
	return cipher.NewGCM(block)
}
