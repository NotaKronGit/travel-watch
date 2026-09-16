// Package mtls configures mutual TLS for internal RPC; plaintext is unsupported.
package mtls

import (
	"crypto/tls"
	"crypto/x509"
	"encoding/base64"
	"errors"
)

type Config struct {
	CABase64   string `mapstructure:"ca_base64" json:"-"`
	CertBase64 string `mapstructure:"cert_base64" json:"-"`
	KeyBase64  string `mapstructure:"key_base64" json:"-"`
	PeerName   string `mapstructure:"peer_name"`
}

func (c Config) Validate() error {
	if c.CABase64 == "" || c.CertBase64 == "" || c.KeyBase64 == "" || c.PeerName == "" {
		return errors.New("mTLS CA, certificate, key and peer identity required")
	}
	return nil
}
func (c Config) Load(server bool) (*tls.Config, error) {
	if err := c.Validate(); err != nil {
		return nil, err
	}
	certPEM, err := base64.StdEncoding.DecodeString(c.CertBase64)
	if err != nil {
		return nil, errors.New("invalid certificate Base64")
	}
	keyPEM, err := base64.StdEncoding.DecodeString(c.KeyBase64)
	if err != nil {
		return nil, errors.New("invalid key Base64")
	}
	cert, err := tls.X509KeyPair(certPEM, keyPEM)
	if err != nil {
		return nil, errors.New("cannot load mTLS certificate/key")
	}
	pem, err := base64.StdEncoding.DecodeString(c.CABase64)
	if err != nil {
		return nil, errors.New("cannot read mTLS CA")
	}
	roots := x509.NewCertPool()
	if !roots.AppendCertsFromPEM(pem) {
		return nil, errors.New("invalid mTLS CA")
	}
	cfg := &tls.Config{MinVersion: tls.VersionTLS13, Certificates: []tls.Certificate{cert}, NextProtos: []string{"h2"}}
	if server {
		cfg.ClientCAs = roots
		cfg.ClientAuth = tls.RequireAndVerifyClientCert
		cfg.VerifyConnection = func(state tls.ConnectionState) error {
			if len(state.VerifiedChains) == 0 || len(state.PeerCertificates) == 0 {
				return errors.New("unverified RPC client")
			}
			// Trust in the CA alone does not grant another service Cabinet's rights.
			if state.PeerCertificates[0].VerifyHostname(c.PeerName) != nil {
				return errors.New("RPC client identity not allowed")
			}
			return nil
		}
	} else {
		cfg.RootCAs = roots
		cfg.ServerName = c.PeerName
	}
	return cfg, nil
}
