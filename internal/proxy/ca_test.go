package proxy

import (
	"crypto/x509"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestEnsureCACreatesOnce(t *testing.T) {
	dir := t.TempDir()

	created, err := EnsureCA(dir)
	assert.NoError(t, err)
	assert.True(t, created)

	first, err := os.ReadFile(CACertPath(dir))
	assert.NoError(t, err)

	created, err = EnsureCA(dir)
	assert.NoError(t, err)
	assert.False(t, created)

	second, err := os.ReadFile(CACertPath(dir))
	assert.NoError(t, err)
	assert.Equal(t, first, second)
}

func TestEnsureServerCertSignedByCA(t *testing.T) {
	dir := t.TempDir()

	_, err := EnsureCA(dir)
	assert.NoError(t, err)

	created, err := EnsureServerCert(dir, CertHosts(DefaultDomain, nil))
	assert.NoError(t, err)
	assert.True(t, created)

	caDer, err := readPem(CACertPath(dir), "CERTIFICATE")
	assert.NoError(t, err)
	caCert, err := x509.ParseCertificate(caDer)
	assert.NoError(t, err)

	serverDer, err := readPem(ServerCertPath(dir), "CERTIFICATE")
	assert.NoError(t, err)
	serverCert, err := x509.ParseCertificate(serverDer)
	assert.NoError(t, err)

	roots := x509.NewCertPool()
	roots.AddCert(caCert)

	_, err = serverCert.Verify(x509.VerifyOptions{Roots: roots, DNSName: "my-shop." + DefaultDomain})
	assert.NoError(t, err)

	assert.ElementsMatch(t, []string{DefaultDomain, "*." + DefaultDomain}, serverCert.DNSNames)
}

func TestEnsureServerCertIsIdempotent(t *testing.T) {
	dir := t.TempDir()

	_, err := EnsureCA(dir)
	assert.NoError(t, err)

	_, err = EnsureServerCert(dir, CertHosts(DefaultDomain, nil))
	assert.NoError(t, err)

	created, err := EnsureServerCert(dir, CertHosts(DefaultDomain, nil))
	assert.NoError(t, err)
	assert.False(t, created)
}

func TestEnsureServerCertRegeneratesForNewHost(t *testing.T) {
	dir := t.TempDir()

	_, err := EnsureCA(dir)
	assert.NoError(t, err)

	_, err = EnsureServerCert(dir, CertHosts(DefaultDomain, nil))
	assert.NoError(t, err)

	created, err := EnsureServerCert(dir, CertHosts(DefaultDomain, []string{"shop.example.test"}))
	assert.NoError(t, err)
	assert.True(t, created)

	serverDer, err := readPem(ServerCertPath(dir), "CERTIFICATE")
	assert.NoError(t, err)
	serverCert, err := x509.ParseCertificate(serverDer)
	assert.NoError(t, err)

	assert.Contains(t, serverCert.DNSNames, "shop.example.test")
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

func TestSanitizeName(t *testing.T) {
	assert.Equal(t, "my-shop", SanitizeName("My_Shop"))
	assert.Equal(t, "shop6", SanitizeName("shop6"))
	assert.Equal(t, "my-shop", SanitizeName("-my.shop-"))
	assert.Equal(t, "shopware", SanitizeName("...."))
}
