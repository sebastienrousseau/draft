// SPDX-FileCopyrightText: 2026 Sebastien Rousseau
// SPDX-License-Identifier: MIT OR Apache-2.0

package c2patest

import (
	"crypto/x509"
	"encoding/pem"
	"os"
	"testing"
)

func TestChainProducesAParsableLeafAndKey(t *testing.T) {
	certPath, keyPath, err := Chain(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	certPEM, err := os.ReadFile(certPath)
	if err != nil {
		t.Fatal(err)
	}
	// The chain is leaf then root: two certificate blocks.
	blocks := 0
	rest := certPEM
	var leaf *x509.Certificate
	for {
		var b *pem.Block
		b, rest = pem.Decode(rest)
		if b == nil {
			break
		}
		c, err := x509.ParseCertificate(b.Bytes)
		if err != nil {
			t.Fatalf("certificate %d does not parse: %v", blocks, err)
		}
		if blocks == 0 {
			leaf = c
		}
		blocks++
	}
	if blocks != 2 {
		t.Fatalf("chain has %d certificates, want 2 (leaf + root)", blocks)
	}
	if leaf.KeyUsage&x509.KeyUsageDigitalSignature == 0 {
		t.Error("leaf lacks the digitalSignature key usage a signer needs")
	}

	keyPEM, err := os.ReadFile(keyPath)
	if err != nil {
		t.Fatal(err)
	}
	kb, _ := pem.Decode(keyPEM)
	if kb == nil {
		t.Fatal("key is not PEM")
	}
	if _, err := x509.ParseECPrivateKey(kb.Bytes); err != nil {
		t.Errorf("leaf key does not parse: %v", err)
	}
}

func TestMustChain(t *testing.T) {
	cert, key := MustChain(t.TempDir())
	if cert == "" || key == "" {
		t.Error("MustChain returned empty paths")
	}
}
