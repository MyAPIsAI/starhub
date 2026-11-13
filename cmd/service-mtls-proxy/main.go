// service-mtls-proxy is an optional, explicitly configured transport sidecar.
package main

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"flag"
	"log"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"starhub/internal/servicetls"
	"strings"
	"syscall"
	"time"
)

func run() error {
	direction := flag.String("direction", "ingress", "ingress or egress")
	role := flag.String("role", "gateway", "gateway or resource backend")
	listen := flag.String("listen", "127.0.0.1:8443", "explicit listener address")
	upstream := flag.String("upstream", "", "fixed upstream base URL")
	certFile := flag.String("cert", "", "local certificate PEM file")
	keyFile := flag.String("key", "", "local private key PEM file")
	caFile := flag.String("ca", "", "trusted peer CA PEM file")
	peerURI := flag.String("peer-uri", "", "exact allowed SPIFFE peer URI")
	flag.Parse()
	if flag.NArg() != 0 {
		return errors.New("unexpected positional proxy arguments")
	}
	target, err := url.Parse(*upstream)
	if err != nil {
		return errors.New("invalid upstream URL")
	}
	if err := servicetls.ValidateEndpoint(*direction, *listen, target); err != nil {
		return err
	}
	certificate, err := tls.LoadX509KeyPair(*certFile, *keyFile)
	if err != nil {
		return err
	}
	raw, err := os.ReadFile(*caFile)
	if err != nil {
		return err
	}
	roots := x509.NewCertPool()
	if !roots.AppendCertsFromPEM(raw) {
		return errors.New("CA file contains no certificates")
	}
	transport := http.DefaultTransport.(*http.Transport).Clone()
	// A sidecar never inherits an unreviewed system HTTP proxy.
	transport.Proxy = nil
	server := &http.Server{Addr: *listen, ReadHeaderTimeout: 10 * time.Second, ReadTimeout: 30 * time.Second, WriteTimeout: 90 * time.Second, IdleTimeout: 60 * time.Second, MaxHeaderBytes: 32 << 10}
	if *direction == "ingress" {
		server.TLSConfig, err = servicetls.ServerConfig(certificate, roots, strings.TrimSpace(*peerURI))
	} else {
		transport.TLSClientConfig, err = servicetls.ClientConfig(certificate, roots, strings.TrimSpace(*peerURI), target.Hostname())
	}
	if err != nil {
		return err
	}
	server.Handler, err = servicetls.Proxy(target, *role, transport)
	if err != nil {
		return err
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	done := make(chan error, 1)
	go func() {
		if *direction == "ingress" {
			done <- server.ListenAndServeTLS("", "")
		} else {
			done <- server.ListenAndServe()
		}
	}()
	select {
	case err := <-done:
		return err
	case <-ctx.Done():
		shutdown, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		return server.Shutdown(shutdown)
	}
}

func main() {
	if err := run(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		log.Fatal(err)
	}
}
