// Package keygen produces ED25519 SSH keypairs along with their
// authorized_keys representation, OpenSSH-style PEM-encoded private key,
// and SHA256 fingerprint.
package keygen

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/pem"
	"strings"

	"golang.org/x/crypto/ssh"
)

// Result holds a generated ED25519 keypair in multiple formats.
type Result struct {
	PublicKey     ed25519.PublicKey
	PrivateKey    ed25519.PrivateKey
	AuthorizedKey string
	Fingerprint   string
	EncodedKey    []byte // PEM-encoded private key
}

// Generate creates a new random ED25519 keypair.
func Generate() (Result, error) {
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return Result{}, err
	}

	sshPub, err := ssh.NewPublicKey(pub)
	if err != nil {
		return Result{}, err
	}

	pemBlock, err := ssh.MarshalPrivateKey(priv, "")
	if err != nil {
		return Result{}, err
	}

	h := sha256.New()
	h.Write(sshPub.Marshal())

	return Result{
		PublicKey:     pub,
		PrivateKey:    priv,
		AuthorizedKey: strings.TrimSpace(string(ssh.MarshalAuthorizedKey(sshPub))),
		Fingerprint:   base64.StdEncoding.EncodeToString(h.Sum(nil)),
		EncodedKey:    pem.EncodeToMemory(pemBlock),
	}, nil
}
