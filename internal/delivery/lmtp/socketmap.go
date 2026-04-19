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
type identityResolver func(recipient string) (string, error)

func newIdentityResolver(cfg *config.Config) identityResolver {
	if cfg == nil || !cfg.Socketmap.Enabled {
		return nil
	}

	return newSocketmapIdentityResolver(cfg)
}

func newSocketmapIdentityResolver(cfg *config.Config) identityResolver {
	network := cfg.Socketmap.Network
	address := cfg.Socketmap.Address
	timeout := time.Duration(cfg.Socketmap.TimeoutSeconds) * time.Second

	return func(recipient string) (string, error) {
		conn, err := net.DialTimeout(network, address, timeout)
		if err != nil {
			return "", fmt.Errorf("socketmap dial failed: %w", err)
		}
		defer func() { _ = conn.Close() }()

		_ = conn.SetDeadline(time.Now().Add(timeout))

		if err := protocol.WriteNetstring(conn, fmt.Sprintf("user-exists %s", recipient)); err != nil {
			return "", fmt.Errorf("socketmap write failed: %w", err)
		}

		resp, err := protocol.ReadNetstring(bufio.NewReader(conn))
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
}
