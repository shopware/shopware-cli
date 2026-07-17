package proxy

import (
	"crypto/x509"
	"encoding/pem"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestEnsureCertificateCreatesCAAndCert(t *testing.T) {
	t.Setenv("CAROOT", t.TempDir())
	dir := t.TempDir()

	info, err := EnsureCertificate(dir, CertHosts(DefaultDomain, nil))
	assert.NoError(t, err)
	assert.True(t, info.CACreated)
	assert.True(t, info.Changed)
	assert.FileExists(t, info.CAPath)
	assert.FileExists(t, ServerCertPath(dir))
	assert.FileExists(t, ServerKeyPath(dir))

	cert := readTestCertificate(t, ServerCertPath(dir))
	assert.ElementsMatch(t, []string{DefaultDomain, "*." + DefaultDomain}, cert.DNSNames)

	caCert := readTestCertificate(t, info.CAPath)
	roots := x509.NewCertPool()
	roots.AddCert(caCert)
	_, err = cert.Verify(x509.VerifyOptions{Roots: roots, DNSName: "my-shop." + DefaultDomain})
	assert.NoError(t, err)
}

func TestEnsureCertificateIsIdempotent(t *testing.T) {
	t.Setenv("CAROOT", t.TempDir())
	dir := t.TempDir()

	_, err := EnsureCertificate(dir, CertHosts(DefaultDomain, nil))
	assert.NoError(t, err)

	info, err := EnsureCertificate(dir, CertHosts(DefaultDomain, nil))
	assert.NoError(t, err)
	assert.False(t, info.CACreated)
	assert.False(t, info.Changed)
}

func TestEnsureCertificateRegeneratesForNewHost(t *testing.T) {
	t.Setenv("CAROOT", t.TempDir())
	dir := t.TempDir()

	_, err := EnsureCertificate(dir, CertHosts(DefaultDomain, nil))
	assert.NoError(t, err)

	info, err := EnsureCertificate(dir, CertHosts(DefaultDomain, []string{"shop.example.test"}))
	assert.NoError(t, err)
	assert.True(t, info.Changed)

	cert := readTestCertificate(t, ServerCertPath(dir))
	assert.Contains(t, cert.DNSNames, "shop.example.test")
}

func TestEnsureCertificateRegeneratesWhenCAChanges(t *testing.T) {
	firstRoot := t.TempDir()
	t.Setenv("CAROOT", firstRoot)
	dir := t.TempDir()

	_, err := EnsureCertificate(dir, CertHosts(DefaultDomain, nil))
	assert.NoError(t, err)

	// Switch to a different CA: the existing certificate no longer chains to
	// it and must be re-issued.
	t.Setenv("CAROOT", t.TempDir())

	info, err := EnsureCertificate(dir, CertHosts(DefaultDomain, nil))
	assert.NoError(t, err)
	assert.True(t, info.Changed)
	assert.True(t, certIssuedBy(ServerCertPath(dir), readTestCertificate(t, info.CAPath)))
}

func TestCertHosts(t *testing.T) {
	assert.ElementsMatch(t,
		[]string{"127.0.0.1.sslip.io", "*.127.0.0.1.sslip.io"},
		CertHosts("127.0.0.1.sslip.io", []string{"my-shop.127.0.0.1.sslip.io"}),
	)

	assert.ElementsMatch(t,
		[]string{"127.0.0.1.sslip.io", "*.127.0.0.1.sslip.io", "shop.example.test", "deep.sub.127.0.0.1.sslip.io"},
		CertHosts("127.0.0.1.sslip.io", []string{"shop.example.test", "deep.sub.127.0.0.1.sslip.io", "shop.example.test"}),
	)
}

func readTestCertificate(t *testing.T, path string) *x509.Certificate {
	t.Helper()

	content, err := os.ReadFile(path)
	assert.NoError(t, err)

	block, _ := pem.Decode(content)
	assert.NotNil(t, block)

	cert, err := x509.ParseCertificate(block.Bytes)
	assert.NoError(t, err)

	return cert
}
