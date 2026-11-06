package config

import (
	"crypto/tls"
	"crypto/x509"
	"errors"
	"net/http"
	"net/netip"
	"net/url"
	"os"
	"sync/atomic"
	"time"
)

type ResourceMonitoring struct {
	EventsURL, SnapshotURL, IncidentsURL string
	transport                            *http.Transport
}

var resourceMonitoring atomic.Pointer[ResourceMonitoring]

func ConfigureResourceMonitoring(events, snapshot, incidents, ca, cert, key string) error {
	remote := false
	for _, address := range []string{events, snapshot, incidents} {
		if address == "" {
			continue
		}
		u, err := url.Parse(address)
		if err != nil || u.Host == "" || u.User != nil || u.Fragment != "" {
			return errors.New("invalid resource monitoring endpoint")
		}
		ip, _ := netip.ParseAddr(u.Hostname())
		local := u.Hostname() == "localhost" || ip.IsLoopback()
		if u.Scheme != "https" && !(u.Scheme == "http" && local) {
			return errors.New("resource monitoring requires HTTPS outside loopback")
		}
		remote = remote || !local
	}
	if (cert == "") != (key == "") || (remote && cert == "") {
		return errors.New("resource monitoring requires a client certificate and key outside loopback")
	}
	transport := http.DefaultTransport.(*http.Transport).Clone()
	tlsConfig := &tls.Config{MinVersion: tls.VersionTLS12}
	if cert != "" {
		pair, err := tls.LoadX509KeyPair(cert, key)
		if err != nil {
			return errors.New("unable to load resource client certificate")
		}
		tlsConfig.Certificates = []tls.Certificate{pair}
	}
	if ca != "" {
		raw, err := os.ReadFile(ca)
		if err != nil {
			return errors.New("unable to load resource CA")
		}
		roots, _ := x509.SystemCertPool()
		if roots == nil {
			roots = x509.NewCertPool()
		}
		if !roots.AppendCertsFromPEM(raw) {
			return errors.New("invalid resource CA")
		}
		tlsConfig.RootCAs = roots
	}
	transport.TLSClientConfig = tlsConfig
	next := &ResourceMonitoring{events, snapshot, incidents, transport}
	if old := resourceMonitoring.Swap(next); old != nil {
		old.transport.CloseIdleConnections()
	}
	return nil
}

func ResourceMonitoringSource() ResourceMonitoring {
	if v := resourceMonitoring.Load(); v != nil {
		return *v
	}
	return ResourceMonitoring{}
}

func ResourceHTTPClient(timeout time.Duration) *http.Client {
	var transport http.RoundTripper = http.DefaultTransport
	if v := resourceMonitoring.Load(); v != nil {
		transport = v.transport
	}
	return &http.Client{Timeout: timeout, Transport: transport, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
}
