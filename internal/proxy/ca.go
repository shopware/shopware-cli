package proxy

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
	"slices"
	"strings"
	"time"
)

const (
	caCertFile     = "rootCA.pem"
	caKeyFile      = "rootCA-key.pem"
	serverCertFile = "cert.pem"
	serverKeyFile  = "key.pem"

	caValidity     = 10 * 365 * 24 * time.Hour
	serverValidity = 825 * 24 * time.Hour
	// renewBefore triggers regeneration when the server certificate is close to expiry.
	renewBefore = 30 * 24 * time.Hour
)

// CACertPath returns the path of the local CA certificate inside the proxy directory.
func CACertPath(dir string) string {
	return filepath.Join(dir, "ca", caCertFile)
}

func caKeyPath(dir string) string {
	return filepath.Join(dir, "ca", caKeyFile)
}

// ServerCertPath returns the path of the server certificate used by Traefik.
func ServerCertPath(dir string) string {
	return filepath.Join(dir, "traefik", "certs", serverCertFile)
}

// ServerKeyPath returns the path of the server certificate key used by Traefik.
func ServerKeyPath(dir string) string {
	return filepath.Join(dir, "traefik", "certs", serverKeyFile)
}

// EnsureCA creates the local certificate authority if it does not exist yet.
// It returns true when a new CA has been created.
func EnsureCA(dir string) (bool, error) {
	certPath, keyPath := CACertPath(dir), caKeyPath(dir)

	if fileExists(certPath) && fileExists(keyPath) {
		return false, nil
	}

	if err := os.MkdirAll(filepath.Dir(certPath), 0o700); err != nil {
		return false, err
	}

	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return false, fmt.Errorf("generate CA key: %w", err)
	}

	serial, err := randomSerial()
	if err != nil {
		return false, err
	}

	template := &x509.Certificate{
		SerialNumber: serial,
		Subject: pkix.Name{
			Organization:       []string{"shopware-cli development CA"},
			OrganizationalUnit: []string{"shopware-cli"},
			CommonName:         "shopware-cli development CA",
		},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(caValidity),
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageCRLSign,
		BasicConstraintsValid: true,
		IsCA:                  true,
		MaxPathLenZero:        true,
	}

	der, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	if err != nil {
		return false, fmt.Errorf("create CA certificate: %w", err)
	}

	if err := writePem(certPath, "CERTIFICATE", der, 0o644); err != nil {
		return false, err
	}

	keyDer, err := x509.MarshalECPrivateKey(key)
	if err != nil {
		return false, err
	}

	if err := writePem(keyPath, "EC PRIVATE KEY", keyDer, 0o600); err != nil {
		return false, err
	}

	return true, nil
}

// EnsureServerCert makes sure a server certificate signed by the local CA
// exists and covers all given hosts. It returns true when the certificate has
// been (re-)generated.
func EnsureServerCert(dir string, hosts []string) (bool, error) {
	if len(hosts) == 0 {
		return false, fmt.Errorf("at least one host is required")
	}

	certPath, keyPath := ServerCertPath(dir), ServerKeyPath(dir)

	if certCovers(certPath, hosts) && fileExists(keyPath) {
		return false, nil
	}

	caCert, caKey, err := loadCA(dir)
	if err != nil {
		return false, err
	}

	if err := os.MkdirAll(filepath.Dir(certPath), 0o700); err != nil {
		return false, err
	}

	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return false, fmt.Errorf("generate server key: %w", err)
	}

	serial, err := randomSerial()
	if err != nil {
		return false, err
	}

	template := &x509.Certificate{
		SerialNumber: serial,
		Subject: pkix.Name{
			Organization: []string{"shopware-cli development certificate"},
			CommonName:   hosts[0],
		},
		NotBefore:   time.Now().Add(-time.Hour),
		NotAfter:    time.Now().Add(serverValidity),
		KeyUsage:    x509.KeyUsageDigitalSignature,
		ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		DNSNames:    slices.Clone(hosts),
	}

	der, err := x509.CreateCertificate(rand.Reader, template, caCert, &key.PublicKey, caKey)
	if err != nil {
		return false, fmt.Errorf("create server certificate: %w", err)
	}

	if err := writePem(certPath, "CERTIFICATE", der, 0o644); err != nil {
		return false, err
	}

	keyDer, err := x509.MarshalECPrivateKey(key)
	if err != nil {
		return false, err
	}

	if err := writePem(keyPath, "EC PRIVATE KEY", keyDer, 0o600); err != nil {
		return false, err
	}

	return true, nil
}

// CertHosts declares the SANs needed for a base domain plus explicitly
// registered hosts. The wildcard covers one subdomain level, which matches
// <project>.<domain>.
func CertHosts(domain string, extraHosts []string) []string {
	hosts := []string{domain, "*." + domain}

	for _, host := range extraHosts {
		if host == domain || host == "*."+domain || matchesWildcard(host, domain) {
			continue
		}

		if !slices.Contains(hosts, host) {
			hosts = append(hosts, host)
		}
	}

	return hosts
}

func matchesWildcard(host, domain string) bool {
	sub, found := strings.CutSuffix(host, "."+domain)

	return found && sub != "" && !strings.Contains(sub, ".")
}

func loadCA(dir string) (*x509.Certificate, *ecdsa.PrivateKey, error) {
	certDer, err := readPem(CACertPath(dir), "CERTIFICATE")
	if err != nil {
		return nil, nil, fmt.Errorf("load CA certificate: %w", err)
	}

	cert, err := x509.ParseCertificate(certDer)
	if err != nil {
		return nil, nil, fmt.Errorf("parse CA certificate: %w", err)
	}

	keyDer, err := readPem(caKeyPath(dir), "EC PRIVATE KEY")
	if err != nil {
		return nil, nil, fmt.Errorf("load CA key: %w", err)
	}

	key, err := x509.ParseECPrivateKey(keyDer)
	if err != nil {
		return nil, nil, fmt.Errorf("parse CA key: %w", err)
	}

	return cert, key, nil
}

func certCovers(certPath string, hosts []string) bool {
	der, err := readPem(certPath, "CERTIFICATE")
	if err != nil {
		return false
	}

	cert, err := x509.ParseCertificate(der)
	if err != nil {
		return false
	}

	if time.Now().Add(renewBefore).After(cert.NotAfter) {
		return false
	}

	for _, host := range hosts {
		if !slices.Contains(cert.DNSNames, host) {
			return false
		}
	}

	return true
}

func randomSerial() (*big.Int, error) {
	serial, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	if err != nil {
		return nil, fmt.Errorf("generate serial number: %w", err)
	}

	return serial, nil
}

func writePem(path, blockType string, der []byte, perm os.FileMode) error {
	return os.WriteFile(path, pem.EncodeToMemory(&pem.Block{Type: blockType, Bytes: der}), perm)
}

func readPem(path, blockType string) ([]byte, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	block, _ := pem.Decode(content)
	if block == nil || block.Type != blockType {
		return nil, fmt.Errorf("%s does not contain a %s PEM block", path, blockType)
	}

	return block.Bytes, nil
}

func fileExists(path string) bool {
	_, err := os.Stat(path)

	return err == nil
}
