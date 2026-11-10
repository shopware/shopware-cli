package mkcert

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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
	assert.Equal(t, x509.ECDSAWithSHA256, ca.Cert.SignatureAlgorithm)
	assert.NotEmpty(t, ca.Cert.SubjectKeyId)
	assert.Positive(t, ca.Cert.SerialNumber.Sign())
	assert.NoError(t, ca.Cert.CheckSignatureFrom(ca.Cert))

	// A second load must reuse the CA, not create a new one.
	reloaded, created, err := LoadOrCreateCA()
	assert.NoError(t, err)
	assert.False(t, created)
	assert.Equal(t, ca.Cert.SerialNumber, reloaded.Cert.SerialNumber)
}

func TestMakeCert(t *testing.T) {
	for _, existingRSA := range []bool{false, true} {
		name := "new ECDSA CA"
		if existingRSA {
			name = "existing RSA CA"
		}
		t.Run(name, func(t *testing.T) {
			testMakeCert(t, existingRSA)
		})
	}
}

func testMakeCert(t *testing.T, existingRSA bool) {
	t.Helper()
	t.Setenv("CAROOT", t.TempDir())

	var originalCA []byte
	if existingRSA {
		// Reproduce mkcert's RSA CA and PKCS#8 storage, including an existing
		// key identifier that must be preserved when issuing new certificates.
		key, err := rsa.GenerateKey(rand.Reader, 3072)
		require.NoError(t, err)
		template := &x509.Certificate{
			Subject:               pkix.Name{CommonName: "existing mkcert CA"},
			SubjectKeyId:          []byte{1, 2, 3, 4},
			NotBefore:             time.Now().Add(-time.Hour),
			NotAfter:              time.Now().AddDate(10, 0, 0),
			IsCA:                  true,
			BasicConstraintsValid: true,
			KeyUsage:              x509.KeyUsageCertSign,
			MaxPathLenZero:        true,
		}
		der, err := x509.CreateCertificate(rand.Reader, template, template, key.Public(), key)
		require.NoError(t, err)
		originalCA = pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
		require.NoError(t, os.WriteFile(filepath.Join(CAROOT(), rootName), originalCA, 0o644))
		keyDER, err := x509.MarshalPKCS8PrivateKey(key)
		require.NoError(t, err)
		require.NoError(t, os.WriteFile(filepath.Join(CAROOT(), rootKeyName), pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: keyDER}), 0o400))
	}

	ca, created, err := LoadOrCreateCA()
	require.NoError(t, err)
	assert.Equal(t, !existingRSA, created)
	if existingRSA {
		storedCA, err := os.ReadFile(ca.CertPath())
		require.NoError(t, err)
		assert.Equal(t, originalCA, storedCA)
	}

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
	assert.Equal(t, x509.KeyUsageDigitalSignature, cert.KeyUsage)
	assert.Equal(t, ca.Cert.SubjectKeyId, cert.AuthorityKeyId)
	assert.Positive(t, cert.SerialNumber.Sign())

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
	key, err := x509.ParsePKCS8PrivateKey(keyBlock.Bytes)
	require.NoError(t, err)
	ecKey, ok := key.(*ecdsa.PrivateKey)
	require.True(t, ok)
	assert.Equal(t, elliptic.P256(), ecKey.Curve)

	// Exercise actual TLS handshakes with both supported TLS versions.
	pair, err := tls.LoadX509KeyPair(certFile, keyFile)
	require.NoError(t, err)
	server := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	server.TLS = &tls.Config{Certificates: []tls.Certificate{pair}, MinVersion: tls.VersionTLS12}
	server.StartTLS()
	t.Cleanup(server.Close)
	for _, version := range []uint16{tls.VersionTLS12, tls.VersionTLS13} {
		transport := &http.Transport{TLSClientConfig: &tls.Config{RootCAs: roots, MinVersion: version, MaxVersion: version}}
		t.Cleanup(transport.CloseIdleConnections)
		client := &http.Client{Transport: transport, Timeout: 5 * time.Second}
		request, err := http.NewRequestWithContext(t.Context(), http.MethodGet, server.URL, nil)
		require.NoError(t, err)
		response, err := client.Do(request)
		require.NoError(t, err)
		assert.Equal(t, http.StatusNoContent, response.StatusCode)
		require.NoError(t, response.Body.Close())
	}
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
