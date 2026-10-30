// Package testenv levanta servidores IMAP y SMTP en memoria, con TLS
// autofirmado, para los tests del núcleo. Solo se usa desde tests.
package testenv

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"math/big"
	"net"
	"testing"
	"time"
)

// TLS agrupa la configuración del servidor y la del cliente de prueba.
type TLS struct {
	Server *tls.Config // certificado autofirmado para 127.0.0.1
	Client *tls.Config // solo tiene RootCAs: el cliente verifica el certificado
}

// NewTLS genera un certificado autofirmado válido para "127.0.0.1".
func NewTLS(t testing.TB) TLS {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("testenv: generar clave: %v", err)
	}
	tmpl := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: "postacontrol-test"},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(24 * time.Hour),
		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageCertSign,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
		IsCA:                  true,
		IPAddresses:           []net.IP{net.ParseIP("127.0.0.1")},
		DNSNames:              []string{"localhost"},
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		t.Fatalf("testenv: crear certificado: %v", err)
	}
	leaf, err := x509.ParseCertificate(der)
	if err != nil {
		t.Fatalf("testenv: parsear certificado: %v", err)
	}
	pool := x509.NewCertPool()
	pool.AddCert(leaf)
	return TLS{
		Server: &tls.Config{Certificates: []tls.Certificate{{Certificate: [][]byte{der}, PrivateKey: key}}},
		Client: &tls.Config{RootCAs: pool},
	}
}
