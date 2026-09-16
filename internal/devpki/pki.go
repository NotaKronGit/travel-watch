// Package devpki generates disposable local-development identities, never production keys.
package devpki

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/base64"
	"encoding/pem"
	"github.com/NotaKronGit/travel-watch/api/mtls"
	"math/big"
	"time"
)

type Authority struct {
	cert *x509.Certificate
	key  ed25519.PrivateKey
	pem  []byte
}

func serial() (*big.Int, error) { return rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128)) }
func New() (*Authority, error) {
	pub, key, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return nil, err
	}
	id, err := serial()
	if err != nil {
		return nil, err
	}
	c := &x509.Certificate{SerialNumber: id, Subject: pkix.Name{CommonName: "Travel Watch local development CA"}, NotBefore: time.Now().Add(-time.Minute), NotAfter: time.Now().AddDate(0, 3, 0), IsCA: true, BasicConstraintsValid: true, KeyUsage: x509.KeyUsageCertSign | x509.KeyUsageCRLSign}
	der, err := x509.CreateCertificate(rand.Reader, c, c, pub, key)
	if err != nil {
		return nil, err
	}
	return &Authority{cert: c, key: key, pem: pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})}, nil
}
func (a *Authority) Issue(name, peer string, server bool) (mtls.Config, error) {
	pub, key, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return mtls.Config{}, err
	}
	id, err := serial()
	if err != nil {
		return mtls.Config{}, err
	}
	usage := x509.ExtKeyUsageClientAuth
	if server {
		usage = x509.ExtKeyUsageServerAuth
	}
	c := &x509.Certificate{SerialNumber: id, Subject: pkix.Name{CommonName: name}, DNSNames: []string{name}, NotBefore: time.Now().Add(-time.Minute), NotAfter: time.Now().AddDate(0, 1, 0), BasicConstraintsValid: true, KeyUsage: x509.KeyUsageDigitalSignature, ExtKeyUsage: []x509.ExtKeyUsage{usage}}
	der, err := x509.CreateCertificate(rand.Reader, c, a.cert, pub, a.key)
	if err != nil {
		return mtls.Config{}, err
	}
	raw, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		return mtls.Config{}, err
	}
	b64 := base64.StdEncoding.EncodeToString
	return mtls.Config{CABase64: b64(a.pem), CertBase64: b64(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})), KeyBase64: b64(pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: raw})), PeerName: peer}, nil
}
