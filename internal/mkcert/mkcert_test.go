package mkcert

import (
	"crypto/x509"
	"encoding/pem"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestCAROOTHonorsEnv(t *testing.T) {
	t.Setenv("CAROOT", "/some/where")
	assert.Equal(t, "/some/where", CAROOT())
}

func TestLoadOrCreateCA(t *testing.T) {
	t.Setenv("CAROOT", t.TempDir())

	ca, created, err := LoadOrCreateCA()
	assert.NoError(t, err)
	assert.True(t, created)
	assert.True(t, ca.HasKey())
	assert.True(t, ca.Cert.IsCA)
	assert.Contains(t, ca.Cert.Subject.Organization, "mkcert development CA")
	assert.FileExists(t, ca.CertPath())
	assert.FileExists(t, filepath.Join(CAROOT(), "rootCA-key.pem"))

	// A second load must reuse the CA, not create a new one.
	reloaded, created, err := LoadOrCreateCA()
	assert.NoError(t, err)
	assert.False(t, created)
	assert.Equal(t, ca.Cert.SerialNumber, reloaded.Cert.SerialNumber)
}

func TestMakeCert(t *testing.T) {
	t.Setenv("CAROOT", t.TempDir())

	ca, _, err := LoadOrCreateCA()
	assert.NoError(t, err)

	dir := t.TempDir()
	certFile := filepath.Join(dir, "cert.pem")
	keyFile := filepath.Join(dir, "key.pem")

	assert.NoError(t, ca.MakeCert([]string{"127.0.0.1.sslip.io", "*.127.0.0.1.sslip.io", "127.0.0.1"}, certFile, keyFile))

	certPEM, err := os.ReadFile(certFile)
	assert.NoError(t, err)
	block, _ := pem.Decode(certPEM)
	assert.NotNil(t, block)

	cert, err := x509.ParseCertificate(block.Bytes)
	assert.NoError(t, err)
	assert.ElementsMatch(t, []string{"127.0.0.1.sslip.io", "*.127.0.0.1.sslip.io"}, cert.DNSNames)
	assert.Len(t, cert.IPAddresses, 1)

	roots := x509.NewCertPool()
	roots.AddCert(ca.Cert)

	_, err = cert.Verify(x509.VerifyOptions{Roots: roots, DNSName: "my-shop.127.0.0.1.sslip.io"})
	assert.NoError(t, err)

	// The key must be PKCS#8, the format mkcert writes and reads.
	keyPEM, err := os.ReadFile(keyFile)
	assert.NoError(t, err)
	keyBlock, _ := pem.Decode(keyPEM)
	assert.NotNil(t, keyBlock)
	assert.Equal(t, "PRIVATE KEY", keyBlock.Type)
	_, err = x509.ParsePKCS8PrivateKey(keyBlock.Bytes)
	assert.NoError(t, err)
}

func TestKeylessCA(t *testing.T) {
	t.Setenv("CAROOT", t.TempDir())

	ca, _, err := LoadOrCreateCA()
	assert.NoError(t, err)

	// Remove the key to simulate mkcert's keyless distribution mode.
	assert.NoError(t, os.Remove(filepath.Join(CAROOT(), "rootCA-key.pem")))

	keyless, created, err := LoadOrCreateCA()
	assert.NoError(t, err)
	assert.False(t, created)
	assert.False(t, keyless.HasKey())
	assert.Equal(t, ca.Cert.SerialNumber, keyless.Cert.SerialNumber)

	err = keyless.MakeCert([]string{"example.test"}, filepath.Join(t.TempDir(), "c.pem"), filepath.Join(t.TempDir(), "k.pem"))
	assert.Error(t, err)
}
