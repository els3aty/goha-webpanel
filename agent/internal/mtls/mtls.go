// Package mtls provides mTLS (mutual TLS) configuration for the agent server.
//
// SECURITY:
//   - The agent presents its certificate to the Control Plane (server auth)
//   - The agent ALSO requires the Control Plane to present a valid certificate (client auth)
//   - Both certificates must be signed by the internal CA
//   - Any connection without a valid client certificate is rejected at the TLS layer
//   - TLS 1.2 minimum; TLS 1.3 preferred
//   - Only secure cipher suites allowed (no RC4, no DES, no export ciphers)
package mtls

import (
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"os"
)

// Config builds a *tls.Config for the agent's mTLS server.
// This config:
//   - Requires client certificate (Control Plane must authenticate)
//   - Verifies client cert against internal CA
//   - Rejects any connection without a valid, CA-signed client certificate
//
// certFile: agent's own certificate (signed by internal CA)
// keyFile:  agent's private key
// caFile:   internal CA certificate (used to verify Control Plane's cert)
func Config(certFile, keyFile, caFile string) (*tls.Config, error) {
	// Load agent certificate and private key
	cert, err := tls.LoadX509KeyPair(certFile, keyFile)
	if err != nil {
		return nil, fmt.Errorf("failed to load agent certificate: %w", err)
	}

	// Load internal CA certificate pool
	caCert, err := os.ReadFile(caFile)
	if err != nil {
		return nil, fmt.Errorf("failed to read CA certificate: %w", err)
	}

	caPool := x509.NewCertPool()
	if !caPool.AppendCertsFromPEM(caCert) {
		return nil, fmt.Errorf("failed to parse CA certificate — invalid PEM")
	}

	cfg := &tls.Config{
		// Agent's own certificate (presented to Control Plane)
		Certificates: []tls.Certificate{cert},

		// REQUIRE client certificate — no anonymous connections allowed
		ClientAuth: tls.RequireAndVerifyClientCert,

		// Only accept client certs signed by our internal CA
		ClientCAs: caPool,

		// Minimum TLS version: 1.2 (1.3 preferred)
		MinVersion: tls.VersionTLS12,

		// Prefer TLS 1.3 cipher suites (automatic in Go's TLS stack)
		// Explicitly restrict TLS 1.2 cipher suites to secure options only
		CipherSuites: secureCipherSuites(),

		// Disable session resumption tickets to force full handshake
		// (better for mutual auth scenarios)
		SessionTicketsDisabled: true,

		// Prefer server cipher suite order
		PreferServerCipherSuites: true,
	}

	return cfg, nil
}

// ClientConfig builds a *tls.Config for use when the Control Plane
// initiates a connection TO the agent (e.g., for certificate rotation callbacks).
// certFile, keyFile: Control Plane's certificate and key
// caFile: internal CA to verify agent's certificate
func ClientConfig(certFile, keyFile, caFile string) (*tls.Config, error) {
	cert, err := tls.LoadX509KeyPair(certFile, keyFile)
	if err != nil {
		return nil, fmt.Errorf("failed to load client certificate: %w", err)
	}

	caCert, err := os.ReadFile(caFile)
	if err != nil {
		return nil, fmt.Errorf("failed to read CA certificate: %w", err)
	}

	caPool := x509.NewCertPool()
	if !caPool.AppendCertsFromPEM(caCert) {
		return nil, fmt.Errorf("failed to parse CA certificate")
	}

	return &tls.Config{
		Certificates: []tls.Certificate{cert},
		RootCAs:      caPool,
		MinVersion:   tls.VersionTLS12,
		CipherSuites: secureCipherSuites(),
	}, nil
}

// VerifyPeerCertificate extracts the Common Name from the peer's certificate.
// Used by the agent to log which Control Plane instance is connecting.
func VerifyPeerCertificate(state tls.ConnectionState) (commonName string, err error) {
	if len(state.PeerCertificates) == 0 {
		return "", fmt.Errorf("no peer certificate presented")
	}
	return state.PeerCertificates[0].Subject.CommonName, nil
}

// secureCipherSuites returns a list of approved TLS 1.2 cipher suites.
// TLS 1.3 cipher suites are always secure and handled automatically by Go.
// We exclude: RC4, DES, 3DES, export ciphers, NULL, ANON, MD5
func secureCipherSuites() []uint16 {
	return []uint16{
		tls.TLS_ECDHE_RSA_WITH_AES_256_GCM_SHA384,
		tls.TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256,
		tls.TLS_ECDHE_ECDSA_WITH_AES_256_GCM_SHA384,
		tls.TLS_ECDHE_ECDSA_WITH_AES_128_GCM_SHA256,
		tls.TLS_ECDHE_RSA_WITH_CHACHA20_POLY1305_SHA256,
		tls.TLS_ECDHE_ECDSA_WITH_CHACHA20_POLY1305_SHA256,
		// Deliberately excluded:
		// tls.TLS_RSA_WITH_RC4_128_SHA    ← RC4, broken
		// tls.TLS_RSA_WITH_3DES_EDE_CBC_SHA ← 3DES, weak
		// tls.TLS_RSA_WITH_AES_*           ← No forward secrecy (no ECDHE)
	}
}
