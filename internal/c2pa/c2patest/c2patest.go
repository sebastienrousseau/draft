// SPDX-FileCopyrightText: 2026 Sebastien Rousseau
// SPDX-License-Identifier: MIT OR Apache-2.0

// Package c2patest generates a throwaway C2PA signing certificate chain for
// tests. It exists so the several test files that exercise signing do not each
// re-implement certificate generation, and so no signing key is ever committed
// to the repository — the material lives only for the duration of a test.
package c2patest

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
	"math/big"
	"os"
	"path/filepath"
	"time"
)

// Chain writes a P-256 root CA and a leaf signing certificate signed by it into
// dir, and returns the paths to the leaf+root chain PEM and the leaf key PEM.
// The leaf carries the digitalSignature key usage and emailProtection extended
// key usage a C2PA signer requires.
func Chain(dir string) (certPath, keyPath string, err error) {
	rootKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return "", "", err
	}
	now := time.Now()
	rootTmpl := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: "draft dev root", Organization: []string{"draft"}},
		NotBefore:             now.Add(-time.Hour),
		NotAfter:              now.Add(24 * time.Hour),
		IsCA:                  true,
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageCRLSign,
		BasicConstraintsValid: true,
	}
	rootDER, err := x509.CreateCertificate(rand.Reader, rootTmpl, rootTmpl, &rootKey.PublicKey, rootKey)
	if err != nil {
		return "", "", err
	}
	rootCert, err := x509.ParseCertificate(rootDER)
	if err != nil {
		return "", "", err
	}

	leafKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return "", "", err
	}
	leafTmpl := &x509.Certificate{
		SerialNumber:          big.NewInt(2),
		Subject:               pkix.Name{CommonName: "draft dev signer", Organization: []string{"draft"}},
		NotBefore:             now.Add(-time.Hour),
		NotAfter:              now.Add(24 * time.Hour),
		KeyUsage:              x509.KeyUsageDigitalSignature,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageEmailProtection},
		BasicConstraintsValid: true,
	}
	leafDER, err := x509.CreateCertificate(rand.Reader, leafTmpl, rootCert, &leafKey.PublicKey, rootKey)
	if err != nil {
		return "", "", err
	}

	certPath = filepath.Join(dir, "chain.pem")
	chain := append(
		pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: leafDER}),
		pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: rootDER})...,
	)
	if err := os.WriteFile(certPath, chain, 0o644); err != nil {
		return "", "", err
	}
	keyDER, err := x509.MarshalECPrivateKey(leafKey)
	if err != nil {
		return "", "", err
	}
	keyPath = filepath.Join(dir, "key.pem")
	if err := os.WriteFile(keyPath, pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER}), 0o600); err != nil {
		return "", "", err
	}
	return certPath, keyPath, nil
}

// MustChain is Chain with the error turned into a panic, for tests that treat a
// setup failure as fatal.
func MustChain(dir string) (certPath, keyPath string) {
	c, k, err := Chain(dir)
	if err != nil {
		panic(fmt.Sprintf("c2patest: generating dev chain: %v", err))
	}
	return c, k
}
