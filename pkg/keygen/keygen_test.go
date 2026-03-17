package keygen

import (
	"strings"
	"testing"
)

func TestGenerate(t *testing.T) {
	k, err := Generate()
	if err != nil {
		t.Fatalf("Generate() error: %v", err)
	}

	if len(k.PublicKey) == 0 {
		t.Error("PublicKey is empty")
	}
	if len(k.PrivateKey) == 0 {
		t.Error("PrivateKey is empty")
	}
	if !strings.HasPrefix(k.AuthorizedKey, "ssh-ed25519 ") {
		t.Errorf("AuthorizedKey has wrong prefix: %s", k.AuthorizedKey)
	}
	if k.Fingerprint == "" {
		t.Error("Fingerprint is empty")
	}
	if !strings.Contains(string(k.EncodedKey), "OPENSSH PRIVATE KEY") {
		t.Error("EncodedKey missing PEM header")
	}
}

func TestGenerateUniqueness(t *testing.T) {
	k1, err := Generate()
	if err != nil {
		t.Fatalf("Generate() error: %v", err)
	}
	k2, err := Generate()
	if err != nil {
		t.Fatalf("Generate() error: %v", err)
	}
	if k1.Fingerprint == k2.Fingerprint {
		t.Error("two generated keys have the same fingerprint")
	}
}
