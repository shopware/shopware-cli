package project

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/base64"
	"encoding/pem"
	"fmt"
	"math/big"
	"os"
	"path"
	"path/filepath"
	"time"

	"github.com/spf13/cobra"
)

var projectNewJWTCmd = &cobra.Command{
	Use:   "generate-jwt",
	Short: "Generate a new JWT secret key",
	RunE: func(cmd *cobra.Command, args []string) error {
		publicKey, privateKey, err := generatePrivatePublicKey(2048)
		if err != nil {
			return err
		}

		envMode, _ := cmd.PersistentFlags().GetBool("env")

		if envMode {
			fmt.Printf("JWT_PRIVATE_KEY=%s\n", base64.StdEncoding.EncodeToString(privateKey))
			fmt.Printf("JWT_PUBLIC_KEY=%s\n", base64.StdEncoding.EncodeToString(publicKey))

			return nil
		}

		if len(args) == 0 {
			return fmt.Errorf("project root path is required, please pass a path to the project root")
		}

		projectRoot := args[0]
		jwtFolder := path.Join(projectRoot, "config", "jwt")

		if _, err := os.Stat(jwtFolder); os.IsNotExist(err) {
			if err := os.MkdirAll(jwtFolder, 0o700); err != nil {
				return err
			}
		}

		if err := os.WriteFile(filepath.Join(jwtFolder, "private.pem"), privateKey, 0o600); err != nil {
			return err
		}

		if err := os.WriteFile(filepath.Join(jwtFolder, "public.pem"), publicKey, 0o600); err != nil {
			return err
		}

		return nil
	},
}

func init() {
	projectRootCmd.AddCommand(projectNewJWTCmd)
	projectNewJWTCmd.PersistentFlags().Bool("env", false, "Provide secrets as environment variables")
}

func generatePrivatePublicKey(keyLength int) ([]byte, []byte, error) {
	rsaKey, err := generatePrivateKey(keyLength)
	if err != nil {
		return nil, nil, err
	}

	rsaPrivKey := encodePrivateKeyToPEM(rsaKey)
	rsaPubKey, err := generatePublicKey(rsaKey)
	if err != nil {
		return nil, nil, fmt.Errorf("unable to generate public key: %v", err)
	}

	return rsaPubKey, rsaPrivKey, nil
}

// generatePrivateKey creates a RSA Private Key of specified byte size.
func generatePrivateKey(bitSize int) (*rsa.PrivateKey, error) {
	// Private Key generation
	privateKey, err := rsa.GenerateKey(rand.Reader, bitSize)
	if err != nil {
		return nil, err
	}

	// Validate Private Key
	err = privateKey.Validate()
	if err != nil {
		return nil, err
	}

	return privateKey, nil
}

func encodePrivateKeyToPEM(privateKey *rsa.PrivateKey) []byte {
	key := &pem.Block{
		Type:  "RSA PRIVATE KEY",
		Bytes: x509.MarshalPKCS1PrivateKey(privateKey),
	}

	return pem.EncodeToMemory(key)
}

func generatePublicKey(key *rsa.PrivateKey) ([]byte, error) {
	template := x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject: pkix.Name{
			Organization: []string{"Acme Co"},
		},
		NotBefore: time.Now(),
		NotAfter:  time.Now().Add(time.Hour * 24 * 180),

		KeyUsage:              x509.KeyUsageKeyEncipherment | x509.KeyUsageDigitalSignature,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
	}

	derBytes, err := x509.CreateCertificate(rand.Reader, &template, &template, &key.PublicKey, key)
	if err != nil {
		return nil, err
	}

	pubPem := &pem.Block{Type: "CERTIFICATE", Bytes: derBytes}

	return pem.EncodeToMemory(pubPem), nil
}
