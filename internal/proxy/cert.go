package proxy

import (
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/shopware/shopware-cli/internal/mkcert"
)

const (
	serverCertFile = "cert.pem"
	serverKeyFile  = "key.pem"

	// renewBefore triggers regeneration when the server certificate is close to expiry.
	renewBefore = 30 * 24 * time.Hour
)

// ServerCertPath returns the path of the server certificate used by Traefik.
func ServerCertPath(dir string) string {
	return filepath.Join(dir, "traefik", "certs", serverCertFile)
}

// ServerKeyPath returns the path of the server certificate key used by Traefik.
func ServerKeyPath(dir string) string {
	return filepath.Join(dir, "traefik", "certs", serverKeyFile)
}

// CertInfo describes the certificate the proxy serves.
type CertInfo struct {
	// CAPath is the mkcert root CA certificate the server certificate chains to.
	CAPath string
	// Changed is true when the server certificate was (re-)generated.
	Changed bool
	// CACreated is true when a new root CA was created (it is not trusted yet).
	CACreated bool
}

// EnsureCertificate makes sure a server certificate covering all hosts
// exists. Certificates are issued by the mkcert root CA in mkcert's CAROOT
// (created when missing), so they are trusted as soon as the user ran
// "mkcert -install" or "shopware-cli project proxy trust" once.
func EnsureCertificate(dir string, hosts []string) (CertInfo, error) {
	ca, caCreated, err := mkcert.LoadOrCreateCA()
	if err != nil {
		return CertInfo{}, err
	}

	info := CertInfo{CAPath: ca.CertPath(), CACreated: caCreated}

	if certCovers(ServerCertPath(dir), hosts) && fileExists(ServerKeyPath(dir)) && certIssuedBy(ServerCertPath(dir), ca.Cert) {
		return info, nil
	}

	if !ca.HasKey() {
		return CertInfo{}, fmt.Errorf("the CA in %s has no private key (rootCA-key.pem), certificates cannot be issued", filepath.Dir(ca.CertPath()))
	}

	if err := os.MkdirAll(filepath.Dir(ServerCertPath(dir)), 0o700); err != nil {
		return CertInfo{}, err
	}

	if err := ca.MakeCert(hosts, ServerCertPath(dir), ServerKeyPath(dir)); err != nil {
		return CertInfo{}, err
	}

	info.Changed = true

	return info, nil
}

// CACertPath loads (or creates) the mkcert root CA and returns the path of
// its certificate.
func CACertPath() (string, error) {
	ca, _, err := mkcert.LoadOrCreateCA()
	if err != nil {
		return "", err
	}

	return ca.CertPath(), nil
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

// certCovers reports whether the certificate at certPath contains all hosts
// as SANs and is not close to expiry.
func certCovers(certPath string, hosts []string) bool {
	cert, err := readCertificate(certPath)
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

// certIssuedBy reports whether the certificate at certPath is signed by caCert.
func certIssuedBy(certPath string, caCert *x509.Certificate) bool {
	cert, err := readCertificate(certPath)
	if err != nil {
		return false
	}

	return cert.CheckSignatureFrom(caCert) == nil
}

func readCertificate(path string) (*x509.Certificate, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	block, _ := pem.Decode(content)
	if block == nil || block.Type != "CERTIFICATE" {
		return nil, fmt.Errorf("%s does not contain a certificate", path)
	}

	return x509.ParseCertificate(block.Bytes)
}

func fileExists(path string) bool {
	_, err := os.Stat(path)

	return err == nil
}
