package lmtp

import (
	"bufio"
	"fmt"
	"net"
	"strings"
	"time"

	"raven/internal/delivery/config"
	"raven/internal/socketmap/protocol"
)

// identityResolver resolves recipient address to mailbox identity.
type identityResolver interface {
	Resolve(recipient string) (string, error)
	Close() error
}

type identityResolverFunc func(recipient string) (string, error)

func (f identityResolverFunc) Resolve(recipient string) (string, error) {
	return f(recipient)
}

func (f identityResolverFunc) Close() error {
	return nil
}

func newIdentityResolver(cfg *config.Config) identityResolver {
	if cfg == nil || !cfg.Socketmap.Enabled {
		return nil
	}

	return newSocketmapIdentityResolver(cfg)
}

type socketmapIdentityResolver struct {
	network string
	address string
	timeout time.Duration
	conn    net.Conn
	reader  *bufio.Reader
}

func newSocketmapIdentityResolver(cfg *config.Config) identityResolver {
	return &socketmapIdentityResolver{
		network: cfg.Socketmap.Network,
		address: cfg.Socketmap.Address,
		timeout: time.Duration(cfg.Socketmap.TimeoutSeconds) * time.Second,
	}
}

func (r *socketmapIdentityResolver) Resolve(recipient string) (string, error) {
	if err := r.ensureConn(); err != nil {
		return "", err
	}

	identity, err := r.resolveWithConn(recipient)
	if err == nil {
		return identity, nil
	}

	// Retry once on stale/broken connection by reconnecting.
	r.closeConn()
	if retryErr := r.ensureConn(); retryErr != nil {
		return "", retryErr
	}

	return r.resolveWithConn(recipient)
}

func (r *socketmapIdentityResolver) Close() error {
	r.closeConn()
	return nil
}

func (r *socketmapIdentityResolver) ensureConn() error {
	if r.conn != nil {
		return nil
	}

	conn, err := net.DialTimeout(r.network, r.address, r.timeout)
	if err != nil {
		return fmt.Errorf("socketmap dial failed: %w", err)
	}

	r.conn = conn
	r.reader = bufio.NewReader(conn)
	return nil
}

func (r *socketmapIdentityResolver) resolveWithConn(recipient string) (string, error) {
	_ = r.conn.SetDeadline(time.Now().Add(r.timeout))

	if err := protocol.WriteNetstring(r.conn, fmt.Sprintf("user-exists %s", recipient)); err != nil {
		return "", fmt.Errorf("socketmap write failed: %w", err)
	}

	resp, err := protocol.ReadNetstring(r.reader)
	if err != nil {
		return "", fmt.Errorf("socketmap read failed: %w", err)
	}

	if strings.HasPrefix(resp, "OK ") {
		parts := strings.Fields(resp)
		if len(parts) >= 2 {
			return parts[1], nil
		}
		return recipient, nil
	}

	if resp == "NOTFOUND" {
		return recipient, nil
	}

	if strings.HasPrefix(resp, "PERM") || strings.HasPrefix(resp, "TEMP") {
		return "", fmt.Errorf("socketmap returned %q", resp)
	}

	return recipient, nil
}

func (r *socketmapIdentityResolver) closeConn() {
	if r.conn != nil {
		_ = r.conn.Close()
	}
	r.conn = nil
	r.reader = nil
}
