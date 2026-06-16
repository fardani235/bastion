package vault

import (
	"bytes"
	"testing"
)

var testParams = DeriveParams{Time: 1, Memory: 64, Threads: 1}

func TestClampParam(t *testing.T) {
	const min, max, def uint32 = 16, 2048, 64
	cases := []struct {
		name     string
		v, want  uint32
	}{
		{"in range low edge", min, min},
		{"in range high edge", max, max},
		{"mid range", 100, 100},
		{"below min falls back to default", min - 1, def},
		{"above max falls back to default", max + 1, def},
		{"zero falls back to default", 0, def},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := ClampParam(c.v, min, max, def); got != c.want {
				t.Fatalf("ClampParam(%d, %d, %d, %d) = %d, want %d", c.v, min, max, def, got, c.want)
			}
		})
	}
}

func TestDerive_DeterministicForSameInputs(t *testing.T) {
	salt := []byte("0123456789abcdef") // 16 bytes
	k1 := Derive("hunter2", salt, testParams)
	k2 := Derive("hunter2", salt, testParams)

	if !bytes.Equal(k1, k2) {
		t.Fatalf("Derive must be deterministic: got %x and %x", k1, k2)
	}
	if len(k1) != 32 {
		t.Fatalf("expected 32-byte key, got %d", len(k1))
	}
}

func TestDerive_DifferentPasswordsDifferentKeys(t *testing.T) {
	salt := []byte("0123456789abcdef")
	k1 := Derive("password-a", salt, testParams)
	k2 := Derive("password-b", salt, testParams)

	if bytes.Equal(k1, k2) {
		t.Fatal("different passwords must produce different keys")
	}
}

func TestDerive_DifferentSaltsDifferentKeys(t *testing.T) {
	k1 := Derive("hunter2", []byte("0000000000000000"), testParams)
	k2 := Derive("hunter2", []byte("ffffffffffffffff"), testParams)

	if bytes.Equal(k1, k2) {
		t.Fatal("different salts must produce different keys")
	}
}

func TestEncryptDecrypt_RoundTrip(t *testing.T) {
	key := Derive("hunter2", []byte("0123456789abcdef"), testParams)
	plaintext := []byte("super-secret-ssh-password")

	ct, err := Encrypt(key, plaintext)
	if err != nil {
		t.Fatalf("Encrypt: %v", err)
	}
	if bytes.Equal(ct, plaintext) {
		t.Fatal("ciphertext must not equal plaintext")
	}

	pt, err := Decrypt(key, ct)
	if err != nil {
		t.Fatalf("Decrypt: %v", err)
	}
	if !bytes.Equal(pt, plaintext) {
		t.Fatalf("round-trip mismatch: got %q want %q", pt, plaintext)
	}
}

func TestEncrypt_DifferentNonceEachCall(t *testing.T) {
	key := Derive("hunter2", []byte("0123456789abcdef"), testParams)
	ct1, _ := Encrypt(key, []byte("same plaintext"))
	ct2, _ := Encrypt(key, []byte("same plaintext"))

	if bytes.Equal(ct1, ct2) {
		t.Fatal("encrypting the same plaintext twice must produce different ciphertexts (nonce must be fresh)")
	}
}

func TestDecrypt_WrongKeyFails(t *testing.T) {
	good := Derive("hunter2", []byte("0123456789abcdef"), testParams)
	bad := Derive("wrong", []byte("0123456789abcdef"), testParams)
	ct, _ := Encrypt(good, []byte("payload"))

	if _, err := Decrypt(bad, ct); err == nil {
		t.Fatal("Decrypt with wrong key must fail")
	}
}

func TestDecrypt_TamperedCiphertextFails(t *testing.T) {
	key := Derive("hunter2", []byte("0123456789abcdef"), testParams)
	ct, _ := Encrypt(key, []byte("payload"))

	// Flip one byte in the middle of the ciphertext (after the 12-byte nonce).
	ct[15] ^= 0x01

	if _, err := Decrypt(key, ct); err == nil {
		t.Fatal("Decrypt of tampered ciphertext must fail (GCM auth tag)")
	}
}

func TestDecrypt_TooShortFails(t *testing.T) {
	key := Derive("hunter2", []byte("0123456789abcdef"), testParams)
	if _, err := Decrypt(key, []byte{0x00, 0x01}); err == nil {
		t.Fatal("Decrypt of a too-short blob must fail")
	}
}

func TestNewVerifyBlob_RoundTrip(t *testing.T) {
	key := Derive("hunter2", []byte("0123456789abcdef"), testParams)

	blob, err := NewVerifyBlob(key)
	if err != nil {
		t.Fatalf("NewVerifyBlob: %v", err)
	}
	if err := CheckVerifyBlob(key, blob); err != nil {
		t.Fatalf("CheckVerifyBlob (correct key): %v", err)
	}
}

func TestCheckVerifyBlob_WrongKey(t *testing.T) {
	good := Derive("hunter2", []byte("0123456789abcdef"), testParams)
	bad := Derive("wrong", []byte("0123456789abcdef"), testParams)

	blob, _ := NewVerifyBlob(good)
	if err := CheckVerifyBlob(bad, blob); err == nil {
		t.Fatal("CheckVerifyBlob with wrong key must fail")
	}
}

func TestCheckVerifyBlob_DifferentPlaintextFails(t *testing.T) {
	// Encrypt some OTHER plaintext under the correct key — CheckVerifyBlob
	// must reject it because the decrypted plaintext won't match the fixed
	// verification string.
	key := Derive("hunter2", []byte("0123456789abcdef"), testParams)
	ct, _ := Encrypt(key, []byte("not-the-verify-string"))

	if err := CheckVerifyBlob(key, ct); err == nil {
		t.Fatal("CheckVerifyBlob must reject decryptable blobs with wrong plaintext")
	}
}

func TestEncrypt_WrongKeyLengthFails(t *testing.T) {
	// AES requires a 16/24/32-byte key. A short key must surface an error
	// rather than silently producing a weak or malformed blob.
	if _, err := Encrypt([]byte("too-short"), []byte("payload")); err == nil {
		t.Fatal("Encrypt with a non-AES key length must fail")
	}
}

func TestDecrypt_WrongKeyLengthFails(t *testing.T) {
	// A long-enough blob but an invalid key length must error at cipher
	// construction, not panic.
	blob := make([]byte, NonceLen+16)
	if _, err := Decrypt([]byte("too-short"), blob); err == nil {
		t.Fatal("Decrypt with a non-AES key length must fail")
	}
}

func TestNewSalt_LengthAndRandomness(t *testing.T) {
	s1, err := NewSalt()
	if err != nil {
		t.Fatalf("NewSalt: %v", err)
	}
	if len(s1) != SaltLen {
		t.Fatalf("expected %d-byte salt, got %d", SaltLen, len(s1))
	}

	s2, _ := NewSalt()
	if bytes.Equal(s1, s2) {
		t.Fatal("two NewSalt calls must produce different salts")
	}
}
