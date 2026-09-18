package tvspoof

import (
	"context"
	"net"
	"time"

	tls "github.com/refraction-networking/utls"
)

// customUTLSDialer creates a raw TCP connection and wraps it with uTLS
// using a Chrome TLS fingerprint. This makes the ClientHello look identical
// to a real Chrome browser, bypassing TLS fingerprint-based WAFs.
//
// This function is used as the NetDialTLSContext in gorilla/websocket's Dialer,
// which tells gorilla that TLS is already handled and it should NOT perform
// its own TLS handshake on top (which would cause double-wrapping).
func customUTLSDialer(ctx context.Context, network, addr string) (net.Conn, error) {
	// 1. Establish raw TCP connection
	rawConn, err := net.DialTimeout(network, addr, 10*time.Second)
	if err != nil {
		return nil, err
	}

	// 2. Wrap with uTLS using Chrome Auto fingerprint
	host, _, _ := net.SplitHostPort(addr)
	config := &tls.Config{ServerName: host}

	// HelloChrome_Auto automatically follows the latest Chrome version
	tlsConn := tls.UClient(rawConn, config, tls.HelloChrome_Auto)

	// 3. Perform TLS handshake with context support
	if err := tlsConn.HandshakeContext(ctx); err != nil {
		rawConn.Close()
		return nil, err
	}

	return tlsConn, nil
}

// customUTLSDialerHTTP is the HTTP variant of the websocket dialer. The
// explicit HTTP/1.1 ALPN avoids an HTTP/2 upgrade that net/http cannot observe
// when TLS is supplied through DialTLSContext.
func customUTLSDialerHTTP(ctx context.Context, network, addr string) (net.Conn, error) {
	rawConn, err := net.DialTimeout(network, addr, 10*time.Second)
	if err != nil {
		return nil, err
	}
	host, _, _ := net.SplitHostPort(addr)
	config := &tls.Config{ServerName: host, NextProtos: []string{"http/1.1"}}
	tlsConn := tls.UClient(rawConn, config, tls.HelloChrome_Auto)
	if err := tlsConn.BuildHandshakeState(); err != nil {
		rawConn.Close()
		return nil, err
	}
	for _, extension := range tlsConn.Extensions {
		if alpn, ok := extension.(*tls.ALPNExtension); ok {
			alpn.AlpnProtocols = []string{"http/1.1"}
		}
	}
	if err := tlsConn.HandshakeContext(ctx); err != nil {
		rawConn.Close()
		return nil, err
	}
	return tlsConn, nil
}
