package servicetls

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"io"
	"log"
	"math/big"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

type testAuthority struct {
	certificate *x509.Certificate
	key         *ecdsa.PrivateKey
	roots       *x509.CertPool
}

func authority(t *testing.T) testAuthority {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	template := &x509.Certificate{SerialNumber: big.NewInt(1), Subject: pkix.Name{CommonName: "Local acceptance fixture CA"}, NotBefore: time.Now().Add(-48 * time.Hour), NotAfter: time.Now().Add(48 * time.Hour), IsCA: true, BasicConstraintsValid: true, KeyUsage: x509.KeyUsageCertSign | x509.KeyUsageCRLSign}
	raw, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	certificate, err := x509.ParseCertificate(raw)
	if err != nil {
		t.Fatal(err)
	}
	roots := x509.NewCertPool()
	roots.AddCert(certificate)
	return testAuthority{certificate, key, roots}
}
func (ca testAuthority) issue(t *testing.T, identity string, usage x509.ExtKeyUsage, expires time.Time) tls.Certificate {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	serial, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 120))
	if err != nil {
		t.Fatal(err)
	}
	template := &x509.Certificate{SerialNumber: serial, Subject: pkix.Name{CommonName: "Local service fixture"}, NotBefore: time.Now().Add(-24 * time.Hour), NotAfter: expires, KeyUsage: x509.KeyUsageDigitalSignature, ExtKeyUsage: []x509.ExtKeyUsage{usage}, DNSNames: []string{"localhost"}, IPAddresses: []net.IP{net.ParseIP("127.0.0.1")}}
	if identity != "" {
		uri, err := url.Parse(identity)
		if err != nil {
			t.Fatal(err)
		}
		template.URIs = []*url.URL{uri}
	}
	raw, err := x509.CreateCertificate(rand.Reader, template, ca.certificate, &key.PublicKey, ca.key)
	if err != nil {
		t.Fatal(err)
	}
	leaf, err := x509.ParseCertificate(raw)
	if err != nil {
		t.Fatal(err)
	}
	return tls.Certificate{Certificate: [][]byte{raw, ca.certificate.Raw}, PrivateKey: key, Leaf: leaf}
}

func TestServiceMTLSRejectsUntrustedExpiredAndWrongIdentityCertificates(t *testing.T) {
	ca, other := authority(t), authority(t)
	serverCertificate := ca.issue(t, "spiffe://starhub/gateway/node-a", x509.ExtKeyUsageServerAuth, time.Now().Add(time.Hour))
	valid := ca.issue(t, "spiffe://starhub/resource", x509.ExtKeyUsageClientAuth, time.Now().Add(time.Hour))
	serverTLS, err := ServerConfig(serverCertificate, ca.roots, "spiffe://starhub/resource")
	if err != nil {
		t.Fatal(err)
	}
	var forwarded atomic.Int64
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		forwarded.Add(1)
		if r.Header.Get("Authorization") != "Bearer local-service-fixture" {
			w.WriteHeader(401)
			return
		}
		if strings.Contains(r.Header.Get("X-Forwarded-For"), "203.0.113.1") {
			t.Error("untrusted forwarding header crossed the proxy")
		}
		w.WriteHeader(204)
	}))
	defer backend.Close()
	target, _ := url.Parse(backend.URL)
	handler, err := Proxy(target, "gateway", nil)
	if err != nil {
		t.Fatal(err)
	}
	ingress := httptest.NewUnstartedServer(handler)
	ingress.TLS = serverTLS
	ingress.Config.ErrorLog = log.New(io.Discard, "", 0)
	ingress.StartTLS()
	defer ingress.Close()
	for _, tc := range []struct {
		name        string
		certificate tls.Certificate
		accepted    bool
	}{
		{"valid", valid, true},
		{"missing", tls.Certificate{}, false},
		{"untrusted", other.issue(t, "spiffe://starhub/resource", x509.ExtKeyUsageClientAuth, time.Now().Add(time.Hour)), false},
		{"expired", ca.issue(t, "spiffe://starhub/resource", x509.ExtKeyUsageClientAuth, time.Now().Add(-time.Hour)), false},
		{"wrong identity", ca.issue(t, "spiffe://starhub/unrelated", x509.ExtKeyUsageClientAuth, time.Now().Add(time.Hour)), false},
		{"missing identity", ca.issue(t, "", x509.ExtKeyUsageClientAuth, time.Now().Add(time.Hour)), false},
		{"wrong purpose", ca.issue(t, "spiffe://starhub/resource", x509.ExtKeyUsageServerAuth, time.Now().Add(time.Hour)), false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			before := forwarded.Load()
			transport := http.DefaultTransport.(*http.Transport).Clone()
			transport.Proxy = nil
			transport.TLSClientConfig = &tls.Config{MinVersion: tls.VersionTLS13, RootCAs: ca.roots, GetClientCertificate: func(*tls.CertificateRequestInfo) (*tls.Certificate, error) { return &tc.certificate, nil }}
			defer transport.CloseIdleConnections()
			client := &http.Client{Transport: transport, Timeout: 3 * time.Second}
			req, _ := http.NewRequest("GET", ingress.URL+"/api/metric/attempts", nil)
			req.Header.Set("Authorization", "Bearer local-service-fixture")
			req.Header.Set("X-Forwarded-For", "203.0.113.1")
			response, err := client.Do(req)
			if response != nil {
				response.Body.Close()
			}
			if tc.accepted {
				if err != nil || response.StatusCode != 204 || forwarded.Load() != before+1 {
					t.Fatalf("trusted service failed: %v", err)
				}
			} else if err == nil || forwarded.Load() != before {
				t.Fatal("rejected certificate reached the backend")
			}
		})
	}
	// Use the production client configuration for a complete egress -> mTLS
	// ingress -> application request, retaining the independent service token.
	clientTLS, err := ClientConfig(valid, ca.roots, "spiffe://starhub/gateway/node-a", "127.0.0.1")
	if err != nil {
		t.Fatal(err)
	}
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.Proxy = nil
	transport.TLSClientConfig = clientTLS
	defer transport.CloseIdleConnections()
	remote, _ := url.Parse(ingress.URL)
	egressHandler, err := Proxy(remote, "gateway", transport)
	if err != nil {
		t.Fatal(err)
	}
	egress := httptest.NewServer(egressHandler)
	defer egress.Close()
	for _, tc := range []struct {
		path, token string
		status      int
	}{{"/api/metric/attempts", "local-service-fixture", 204}, {"/api/metric/attempts", "wrong-service-token", 401}, {"/api/user", "local-service-fixture", 404}, {"/api/metric/../user", "local-service-fixture", 404}} {
		req, _ := http.NewRequest("GET", egress.URL+tc.path, nil)
		req.Header.Set("Authorization", "Bearer "+tc.token)
		response, err := egress.Client().Do(req)
		if err != nil {
			t.Fatal(err)
		}
		response.Body.Close()
		if response.StatusCode != tc.status {
			t.Fatalf("path or bearer boundary: %s got %d", tc.path, response.StatusCode)
		}
	}
	wrongIdentity, _ := ClientConfig(valid, ca.roots, "spiffe://starhub/other-gateway", "127.0.0.1")
	badTransport := http.DefaultTransport.(*http.Transport).Clone()
	badTransport.TLSClientConfig = wrongIdentity
	defer badTransport.CloseIdleConnections()
	response, err := (&http.Client{Transport: badTransport, Timeout: 3 * time.Second}).Get(ingress.URL + "/api/metric/attempts")
	if response != nil {
		response.Body.Close()
	}
	if err == nil {
		t.Fatal("egress accepted a trusted certificate for the wrong server identity")
	}
	before := forwarded.Load()
	plain, err := http.Get("http://" + ingress.Listener.Addr().String() + "/api/metric/attempts")
	if err == nil {
		plain.Body.Close()
		if plain.StatusCode < 400 {
			t.Fatal("plaintext was accepted")
		}
	}
	if forwarded.Load() != before {
		t.Fatal("plaintext bypassed mTLS")
	}
}

func TestServiceProxyRejectsUnsafeEndpointsAndRedirects(t *testing.T) {
	for _, tc := range []struct{ direction, listen, upstream string }{
		{"ingress", "127.0.0.1:8443", "http://127.0.0.1:3000"},
		{"egress", "127.0.0.1:8081", "https://resource.example:8443"},
	} {
		target, _ := url.Parse(tc.upstream)
		if err := ValidateEndpoint(tc.direction, tc.listen, target); err != nil {
			t.Fatal(err)
		}
	}
	for _, tc := range []struct{ direction, listen, upstream string }{
		{"egress", "0.0.0.0:8081", "https://resource.example"},
		{"egress", "127.0.0.1:8081", "http://resource.example"},
		{"ingress", "127.0.0.1:8443", "http://resource.example"},
		{"ingress", "127.0.0.1:8443", "http://127.0.0.1/private"},
		{"ingress", "127.0.0.1:8443", "http://user:password@127.0.0.1"},
	} {
		target, _ := url.Parse(tc.upstream)
		if ValidateEndpoint(tc.direction, tc.listen, target) == nil {
			t.Fatal("unsafe endpoint accepted")
		}
	}
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Location", "https://unrelated.example")
		w.WriteHeader(307)
	}))
	defer backend.Close()
	target, _ := url.Parse(backend.URL)
	handler, _ := Proxy(target, "resource", nil)
	request := httptest.NewRequest("POST", "http://proxy/internal/routing/attempt-metadata", strings.NewReader("{}"))
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != 502 {
		t.Fatal("service redirect was relayed")
	}
}

func TestServiceEgressRejectsInvalidServerCertificates(t *testing.T) {
	ca, untrusted := authority(t), authority(t)
	clientCertificate := ca.issue(t, "spiffe://starhub/resource", x509.ExtKeyUsageClientAuth, time.Now().Add(time.Hour))
	for _, tc := range []struct {
		name        string
		certificate tls.Certificate
		serverName  string
	}{
		{"expired", ca.issue(t, "spiffe://starhub/gateway/node-a", x509.ExtKeyUsageServerAuth, time.Now().Add(-time.Hour)), "127.0.0.1"},
		{"untrusted", untrusted.issue(t, "spiffe://starhub/gateway/node-a", x509.ExtKeyUsageServerAuth, time.Now().Add(time.Hour)), "127.0.0.1"},
		{"wrong DNS", ca.issue(t, "spiffe://starhub/gateway/node-a", x509.ExtKeyUsageServerAuth, time.Now().Add(time.Hour)), "wrong.example"},
		{"wrong identity", ca.issue(t, "spiffe://starhub/other", x509.ExtKeyUsageServerAuth, time.Now().Add(time.Hour)), "127.0.0.1"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var reached atomic.Bool
			server := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { reached.Store(true); w.WriteHeader(204) }))
			server.TLS = &tls.Config{MinVersion: tls.VersionTLS13, Certificates: []tls.Certificate{tc.certificate}}
			server.Config.ErrorLog = log.New(io.Discard, "", 0)
			server.StartTLS()
			defer server.Close()
			config, err := ClientConfig(clientCertificate, ca.roots, "spiffe://starhub/gateway/node-a", tc.serverName)
			if err != nil {
				t.Fatal(err)
			}
			transport := http.DefaultTransport.(*http.Transport).Clone()
			transport.Proxy = nil
			transport.TLSClientConfig = config
			defer transport.CloseIdleConnections()
			response, err := (&http.Client{Transport: transport, Timeout: 3 * time.Second}).Get(server.URL)
			if response != nil {
				response.Body.Close()
			}
			if err == nil || reached.Load() {
				t.Fatal("unverified upstream received a service request")
			}
		})
	}
}
