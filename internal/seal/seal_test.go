package seal

import (
	"bytes"
	"errors"
	"testing"
)

func keys(t *testing.T) *Keyring {
	t.Helper()
	k, err := DeriveKeyring("pw", []byte("0123456789abcdef"))
	if err != nil {
		t.Fatal(err)
	}
	return k
}

func TestRoundTrip(t *testing.T) {
	k := keys(t)
	blob, _ := k.Seal("chunks/ab/x", []byte("hello"))
	got, err := k.Open("chunks/ab/x", blob)
	if err != nil || !bytes.Equal(got, []byte("hello")) {
		t.Fatalf("round trip failed: %v", err)
	}
}

// A valid ciphertext moved to a different address must be rejected.
func TestChunkSwapRejected(t *testing.T) {
	k := keys(t)
	blob, _ := k.Seal("chunks/aa/A", []byte("account balance: 100"))
	if _, err := k.Open("chunks/bb/B", blob); !errors.Is(err, ErrTampered) {
		t.Fatalf("swapped chunk accepted: %v", err)
	}
}

func TestNonceIsRandom(t *testing.T) {
	k := keys(t)
	a, _ := k.Seal("x", []byte("same"))
	b, _ := k.Seal("x", []byte("same"))
	if bytes.Equal(a, b) {
		t.Fatal("identical ciphertexts: nonce reuse")
	}
}

func TestIDsAreKeyed(t *testing.T) {
	k1 := keys(t)
	k2, _ := DeriveKeyring("other", []byte("0123456789abcdef"))
	if k1.ID([]byte("x")) == k2.ID([]byte("x")) {
		t.Fatal("chunk IDs do not depend on the key")
	}
}
