package certs

import (
	"crypto"
	"crypto/ecdsa"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/hex"
	"encoding/pem"
	"errors"
	"fmt"
	"math/big"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"cursor/internal/appdata"
	"cursor/internal/logger"
)

// Manager defines the Manager type in this module.
type Manager struct {
	// caCert represents the caCert field in this declaration.
	caCert *x509.Certificate
	// caKey represents the caKey field in this declaration.
	caKey crypto.PrivateKey

	// mu represents the mu field in this declaration.
	mu sync.Mutex
	// cache represents the cache field in this declaration.
	cache map[string]*tls.Certificate
}

// NewManager handles logic related to NewManager.
func NewManager(caCertPath, caKeyPath string) (*Manager, error) {
	certPEM, keyPEM, err := loadCAPEMFromFiles(caCertPath, caKeyPath)
	if err != nil {
		return nil, err
	}
	return NewManagerFromPEM(certPEM, keyPEM)
}

// EnsureLocalCA checks whether a unique CA certificate and private key already exist in the local appdata.
// If not, it dynamically generates a system-wide unique 2048-bit RSA self-signed Root CA certificate and private key,
// and saves them into the local user appdata directory. This way every installation instance has its own unique private key, completely eliminating the MITM risk caused by a leaked shared private key.
func EnsureLocalCA() (*Manager, []byte, error) {
	certPath := appdata.CACertFilePath()
	keyPath := appdata.CAKeyFilePath()

	certPEM, keyPEM, err := loadCAPEMFromFiles(certPath, keyPath)
	if err == nil && len(certPEM) > 0 && len(keyPEM) > 0 {
		mgr, err := NewManagerFromPEM(certPEM, keyPEM)
		if err == nil {
			return mgr, certPEM, nil
		}
	}

	logger.Infof("EnsureLocalCA: Generating a unique local Root CA cert and private key...")
	certPEM, keyPEM, err = GenerateUniqueCA()
	if err != nil {
		return nil, nil, fmt.Errorf("generate unique local CA failed: %w", err)
	}

	if err := os.MkdirAll(filepath.Dir(certPath), 0o700); err != nil {
		return nil, nil, fmt.Errorf("create CA directory failed: %w", err)
	}

	if err := writePrivateCAFile(certPath, certPEM, 0o644); err != nil {
		return nil, nil, fmt.Errorf("write local CA cert failed: %w", err)
	}

	if err := writePrivateCAFile(keyPath, keyPEM, 0o600); err != nil {
		return nil, nil, fmt.Errorf("write local CA key failed: %w", err)
	}

	mgr, err := NewManagerFromPEM(certPEM, keyPEM)
	if err != nil {
		return nil, nil, fmt.Errorf("load generated local CA failed: %w", err)
	}

	return mgr, certPEM, nil
}

// GenerateUniqueCA dynamically generates a 2048-bit RSA self-signed Root CA certificate and private key PEM.
func GenerateUniqueCA() ([]byte, []byte, error) {
	priv, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return nil, nil, err
	}

	serialNumber, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	if err != nil {
		return nil, nil, err
	}

	suffix := make([]byte, 4)
	_, _ = rand.Read(suffix)
	commonName := "Cursor Local Proxy CA (" + hex.EncodeToString(suffix) + ")"

	template := &x509.Certificate{
		SerialNumber: serialNumber,
		Subject: pkix.Name{
			CommonName:   commonName,
			Organization: []string{"Cursor Local Proxy"},
		},
		NotBefore:             time.Now().Add(-24 * time.Hour),
		NotAfter:              time.Now().Add(10 * 365 * 24 * time.Hour),
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageCRLSign | x509.KeyUsageDigitalSignature,
		BasicConstraintsValid: true,
		IsCA:                  true,
		MaxPathLen:            1,
	}

	derBytes, err := x509.CreateCertificate(rand.Reader, template, template, &priv.PublicKey, priv)
	if err != nil {
		return nil, nil, err
	}

	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: derBytes})
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(priv)})

	return certPEM, keyPEM, nil
}

// NewEmbeddedManager keeps the old API name but only uses the locally generated/persisted CA.
func NewEmbeddedManager() (*Manager, error) {
	mgr, _, err := EnsureLocalCA()
	return mgr, err
}

// EmbeddedCACertPEM keeps the old API name but returns the public certificate of the local unique CA.
func EmbeddedCACertPEM() []byte {
	if _, certPEM, err := EnsureLocalCA(); err == nil && len(certPEM) > 0 {
		return certPEM
	}
	return nil
}

// NewManagerFromPEM handles logic related to NewManagerFromPEM.
func NewManagerFromPEM(caCertPEM, caKeyPEM []byte) (*Manager, error) {
	caCert, caKey, err := loadCAFromPEM(caCertPEM, caKeyPEM)
	if err != nil {
		return nil, err
	}
	return &Manager{caCert: caCert, caKey: caKey, cache: make(map[string]*tls.Certificate)}, nil
}
// CATLSCertificate handles logic related to CATLSCertificate.
func (m *Manager) CATLSCertificate() (*tls.Certificate, error) {
	if m == nil || m.caCert == nil || m.caKey == nil {
		return nil, errors.New("CA is not initialized")
	}
	return &tls.Certificate{
		Certificate: [][]byte{append([]byte(nil), m.caCert.Raw...)},
		PrivateKey:  m.caKey,
		Leaf:        m.caCert,
	}, nil
}

// CertificateForServerName handles logic related to CertificateForServerName.
func (m *Manager) CertificateForServerName(serverName string) (*tls.Certificate, error) {
	host := normalizeHost(serverName)
	if host == "" {
		return nil, errors.New("empty server name")
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	if cert, ok := m.cache[host]; ok {
		return cert, nil
	}

	serial, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	if err != nil {
		return nil, err
	}

	leaf := &x509.Certificate{
		SerialNumber: serial,
		Subject: pkix.Name{
			CommonName:   host,
			Organization: []string{"Cursor Local Proxy"},
		},
		NotBefore:             time.Now().Add(-1 * time.Hour),
		NotAfter:              time.Now().Add(365 * 24 * time.Hour),
		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
	}
	if len(m.caCert.SubjectKeyId) > 0 {
		leaf.AuthorityKeyId = append([]byte(nil), m.caCert.SubjectKeyId...)
	}

	if ip := net.ParseIP(host); ip != nil {
		leaf.IPAddresses = []net.IP{ip}
	} else {
		leaf.DNSNames = []string{host}
	}

	leafPrivateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return nil, err
	}
	leafPublicKey := &leafPrivateKey.PublicKey

	der, err := x509.CreateCertificate(rand.Reader, leaf, m.caCert, leafPublicKey, m.caKey)
	if err != nil {
		return nil, err
	}

	leafCertPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
	chainPEM := append([]byte(nil), leafCertPEM...)
	chainPEM = append(chainPEM, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: m.caCert.Raw})...)

	keyPEM, err := marshalPrivateKeyPEM(leafPrivateKey)
	if err != nil {
		return nil, err
	}

	pair, err := tls.X509KeyPair(chainPEM, keyPEM)
	if err != nil {
		return nil, err
	}
	parsedLeaf, err := x509.ParseCertificate(der)
	if err != nil {
		return nil, err
	}
	pair.Leaf = parsedLeaf

	m.cache[host] = &pair
	return &pair, nil
}

// marshalPrivateKeyPEM handles logic related to marshalPrivateKeyPEM.
func marshalPrivateKeyPEM(key any) ([]byte, error) {
	switch k := key.(type) {
	case *rsa.PrivateKey:
		return pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(k)}), nil
	case *ecdsa.PrivateKey:
		der, err := x509.MarshalECPrivateKey(k)
		if err != nil {
			return nil, err
		}
		return pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: der}), nil
	case ed25519.PrivateKey:
		der, err := x509.MarshalPKCS8PrivateKey(k)
		if err != nil {
			return nil, err
		}
		return pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: der}), nil
	default:
		return nil, errors.New("unsupported private key type")
	}
}

// loadCAPEMFromFiles handles logic related to loadCAPEMFromFiles.
func loadCAPEMFromFiles(certPath, keyPath string) ([]byte, []byte, error) {
	certPEM, err := os.ReadFile(certPath)
	if err != nil {
		return nil, nil, err
	}
	keyPEM, err := os.ReadFile(keyPath)
	if err != nil {
		return nil, nil, err
	}
	return certPEM, keyPEM, nil
}

// loadCAFromPEM handles logic related to loadCAFromPEM.
func loadCAFromPEM(certPEM, keyPEM []byte) (*x509.Certificate, crypto.PrivateKey, error) {
	certBlock, _ := pem.Decode(certPEM)
	if certBlock == nil {
		return nil, nil, errors.New("invalid CA cert PEM")
	}
	caCert, err := x509.ParseCertificate(certBlock.Bytes)
	if err != nil {
		return nil, nil, err
	}

	keyBlock, _ := pem.Decode(keyPEM)
	if keyBlock == nil {
		return nil, nil, errors.New("invalid CA key PEM")
	}

	switch keyBlock.Type {
	case "RSA PRIVATE KEY":
		key, err := x509.ParsePKCS1PrivateKey(keyBlock.Bytes)
		if err != nil {
			return nil, nil, err
		}
		if err := validateCAKeyPair(caCert, key); err != nil {
			return nil, nil, err
		}
		return caCert, key, nil
	case "EC PRIVATE KEY":
		key, err := x509.ParseECPrivateKey(keyBlock.Bytes)
		if err != nil {
			return nil, nil, err
		}
		if err := validateCAKeyPair(caCert, key); err != nil {
			return nil, nil, err
		}
		return caCert, key, nil
	case "PRIVATE KEY":
		key, err := x509.ParsePKCS8PrivateKey(keyBlock.Bytes)
		if err != nil {
			return nil, nil, err
		}
		if err := validateCAKeyPair(caCert, key); err != nil {
			return nil, nil, err
		}
		return caCert, key, nil
	default:
		return nil, nil, errors.New("unsupported CA key format")
	}
}

func validateCAKeyPair(cert *x509.Certificate, key crypto.PrivateKey) error {
	if cert == nil || !cert.IsCA || !cert.BasicConstraintsValid {
		return errors.New("CA certificate is not a valid CA")
	}
	if err := cert.CheckSignatureFrom(cert); err != nil {
		return fmt.Errorf("CA certificate is not self-signed: %w", err)
	}
	privateKey, ok := key.(crypto.Signer)
	if !ok || !publicKeysEqual(cert.PublicKey, privateKey.Public()) {
		return errors.New("CA certificate and private key do not match")
	}
	return nil
}

func publicKeysEqual(left, right crypto.PublicKey) bool {
	leftDER, leftErr := x509.MarshalPKIXPublicKey(left)
	rightDER, rightErr := x509.MarshalPKIXPublicKey(right)
	return leftErr == nil && rightErr == nil && string(leftDER) == string(rightDER)
}

func writePrivateCAFile(path string, data []byte, mode os.FileMode) error {
	file, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, mode)
	if err != nil {
		return err
	}
	defer file.Close()
	if err := file.Chmod(mode); err != nil {
		return err
	}
	_, err = file.Write(data)
	return err
}

// normalizeHost handles logic related to normalizeHost.
func normalizeHost(serverName string) string {
	serverName = strings.TrimSpace(serverName)
	if strings.Contains(serverName, ":") {
		h, _, err := net.SplitHostPort(serverName)
		if err == nil {
			serverName = h
		}
	}
	return serverName
}

// cloneBytes handles logic related to cloneBytes.
