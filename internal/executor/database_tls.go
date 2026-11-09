package executor

import (
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
)

// TLSMode mirrors the --ssl-mode values of the mysql and mysqldump clients.
type TLSMode string

const (
	// TLSModeDisabled uses an unencrypted connection.
	TLSModeDisabled TLSMode = "disabled"
	// TLSModePreferred encrypts when the server supports it, without
	// verifying the server certificate, and falls back to plaintext otherwise.
	TLSModePreferred TLSMode = "preferred"
	// TLSModeRequired always encrypts, without verifying the server certificate.
	TLSModeRequired TLSMode = "required"
	// TLSModeVerifyCA always encrypts and verifies the server certificate
	// chain, but not the host name.
	TLSModeVerifyCA TLSMode = "verify-ca"
	// TLSModeVerifyIdentity always encrypts and verifies both the certificate
	// chain and that the certificate matches the host name.
	TLSModeVerifyIdentity TLSMode = "verify-identity"
)

// TLSModes lists the accepted modes, e.g. for flag completion.
var TLSModes = []TLSMode{TLSModeDisabled, TLSModePreferred, TLSModeRequired, TLSModeVerifyCA, TLSModeVerifyIdentity}

// ParseTLSMode parses a mode, accepting the upper-case and underscore
// spelling of the mysql client as well (e.g. VERIFY_IDENTITY).
func ParseTLSMode(value string) (TLSMode, error) {
	normalized := TLSMode(strings.ReplaceAll(strings.ToLower(strings.TrimSpace(value)), "_", "-"))

	for _, mode := range TLSModes {
		if normalized == mode {
			return mode, nil
		}
	}

	return "", fmt.Errorf("invalid TLS mode %q, expected one of: disabled, preferred, required, verify-ca, verify-identity", value)
}

// DatabaseTLS configures transport encryption of a database connection. The
// zero value disables TLS.
type DatabaseTLS struct {
	Mode TLSMode
	// CA, Cert and Key hold PEM encoded data. Without a CA the system roots
	// are used for verification.
	CA   []byte
	Cert []byte
	Key  []byte
	// ServerName overrides the host name the certificate is verified against.
	// Needed when the connection is made to a different address than the
	// database host, e.g. a port published by Docker.
	ServerName string

	// envFiles holds the DATABASE_SSL_* paths. They are only read when the
	// TLS configuration is built and the matching PEM field is still empty,
	// so explicit settings and a disabled mode never touch them.
	envFiles envTLSFiles
}

type envTLSFiles struct {
	ca, cert, key string
	readFile      func(string) ([]byte, error)
}

// pem returns data, or the content of the DATABASE_SSL_* file at path when
// data is empty.
func (f envTLSFiles) pem(data []byte, envName, path string) ([]byte, error) {
	if len(data) > 0 || path == "" || f.readFile == nil {
		return data, nil
	}

	content, err := f.readFile(path)
	if err != nil {
		return nil, fmt.Errorf("could not read %s %q: %w", envName, path, err)
	}

	return content, nil
}

// Enabled reports whether a TLS handshake is attempted.
func (t DatabaseTLS) Enabled() bool {
	return t.Mode != "" && t.Mode != TLSModeDisabled
}

// errTLSDisabled is returned by tlsConfig when no TLS handshake is wanted.
var errTLSDisabled = errors.New("database TLS is disabled")

// tlsConfig builds the crypto/tls configuration for connecting to host.
// Callers check Enabled first.
func (t DatabaseTLS) tlsConfig(host string) (*tls.Config, error) {
	caPEM, err := t.envFiles.pem(t.CA, envDatabaseSSLCA, t.envFiles.ca)
	if err != nil {
		return nil, err
	}

	certPEM, err := t.envFiles.pem(t.Cert, envDatabaseSSLCert, t.envFiles.cert)
	if err != nil {
		return nil, err
	}

	keyPEM, err := t.envFiles.pem(t.Key, envDatabaseSSLKey, t.envFiles.key)
	if err != nil {
		return nil, err
	}

	cfg := &tls.Config{MinVersion: tls.VersionTLS12}

	serverName := t.ServerName
	if serverName == "" {
		serverName = host
	}
	cfg.ServerName = serverName

	var roots *x509.CertPool
	if len(caPEM) > 0 {
		roots = x509.NewCertPool()
		if !roots.AppendCertsFromPEM(caPEM) {
			return nil, errors.New("could not parse the database TLS CA certificate: no PEM certificate found")
		}
		cfg.RootCAs = roots
	}

	if len(certPEM) > 0 || len(keyPEM) > 0 {
		if len(certPEM) == 0 || len(keyPEM) == 0 {
			return nil, errors.New("database TLS client certificate and key must be configured together")
		}

		pair, err := tls.X509KeyPair(certPEM, keyPEM)
		if err != nil {
			return nil, fmt.Errorf("could not load the database TLS client certificate: %w", err)
		}
		cfg.Certificates = []tls.Certificate{pair}
	}

	switch t.Mode {
	case TLSModePreferred, TLSModeRequired:
		cfg.InsecureSkipVerify = true //nolint:gosec // explicitly requested by the user, matches the mysql client modes
	case TLSModeVerifyCA:
		// crypto/tls cannot verify the chain without the host name, so the
		// built-in verification is replaced by a chain-only check.
		cfg.InsecureSkipVerify = true //nolint:gosec // the chain is verified in VerifyPeerCertificate
		cfg.VerifyPeerCertificate = verifyChainOnly(roots)
	case TLSModeVerifyIdentity:
	case TLSModeDisabled:
		return nil, errTLSDisabled
	default:
		return nil, fmt.Errorf("invalid TLS mode %q", t.Mode)
	}

	return cfg, nil
}

func verifyChainOnly(roots *x509.CertPool) func([][]byte, [][]*x509.Certificate) error {
	return func(rawCerts [][]byte, _ [][]*x509.Certificate) error {
		if len(rawCerts) == 0 {
			return errors.New("database server presented no certificate")
		}

		certs := make([]*x509.Certificate, 0, len(rawCerts))
		for _, raw := range rawCerts {
			cert, err := x509.ParseCertificate(raw)
			if err != nil {
				return fmt.Errorf("could not parse database server certificate: %w", err)
			}
			certs = append(certs, cert)
		}

		intermediates := x509.NewCertPool()
		for _, cert := range certs[1:] {
			intermediates.AddCert(cert)
		}

		_, err := certs[0].Verify(x509.VerifyOptions{Roots: roots, Intermediates: intermediates})
		return err
	}
}

// tlsModeFromURLParam translates the tls query parameter of a DATABASE_URL,
// using the values of the go-sql-driver DSN.
func tlsModeFromURLParam(value string) (TLSMode, error) {
	switch strings.ToLower(value) {
	case "true", "1":
		return TLSModeVerifyIdentity, nil
	case "false", "0":
		return TLSModeDisabled, nil
	case "skip-verify":
		return TLSModeRequired, nil
	case "preferred":
		return TLSModePreferred, nil
	}

	if mode, err := ParseTLSMode(value); err == nil {
		return mode, nil
	}

	return "", fmt.Errorf("invalid tls parameter %q in DATABASE_URL, expected true, false, skip-verify, preferred or a mode like verify-ca", value)
}

// Environment variables read by Shopware's MySQLFactory to configure TLS, see
// https://developer.shopware.com/docs/guides/hosting/infrastructure/database-cluster.html
const (
	envDatabaseSSLCA               = "DATABASE_SSL_CA"
	envDatabaseSSLCert             = "DATABASE_SSL_CERT"
	envDatabaseSSLKey              = "DATABASE_SSL_KEY"
	envDatabaseSSLDontVerifyServer = "DATABASE_SSL_DONT_VERIFY_SERVER_CERT"
)

var databaseTLSEnvKeys = []string{envDatabaseSSLCA, envDatabaseSSLCert, envDatabaseSSLKey, envDatabaseSSLDontVerifyServer}

// phpTruthy matches PHP's truthiness of an env string, which Shopware's
// MySQLFactory uses to decide whether a variable is set.
func phpTruthy(value string) bool {
	return value != "" && value != "0"
}

// applyDatabaseTLSEnv configures conn.TLS from Shopware's DATABASE_SSL_*
// variables. lookup returns the value of a variable, readFile the content of a
// path as seen by Shopware (local, inside the container or on the remote
// host). The files are read lazily when the TLS configuration is built. Without
// any of the variables set conn is left untouched.
func applyDatabaseTLSEnv(conn *DatabaseConnection, lookup func(string) string, readFile func(string) ([]byte, error)) {
	ca := lookup(envDatabaseSSLCA)
	cert := lookup(envDatabaseSSLCert)
	key := lookup(envDatabaseSSLKey)
	dontVerify := phpTruthy(lookup(envDatabaseSSLDontVerifyServer))

	if !phpTruthy(ca) && !phpTruthy(cert) && !phpTruthy(key) {
		// DATABASE_SSL_DONT_VERIFY_SERVER_CERT alone does not enable TLS in
		// Shopware either, but relaxes verification of an encrypted URL.
		if dontVerify && conn.TLS.Enabled() && conn.TLS.Mode != TLSModePreferred {
			conn.TLS.Mode = TLSModeRequired
		}
		return
	}

	conn.TLS.Mode = TLSModeVerifyIdentity
	if dontVerify {
		conn.TLS.Mode = TLSModeRequired
	}

	files := envTLSFiles{readFile: readFile}

	// The docs use DATABASE_SSL_CA=true to force TLS without a CA file.
	if phpTruthy(ca) && !strings.EqualFold(ca, "true") && ca != "1" {
		files.ca = ca
	}

	if phpTruthy(cert) {
		files.cert = cert
	}

	if phpTruthy(key) {
		files.key = key
	}

	conn.TLS.envFiles = files
}

// resolveRelative makes path absolute against base, as Shopware resolves
// relative paths from the project root.
func resolveRelative(base, path string) string {
	if filepath.IsAbs(path) || base == "" {
		return path
	}

	return filepath.Join(base, path)
}
