// Copyright 2018 The mkcert Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package mkcert is a library adaptation of https://github.com/FiloSottile/mkcert
// (which is intentionally not importable as a Go module). It creates and loads
// the local development CA in mkcert's CAROOT using mkcert's exact file format
// and naming, so the CA is fully shared with the mkcert tool: a CA created by
// mkcert is reused here, a CA created here works with "mkcert -install" and
// certificates issued by either are interchangeable.
//
// The code is adapted from mkcert's cert.go and main.go: it returns errors
// instead of exiting, and the PKCS#12, CSR and client-certificate paths are
// removed.
package mkcert

import (
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha1" //nolint:gosec // mkcert uses SHA-1 for the Subject Key Identifier, which is not a security-sensitive use.
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/asn1"
	"encoding/pem"
	"fmt"
	"math/big"
	"net"
	"os"
	"os/user"
	"path/filepath"
	"runtime"
	"sync"
	"time"
)

const (
	rootName    = "rootCA.pem"
	rootKeyName = "rootCA-key.pem"
)

// CAROOT returns the mkcert CA directory, honoring the $CAROOT environment
// variable exactly like the mkcert tool.
func CAROOT() string {
	if env := os.Getenv("CAROOT"); env != "" {
		return env
	}

	var dir string
	switch {
	case runtime.GOOS == "windows":
		dir = os.Getenv("LocalAppData")
	case os.Getenv("XDG_DATA_HOME") != "":
		dir = os.Getenv("XDG_DATA_HOME")
	case runtime.GOOS == "darwin":
		dir = os.Getenv("HOME")
		if dir == "" {
			return ""
		}
		dir = filepath.Join(dir, "Library", "Application Support")
	default: // Unix
		dir = os.Getenv("HOME")
		if dir == "" {
			return ""
		}
		dir = filepath.Join(dir, ".local", "share")
	}
	return filepath.Join(dir, "mkcert")
}

var (
	userAndHostnameOnce sync.Once
	userAndHostname     string
)

func getUserAndHostname() string {
	userAndHostnameOnce.Do(func() {
		u, err := user.Current()
		if err == nil {
			userAndHostname = u.Username + "@"
		}
		if h, err := os.Hostname(); err == nil {
			userAndHostname += h
		}
		if err == nil && u.Name != "" && u.Name != u.Username {
			userAndHostname += " (" + u.Name + ")"
		}
	})

	return userAndHostname
}

// CA is the local development certificate authority stored in CAROOT.
type CA struct {
	Cert *x509.Certificate

	root string
	key  crypto.PrivateKey
}

// CertPath returns the path of the CA certificate (rootCA.pem).
func (c *CA) CertPath() string {
	return filepath.Join(c.root, rootName)
}

// HasKey reports whether the CA private key is available. mkcert supports a
// keyless mode where only the certificate is distributed for installing.
func (c *CA) HasKey() bool {
	return c.key != nil
}

// LoadOrCreateCA loads the CA from CAROOT, creating a new one first when none
// exists yet (adapted from mkcert's loadCA/newCA). It returns the CA and
// whether it was newly created.
func LoadOrCreateCA() (*CA, bool, error) {
	root := CAROOT()
	if root == "" {
		return nil, false, fmt.Errorf("failed to find the default CA location, set the CAROOT environment variable")
	}

	if err := os.MkdirAll(root, 0o755); err != nil {
		return nil, false, fmt.Errorf("failed to create the CAROOT: %w", err)
	}

	ca := &CA{root: root}

	created := false
	if !pathExists(filepath.Join(root, rootName)) {
		if err := ca.newCA(); err != nil {
			return nil, false, err
		}
		created = true
	}

	certPEMBlock, err := os.ReadFile(filepath.Join(root, rootName))
	if err != nil {
		return nil, false, fmt.Errorf("failed to read the CA certificate: %w", err)
	}
	certDERBlock, _ := pem.Decode(certPEMBlock)
	if certDERBlock == nil || certDERBlock.Type != "CERTIFICATE" {
		return nil, false, fmt.Errorf("failed to read the CA certificate: unexpected content")
	}
	ca.Cert, err = x509.ParseCertificate(certDERBlock.Bytes)
	if err != nil {
		return nil, false, fmt.Errorf("failed to parse the CA certificate: %w", err)
	}

	if !pathExists(filepath.Join(root, rootKeyName)) {
		return ca, created, nil // keyless mode, where only -install works
	}

	keyPEMBlock, err := os.ReadFile(filepath.Join(root, rootKeyName))
	if err != nil {
		return nil, false, fmt.Errorf("failed to read the CA key: %w", err)
	}
	keyDERBlock, _ := pem.Decode(keyPEMBlock)
	if keyDERBlock == nil || keyDERBlock.Type != "PRIVATE KEY" {
		return nil, false, fmt.Errorf("failed to read the CA key: unexpected content")
	}
	ca.key, err = x509.ParsePKCS8PrivateKey(keyDERBlock.Bytes)
	if err != nil {
		return nil, false, fmt.Errorf("failed to parse the CA key: %w", err)
	}

	return ca, created, nil
}

func (c *CA) newCA() error {
	priv, err := generateKey(true)
	if err != nil {
		return fmt.Errorf("failed to generate the CA key: %w", err)
	}
	pub := priv.(crypto.Signer).Public()

	spkiASN1, err := x509.MarshalPKIXPublicKey(pub)
	if err != nil {
		return fmt.Errorf("failed to encode public key: %w", err)
	}

	var spki struct {
		Algorithm        pkix.AlgorithmIdentifier
		SubjectPublicKey asn1.BitString
	}
	if _, err := asn1.Unmarshal(spkiASN1, &spki); err != nil {
		return fmt.Errorf("failed to decode public key: %w", err)
	}

	skid := sha1.Sum(spki.SubjectPublicKey.Bytes) //nolint:gosec // see import comment

	tpl := &x509.Certificate{
		SerialNumber: randomSerialNumber(),
		Subject: pkix.Name{
			Organization:       []string{"mkcert development CA"},
			OrganizationalUnit: []string{getUserAndHostname()},

			// The CommonName is required by iOS to show the certificate in the
			// "Certificate Trust Settings" menu.
			// https://github.com/FiloSottile/mkcert/issues/47
			CommonName: "mkcert " + getUserAndHostname(),
		},
		SubjectKeyId: skid[:],

		NotAfter:  time.Now().AddDate(10, 0, 0),
		NotBefore: time.Now(),

		KeyUsage: x509.KeyUsageCertSign,

		BasicConstraintsValid: true,
		IsCA:                  true,
		MaxPathLenZero:        true,
	}

	cert, err := x509.CreateCertificate(rand.Reader, tpl, tpl, pub, priv)
	if err != nil {
		return fmt.Errorf("failed to generate CA certificate: %w", err)
	}

	privDER, err := x509.MarshalPKCS8PrivateKey(priv)
	if err != nil {
		return fmt.Errorf("failed to encode CA key: %w", err)
	}
	if err := os.WriteFile(filepath.Join(c.root, rootKeyName), pem.EncodeToMemory(
		&pem.Block{Type: "PRIVATE KEY", Bytes: privDER}), 0o400); err != nil {
		return fmt.Errorf("failed to save CA key: %w", err)
	}

	if err := os.WriteFile(filepath.Join(c.root, rootName), pem.EncodeToMemory(
		&pem.Block{Type: "CERTIFICATE", Bytes: cert}), 0o644); err != nil {
		return fmt.Errorf("failed to save CA certificate: %w", err)
	}

	return nil
}

// MakeCert issues a certificate for the given hosts signed by the CA and
// writes it to certFile/keyFile (adapted from mkcert's makeCert).
func (c *CA) MakeCert(hosts []string, certFile, keyFile string) error {
	if c.key == nil {
		return fmt.Errorf("can't create new certificates because the CA key (%s) is missing", rootKeyName)
	}

	priv, err := generateKey(false)
	if err != nil {
		return fmt.Errorf("failed to generate certificate key: %w", err)
	}
	pub := priv.(crypto.Signer).Public()

	// Certificates last for 2 years and 3 months, which is always less than
	// 825 days, the limit that macOS/iOS apply to all certificates,
	// including custom roots. See https://support.apple.com/en-us/HT210176.
	expiration := time.Now().AddDate(2, 3, 0)

	tpl := &x509.Certificate{
		SerialNumber: randomSerialNumber(),
		Subject: pkix.Name{
			Organization:       []string{"mkcert development certificate"},
			OrganizationalUnit: []string{getUserAndHostname()},
		},

		NotBefore: time.Now(), NotAfter: expiration,

		KeyUsage: x509.KeyUsageKeyEncipherment | x509.KeyUsageDigitalSignature,
	}

	for _, h := range hosts {
		if ip := net.ParseIP(h); ip != nil {
			tpl.IPAddresses = append(tpl.IPAddresses, ip)
		} else {
			tpl.DNSNames = append(tpl.DNSNames, h)
		}
	}

	if len(tpl.IPAddresses) > 0 || len(tpl.DNSNames) > 0 {
		tpl.ExtKeyUsage = append(tpl.ExtKeyUsage, x509.ExtKeyUsageServerAuth)
	}

	cert, err := x509.CreateCertificate(rand.Reader, tpl, c.Cert, pub, c.key)
	if err != nil {
		return fmt.Errorf("failed to generate certificate: %w", err)
	}

	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: cert})
	privDER, err := x509.MarshalPKCS8PrivateKey(priv)
	if err != nil {
		return fmt.Errorf("failed to encode certificate key: %w", err)
	}
	privPEM := pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: privDER})

	if err := os.WriteFile(certFile, certPEM, 0o644); err != nil {
		return fmt.Errorf("failed to save certificate: %w", err)
	}
	if err := os.WriteFile(keyFile, privPEM, 0o600); err != nil {
		return fmt.Errorf("failed to save certificate key: %w", err)
	}

	return nil
}

func generateKey(rootCA bool) (crypto.PrivateKey, error) {
	if rootCA {
		return rsa.GenerateKey(rand.Reader, 3072)
	}
	return rsa.GenerateKey(rand.Reader, 2048)
}

func randomSerialNumber() *big.Int {
	serialNumberLimit := new(big.Int).Lsh(big.NewInt(1), 128)
	serialNumber, err := rand.Int(rand.Reader, serialNumberLimit)
	if err != nil {
		// crypto/rand failing is not recoverable in any meaningful way.
		panic(fmt.Sprintf("failed to generate serial number: %v", err))
	}
	return serialNumber
}

func pathExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}
