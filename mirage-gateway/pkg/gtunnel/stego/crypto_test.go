package stego

import (
	"testing"

	"pgregory.net/rapid"
)

// TestProperty14_ChaCha20Poly1305RoundTrip verifies that for any 32-byte key
// and any plaintext, Encrypt followed by Decrypt produces the original plaintext.
// Corrupted ciphertext must return an error.
// **Validates: Requirements 3.5, 10.3**
func TestProperty14_ChaCha20Poly1305RoundTrip(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		key := rapid.SliceOfN(rapid.Byte(), KeySize, KeySize).Draw(t, "key")
		plaintext := rapid.SliceOfN(rapid.Byte(), 0, 1024).Draw(t, "plaintext")

		ciphertext, err := Encrypt(key, plaintext)
		if err != nil {
			t.Fatalf("Encrypt failed: %v", err)
		}

		decrypted, err := Decrypt(key, ciphertext)
		if err != nil {
			t.Fatalf("Decrypt failed: %v", err)
		}

		if len(plaintext) == 0 && len(decrypted) == 0 {
			return
		}
		if string(plaintext) != string(decrypted) {
			t.Fatalf("round-trip mismatch")
		}
	})
}

// TestProperty14_CorruptedCiphertext verifies corrupted ciphertext returns error.
func TestProperty14_CorruptedCiphertext(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		key := rapid.SliceOfN(rapid.Byte(), KeySize, KeySize).Draw(t, "key")
		plaintext := rapid.SliceOfN(rapid.Byte(), 1, 256).Draw(t, "plaintext")

		ciphertext, err := Encrypt(key, plaintext)
		if err != nil {
			t.Fatalf("Encrypt failed: %v", err)
		}

		// Corrupt a byte in the ciphertext (after nonce)
		idx := rapid.IntRange(NonceSize, len(ciphertext)-1).Draw(t, "corrupt_idx")
		corrupted := make([]byte, len(ciphertext))
		copy(corrupted, ciphertext)
		corrupted[idx] ^= 0xFF

		_, err = Decrypt(key, corrupted)
		if err == nil {
			t.Fatalf("expected error for corrupted ciphertext, got nil")
		}
	})
}
