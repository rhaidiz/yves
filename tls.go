package yves

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"errors"
	"fmt"
	"math/big"
	"net"
	"time"
)

// certs is used to mantain a map of certificates that have already been created.
var certs map[string]*tls.Certificate

// Some constants for creating certificates.
const (
	caMaxAge   = 5 * 365 * 24 * time.Hour
	leafMaxAge = 24 * time.Hour
	caUsage    = x509.KeyUsageDigitalSignature |
		x509.KeyUsageContentCommitment |
		x509.KeyUsageKeyEncipherment |
		x509.KeyUsageDataEncipherment |
		x509.KeyUsageKeyAgreement |
		x509.KeyUsageCertSign |
		x509.KeyUsageCRLSign
	leafUsage = caUsage
)

// getCert obtains a certificate for a given hostname. If a certificate
// has already been created for that hostname, it is retrieved and returned.
func getCert(ca tls.Certificate, host string) (*tls.Certificate, error) {
	if val, ok := certs[host]; ok {
		return val, nil
	}
	cert, err := GenerateCert(ca, host)
	if err != nil {
		return nil, err
	}
	// save host and cert so that the next time I won't regenerate the certificate.
	certs[host] = cert
	return cert, nil
}

// GenerateCert generates a new tls.Certificate certificate to present to the client.
func GenerateCert(ca tls.Certificate, host string) (*tls.Certificate, error) {
	// basic example from https://golang.org/src/crypto/tls/generate_cert.go
	now := time.Now().Add(-1 * time.Hour).UTC()
	if !ca.Leaf.IsCA {
		return nil, errors.New("CA Certificate is not really a CA.")
	}
	serialNumberLimit := new(big.Int).Lsh(big.NewInt(1), 128)
	serialNumber, err := rand.Int(rand.Reader, serialNumberLimit)
	if err != nil {
		return nil, fmt.Errorf("Failed to generate serial number: %s", err)
	}
	template := &x509.Certificate{
		SerialNumber:          serialNumber,
		Subject:               pkix.Name{CommonName: host},
		NotBefore:             now,
		NotAfter:              now.Add(leafMaxAge),
		KeyUsage:              x509.KeyUsageDigitalSignature,
		BasicConstraintsValid: true,
	}

	if ip := net.ParseIP(host); ip != nil {
		template.IPAddresses = append(template.IPAddresses, ip)
	} else {
		template.DNSNames = append(template.DNSNames, host)
	}

	key, err := ecdsa.GenerateKey(elliptic.P384(), rand.Reader)
	if err != nil {
		return nil, err
	}

	derBytes, err := x509.CreateCertificate(rand.Reader, template, ca.Leaf, key.Public(), ca.PrivateKey)
	if err != nil {
		return nil, err
	}

	// Generate the certificate to provide the client.
	cert := new(tls.Certificate)
	cert.Certificate = append(cert.Certificate, derBytes)
	cert.PrivateKey = key
	cert.Leaf, _ = x509.ParseCertificate(derBytes)

	return cert, nil
}
