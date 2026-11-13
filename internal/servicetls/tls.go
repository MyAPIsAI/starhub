// Package servicetls provides an optional sidecar transport boundary. Backend
// service credentials remain mandatory and independent of certificate identity.
package servicetls

import (
	"crypto/tls"
	"crypto/x509"
	"errors"
	"net/url"
	"time"
)

func identityVerifier(roots *x509.CertPool, expected string, usage x509.ExtKeyUsage) (func(tls.ConnectionState) error, error) {
	uri, err := url.Parse(expected)
	if err != nil || uri.Scheme != "spiffe" || uri.Host == "" || uri.Path == "" || uri.User != nil || uri.RawQuery != "" || uri.Fragment != "" || uri.Port() != "" || roots == nil || len(roots.Subjects()) == 0 {
		return nil, errors.New("a trusted CA and an explicit SPIFFE peer URI are required")
	}
	return func(state tls.ConnectionState) error {
		if len(state.PeerCertificates) == 0 || len(state.VerifiedChains) == 0 {
			return errors.New("verified peer certificate is required")
		}
		leaf := state.PeerCertificates[0]
		if len(leaf.URIs) != 1 || leaf.URIs[0].String() != expected {
			return errors.New("service peer identity does not match")
		}
		// Verify again on resumed sessions so certificate validity never relies on
		// an earlier session ticket's verification time.
		intermediates := x509.NewCertPool()
		for _, certificate := range state.PeerCertificates[1:] {
			intermediates.AddCert(certificate)
		}
		_, err := leaf.Verify(x509.VerifyOptions{Roots: roots, Intermediates: intermediates, CurrentTime: time.Now(), KeyUsages: []x509.ExtKeyUsage{usage}})
		return err
	}, nil
}

func ServerConfig(certificate tls.Certificate, roots *x509.CertPool, peerURI string) (*tls.Config, error) {
	verify, err := identityVerifier(roots, peerURI, x509.ExtKeyUsageClientAuth)
	if err != nil {
		return nil, err
	}
	if len(certificate.Certificate) == 0 || certificate.PrivateKey == nil {
		return nil, errors.New("server certificate and private key are required")
	}
	return &tls.Config{MinVersion: tls.VersionTLS13, Certificates: []tls.Certificate{certificate}, ClientAuth: tls.RequireAndVerifyClientCert, ClientCAs: roots, VerifyConnection: verify}, nil
}

func ClientConfig(certificate tls.Certificate, roots *x509.CertPool, peerURI, serverName string) (*tls.Config, error) {
	verify, err := identityVerifier(roots, peerURI, x509.ExtKeyUsageServerAuth)
	if err != nil {
		return nil, err
	}
	if len(certificate.Certificate) == 0 || certificate.PrivateKey == nil || serverName == "" {
		return nil, errors.New("client certificate, private key and upstream server name are required")
	}
	return &tls.Config{MinVersion: tls.VersionTLS13, Certificates: []tls.Certificate{certificate}, RootCAs: roots, ServerName: serverName, VerifyConnection: verify}, nil
}
