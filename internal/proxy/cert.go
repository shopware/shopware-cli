package proxy

import (
	"context"
	"crypto/x509"
)

// CertInfo describes the certificate the proxy serves and which CA issued it.
type CertInfo struct {
	// CAPath is the root CA certificate the server certificate chains to.
	CAPath string
	// Changed is true when the server certificate was (re-)generated.
	Changed bool
	// Mkcert is true when the certificate was issued by the mkcert root CA.
	Mkcert bool
}

// EnsureCertificate makes sure a server certificate covering all hosts exists.
// When mkcert is installed its (usually already trusted) root CA is reused,
// otherwise a shopware-cli owned local CA is created.
func EnsureCertificate(ctx context.Context, dir string, hosts []string) (CertInfo, error) {
	if MkcertAvailable() {
		caPath, err := MkcertCAPath(ctx)
		if err != nil {
			return CertInfo{}, err
		}

		if fileExists(caPath) && certCovers(ServerCertPath(dir), hosts) && certIssuedBy(ServerCertPath(dir), caPath) && fileExists(ServerKeyPath(dir)) {
			return CertInfo{CAPath: caPath, Mkcert: true}, nil
		}

		if err := generateWithMkcert(ctx, dir, hosts); err != nil {
			return CertInfo{}, err
		}

		// mkcert creates its root CA on first use, resolve the path again.
		caPath, err = MkcertCAPath(ctx)
		if err != nil {
			return CertInfo{}, err
		}

		return CertInfo{CAPath: caPath, Changed: true, Mkcert: true}, nil
	}

	if _, err := EnsureCA(dir); err != nil {
		return CertInfo{}, err
	}

	changed, err := EnsureServerCert(dir, hosts)
	if err != nil {
		return CertInfo{}, err
	}

	return CertInfo{CAPath: CACertPath(dir), Changed: changed}, nil
}

// certIssuedBy reports whether the certificate at certPath is signed by the
// CA certificate at caPath.
func certIssuedBy(certPath, caPath string) bool {
	certDer, err := readPem(certPath, "CERTIFICATE")
	if err != nil {
		return false
	}

	cert, err := x509.ParseCertificate(certDer)
	if err != nil {
		return false
	}

	caDer, err := readPem(caPath, "CERTIFICATE")
	if err != nil {
		return false
	}

	caCert, err := x509.ParseCertificate(caDer)
	if err != nil {
		return false
	}

	return cert.CheckSignatureFrom(caCert) == nil
}
