package agent

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"math/big"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// generateTestCA creates a self-signed CA certificate and key for testing.
func generateTestCA(t *testing.T) (*x509.Certificate, *ecdsa.PrivateKey) {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)

	template := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: "Test CA"},
		NotBefore:             time.Now().Add(-1 * time.Hour),
		NotAfter:              time.Now().Add(24 * time.Hour),
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageCRLSign,
		BasicConstraintsValid: true,
		IsCA:                  true,
	}

	certDER, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	require.NoError(t, err)

	cert, err := x509.ParseCertificate(certDER)
	require.NoError(t, err)

	return cert, key
}

// signCert creates a leaf certificate signed by the given CA.
func signCert(t *testing.T, ca *x509.Certificate, caKey *ecdsa.PrivateKey, template *x509.Certificate) (*x509.Certificate, *ecdsa.PrivateKey) {
	t.Helper()
	leafKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)

	certDER, err := x509.CreateCertificate(rand.Reader, template, ca, &leafKey.PublicKey, caKey)
	require.NoError(t, err)

	cert, err := x509.ParseCertificate(certDER)
	require.NoError(t, err)

	return cert, leafKey
}

func TestVerifyKubeletCert_ValidCert_WithCA(t *testing.T) {
	ca, caKey := generateTestCA(t)

	leaf, _ := signCert(t, ca, caKey, &x509.Certificate{
		SerialNumber: big.NewInt(2),
		Subject:      pkix.Name{CommonName: "kubelet"},
		NotBefore:    time.Now().Add(-1 * time.Hour),
		NotAfter:     time.Now().Add(24 * time.Hour),
	})

	caPool := x509.NewCertPool()
	caPool.AddCert(ca)

	verify := verifyKubeletCert(caPool)
	err := verify([][]byte{leaf.Raw}, nil)
	assert.NoError(t, err)
}

func TestVerifyKubeletCert_ExpiredCert(t *testing.T) {
	ca, caKey := generateTestCA(t)

	leaf, _ := signCert(t, ca, caKey, &x509.Certificate{
		SerialNumber: big.NewInt(3),
		Subject:      pkix.Name{CommonName: "kubelet-expired"},
		NotBefore:    time.Now().Add(-48 * time.Hour),
		NotAfter:     time.Now().Add(-1 * time.Hour), // expired
	})

	caPool := x509.NewCertPool()
	caPool.AddCert(ca)

	verify := verifyKubeletCert(caPool)
	err := verify([][]byte{leaf.Raw}, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "expired")
}

func TestVerifyKubeletCert_NoCerts(t *testing.T) {
	verify := verifyKubeletCert(nil)
	err := verify([][]byte{}, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no TLS certificates")
}

func TestVerifyKubeletCert_ValidCert_NoCA(t *testing.T) {
	// Out-of-cluster mode: nil caPool — only expiry checked.
	ca, caKey := generateTestCA(t)

	leaf, _ := signCert(t, ca, caKey, &x509.Certificate{
		SerialNumber: big.NewInt(4),
		Subject:      pkix.Name{CommonName: "kubelet-noca"},
		NotBefore:    time.Now().Add(-1 * time.Hour),
		NotAfter:     time.Now().Add(24 * time.Hour),
	})

	verify := verifyKubeletCert(nil) // no CA pool
	err := verify([][]byte{leaf.Raw}, nil)
	assert.NoError(t, err) // passes — only expiry checked when caPool is nil
}

func TestVerifyKubeletCert_WrongCA(t *testing.T) {
	// Cert signed by CA-A, verified against CA-B.
	caA, caKeyA := generateTestCA(t)
	caB, _ := generateTestCA(t)

	leaf, _ := signCert(t, caA, caKeyA, &x509.Certificate{
		SerialNumber: big.NewInt(5),
		Subject:      pkix.Name{CommonName: "kubelet-wrongca"},
		NotBefore:    time.Now().Add(-1 * time.Hour),
		NotAfter:     time.Now().Add(24 * time.Hour),
	})

	caBPool := x509.NewCertPool()
	caBPool.AddCert(caB) // wrong CA

	verify := verifyKubeletCert(caBPool)
	err := verify([][]byte{leaf.Raw}, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "chain verification failed")
}

func TestVerifyKubeletCert_NotYetValid(t *testing.T) {
	ca, caKey := generateTestCA(t)

	leaf, _ := signCert(t, ca, caKey, &x509.Certificate{
		SerialNumber: big.NewInt(6),
		Subject:      pkix.Name{CommonName: "kubelet-future"},
		NotBefore:    time.Now().Add(24 * time.Hour), // not yet valid
		NotAfter:     time.Now().Add(48 * time.Hour),
	})

	caPool := x509.NewCertPool()
	caPool.AddCert(ca)

	verify := verifyKubeletCert(caPool)
	err := verify([][]byte{leaf.Raw}, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "expired")
}

func TestVerifyKubeletCert_IntermediateChain(t *testing.T) {
	// CA → intermediate → leaf
	rootCA, rootKey := generateTestCA(t)

	intermediateTemplate := &x509.Certificate{
		SerialNumber:          big.NewInt(10),
		Subject:               pkix.Name{CommonName: "Intermediate CA"},
		NotBefore:             time.Now().Add(-1 * time.Hour),
		NotAfter:              time.Now().Add(24 * time.Hour),
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageCRLSign,
		BasicConstraintsValid: true,
		IsCA:                  true,
	}
	intermediate, intermediateKey := signCert(t, rootCA, rootKey, intermediateTemplate)

	leaf, _ := signCert(t, intermediate, intermediateKey, &x509.Certificate{
		SerialNumber: big.NewInt(11),
		Subject:      pkix.Name{CommonName: "kubelet-intermediate"},
		NotBefore:    time.Now().Add(-1 * time.Hour),
		NotAfter:     time.Now().Add(24 * time.Hour),
	})

	rootPool := x509.NewCertPool()
	rootPool.AddCert(rootCA)

	// Send leaf + intermediate as the raw cert chain.
	verify := verifyKubeletCert(rootPool)
	err := verify([][]byte{leaf.Raw, intermediate.Raw}, nil)
	assert.NoError(t, err)
}
