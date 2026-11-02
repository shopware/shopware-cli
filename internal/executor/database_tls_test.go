package executor

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"net"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/shopware/shopware-cli/internal/testhelper"
)

type testCert struct {
	certPEM []byte
	keyPEM  []byte
}

// newTestCert issues a certificate for dnsName, signed by parent (or
// self-signed when parent is nil).
func newTestCert(t *testing.T, dnsName string, isCA bool, parent *x509.Certificate, parentKey *ecdsa.PrivateKey) (testCert, *x509.Certificate, *ecdsa.PrivateKey) {
	t.Helper()

	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)

	tmpl := &x509.Certificate{
		SerialNumber:          big.NewInt(time.Now().UnixNano()),
		Subject:               pkix.Name{CommonName: dnsName},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(time.Hour),
		BasicConstraintsValid: true,
		IsCA:                  isCA,
		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageCertSign,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth, x509.ExtKeyUsageClientAuth},
	}
	if !isCA {
		tmpl.DNSNames = []string{dnsName}
	}

	signer, signerKey := tmpl, key
	if parent != nil {
		signer, signerKey = parent, parentKey
	}

	der, err := x509.CreateCertificate(rand.Reader, tmpl, signer, &key.PublicKey, signerKey)
	require.NoError(t, err)

	cert, err := x509.ParseCertificate(der)
	require.NoError(t, err)

	keyDER, err := x509.MarshalECPrivateKey(key)
	require.NoError(t, err)

	return testCert{
		certPEM: pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}),
		keyPEM:  pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER}),
	}, cert, key
}

// handshake runs a TLS handshake between clientCfg and a server presenting
// serverCert.
func handshake(t *testing.T, clientCfg *tls.Config, serverCert testCert) error {
	t.Helper()

	pair, err := tls.X509KeyPair(serverCert.certPEM, serverCert.keyPEM)
	require.NoError(t, err)

	// A real socket buffers the alert of a failed handshake; net.Pipe would
	// deadlock with both sides writing.
	listener, err := tls.Listen("tcp", "127.0.0.1:0", &tls.Config{Certificates: []tls.Certificate{pair}, MinVersion: tls.VersionTLS12})
	require.NoError(t, err)
	defer func() { _ = listener.Close() }()

	go func() {
		conn, err := listener.Accept()
		if err != nil {
			return
		}
		_ = conn.(*tls.Conn).Handshake()
		_ = conn.Close()
	}()

	conn, err := net.DialTimeout("tcp", listener.Addr().String(), 5*time.Second)
	require.NoError(t, err)
	defer func() { _ = conn.Close() }()
	require.NoError(t, conn.SetDeadline(time.Now().Add(5*time.Second)))

	return tls.Client(conn, clientCfg).Handshake()
}

func TestParseTLSMode(t *testing.T) {
	for input, expected := range map[string]TLSMode{
		"disabled":        TLSModeDisabled,
		"PREFERRED":       TLSModePreferred,
		"required":        TLSModeRequired,
		"VERIFY_CA":       TLSModeVerifyCA,
		"verify-identity": TLSModeVerifyIdentity,
	} {
		mode, err := ParseTLSMode(input)
		require.NoError(t, err, input)
		assert.Equal(t, expected, mode, input)
	}

	_, err := ParseTLSMode("on")
	assert.Error(t, err)
}

func TestApplyDatabaseURLTLSParam(t *testing.T) {
	for param, expected := range map[string]TLSMode{
		"true":        TLSModeVerifyIdentity,
		"false":       TLSModeDisabled,
		"skip-verify": TLSModeRequired,
		"preferred":   TLSModePreferred,
		"verify-ca":   TLSModeVerifyCA,
	} {
		conn := defaultDatabaseConnection()
		require.NoError(t, applyDatabaseURL(conn, "mysql://app:pw@db.example.com/shop?tls="+param), param)
		assert.Equal(t, expected, conn.TLS.Mode, param)
	}

	conn := defaultDatabaseConnection()
	require.NoError(t, applyDatabaseURL(conn, "mysql://app:pw@db.example.com/shop?serverVersion=8.0"))
	assert.False(t, conn.TLS.Enabled())

	assert.Error(t, applyDatabaseURL(defaultDatabaseConnection(), "mysql://app:pw@db.example.com/shop?tls=custom"))
}

func TestDatabaseConnectionFromEnvSSLVariables(t *testing.T) {
	t.Setenv("DATABASE_URL", "")
	for _, key := range databaseTLSEnvKeys {
		t.Setenv(key, "")
	}

	ca, _, _ := newTestCert(t, "Test CA", true, nil, nil)

	projectRoot := t.TempDir()
	testhelper.WriteFile(t, filepath.Join(projectRoot, "config", "ca.pem"), string(ca.certPEM))
	testhelper.WriteFile(t, filepath.Join(projectRoot, ".env"), "DATABASE_URL=mysql://app:pw@db.example.com:3306/shop\nDATABASE_SSL_CA=config/ca.pem\n")

	conn, err := databaseConnectionFromEnv(projectRoot, nil)
	require.NoError(t, err)

	assert.Equal(t, TLSModeVerifyIdentity, conn.TLS.Mode)
	assert.Equal(t, ca.certPEM, conn.TLS.CA, "relative paths resolve from the project root")

	cfg, err := conn.MySQLConfig()
	require.NoError(t, err)
	require.NotNil(t, cfg.TLS)
	assert.Equal(t, "db.example.com", cfg.TLS.ServerName)
	assert.False(t, cfg.TLS.InsecureSkipVerify)
	assert.False(t, cfg.AllowFallbackToPlaintext)
}

func TestDatabaseConnectionFromEnvSSLDontVerify(t *testing.T) {
	t.Setenv("DATABASE_URL", "mysql://app:pw@db.example.com:3306/shop")
	t.Setenv("DATABASE_SSL_CA", "true")
	t.Setenv("DATABASE_SSL_CERT", "")
	t.Setenv("DATABASE_SSL_KEY", "")
	t.Setenv("DATABASE_SSL_DONT_VERIFY_SERVER_CERT", "1")

	conn, err := databaseConnectionFromEnv(t.TempDir(), nil)
	require.NoError(t, err)

	assert.Equal(t, TLSModeRequired, conn.TLS.Mode)
	assert.Empty(t, conn.TLS.CA, "DATABASE_SSL_CA=true forces TLS without a CA file")

	cfg, err := conn.MySQLConfig()
	require.NoError(t, err)
	require.NotNil(t, cfg.TLS)
	assert.True(t, cfg.TLS.InsecureSkipVerify)
}

func TestDatabaseConnectionFromEnvSSLMissingFile(t *testing.T) {
	t.Setenv("DATABASE_URL", "mysql://app:pw@db.example.com:3306/shop")
	t.Setenv("DATABASE_SSL_CA", "/does/not/exist.pem")

	_, err := databaseConnectionFromEnv(t.TempDir(), nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "DATABASE_SSL_CA")
}

func TestDatabaseConnectionFromEnvSSLDisabledByZero(t *testing.T) {
	t.Setenv("DATABASE_URL", "mysql://app:pw@db.example.com:3306/shop")
	t.Setenv("DATABASE_SSL_CA", "0")
	t.Setenv("DATABASE_SSL_CERT", "")
	t.Setenv("DATABASE_SSL_KEY", "")

	conn, err := databaseConnectionFromEnv(t.TempDir(), nil)
	require.NoError(t, err)
	assert.False(t, conn.TLS.Enabled(), "PHP treats \"0\" as unset")
}

func TestDatabaseTLSModes(t *testing.T) {
	caCert, ca, caKey := newTestCert(t, "Test CA", true, nil, nil)
	server, _, _ := newTestCert(t, "db.internal", false, ca, caKey)
	otherCA, _, _ := newTestCert(t, "Other CA", true, nil, nil)

	cases := []struct {
		name    string
		tls     DatabaseTLS
		host    string
		wantErr bool
	}{
		{"verify-identity matching host", DatabaseTLS{Mode: TLSModeVerifyIdentity, CA: caCert.certPEM}, "db.internal", false},
		{"verify-identity wrong host", DatabaseTLS{Mode: TLSModeVerifyIdentity, CA: caCert.certPEM}, "127.0.0.1", true},
		{"verify-identity server name override", DatabaseTLS{Mode: TLSModeVerifyIdentity, CA: caCert.certPEM, ServerName: "db.internal"}, "127.0.0.1", false},
		{"verify-ca ignores host", DatabaseTLS{Mode: TLSModeVerifyCA, CA: caCert.certPEM}, "127.0.0.1", false},
		{"verify-ca untrusted", DatabaseTLS{Mode: TLSModeVerifyCA, CA: otherCA.certPEM}, "db.internal", true},
		{"verify-identity untrusted", DatabaseTLS{Mode: TLSModeVerifyIdentity, CA: otherCA.certPEM}, "db.internal", true},
		{"required skips verification", DatabaseTLS{Mode: TLSModeRequired, CA: otherCA.certPEM}, "127.0.0.1", false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cfg, err := tc.tls.tlsConfig(tc.host)
			require.NoError(t, err)

			err = handshake(t, cfg, server)
			if tc.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestDatabaseTLSConfigErrors(t *testing.T) {
	cfg, err := DatabaseTLS{}.tlsConfig("db")
	require.NoError(t, err)
	assert.Nil(t, cfg)

	_, err = DatabaseTLS{Mode: TLSModeVerifyIdentity, CA: []byte("garbage")}.tlsConfig("db")
	assert.ErrorContains(t, err, "CA certificate")

	client, _, _ := newTestCert(t, "client", false, nil, nil)

	_, err = DatabaseTLS{Mode: TLSModeRequired, Cert: client.certPEM}.tlsConfig("db")
	assert.ErrorContains(t, err, "together")

	cfg, err = DatabaseTLS{Mode: TLSModeRequired, Cert: client.certPEM, Key: client.keyPEM}.tlsConfig("db")
	require.NoError(t, err)
	assert.Len(t, cfg.Certificates, 1)
}

func TestDatabaseConnectionPreferredAllowsFallback(t *testing.T) {
	conn := &DatabaseConnection{Host: "db", Port: "3306", TLS: DatabaseTLS{Mode: TLSModePreferred}}

	cfg, err := conn.MySQLConfig()
	require.NoError(t, err)
	assert.True(t, cfg.AllowFallbackToPlaintext)
	assert.True(t, cfg.TLS.InsecureSkipVerify)
}
