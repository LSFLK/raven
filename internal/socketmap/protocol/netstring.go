package protocol

import (
	"bufio"
	"fmt"
	"io"
	"log"
	"net"
	"strconv"
	"strings"
)

// ReadNetstring reads a netstring from the reader
// Netstring format: <length>:<data>,
// Example: "5:hello," represents the string "hello"
func ReadNetstring(reader *bufio.Reader) (string, error) {
	// Read length prefix (digits before ':')
	lengthStr, err := reader.ReadString(':')
	if err != nil {
		return "", fmt.Errorf("failed to read length: %w", err)
	}

	// Remove the ':' and parse length
	lengthStr = strings.TrimSuffix(lengthStr, ":")
	length, err := strconv.Atoi(lengthStr)
	if err != nil {
		return "", fmt.Errorf("invalid length: %w", err)
	}

	// Validate length to prevent memory exhaustion attacks
	// Maximum allowed netstring size: 10MB (reasonable limit for email-related data)
	const maxNetstringLength = 10 * 1024 * 1024
	if length < 0 {
		return "", fmt.Errorf("invalid length: negative value %d", length)
	}
	if length > maxNetstringLength {
		return "", fmt.Errorf("invalid length: %d exceeds maximum allowed size of %d bytes", length, maxNetstringLength)
	}

	log.Printf("      Netstring length: %d", length)

	// Read exactly 'length' bytes of data using io.ReadFull
	data := make([]byte, length)
	if _, err := io.ReadFull(reader, data); err != nil {
		return "", fmt.Errorf("failed to read data: %w", err)
	}

	// Read and verify the trailing comma
	comma, err := reader.ReadByte()
	if err != nil {
		return "", fmt.Errorf("failed to read comma: %w", err)
	}
	if comma != ',' {
		return "", fmt.Errorf("expected comma, got %c", comma)
	}

	return string(data), nil
}

// WriteNetstring writes a netstring to the connection
func WriteNetstring(conn net.Conn, data string) error {
	netstr := fmt.Sprintf("%d:%s,", len(data), data)
	_, err := conn.Write([]byte(netstr))
	return err
}
