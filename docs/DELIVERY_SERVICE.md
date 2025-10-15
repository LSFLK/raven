# Raven Delivery Service (LMTP/LDA)

The Raven Delivery Service is a stateless LMTP (Local Mail Transfer Protocol) server that handles local mail delivery for the Raven mail system. It replaces traditional LDA (Local Delivery Agent) implementations.

---

## Overview

The delivery service accepts messages via LMTP and stores them in the SQLite database, making them immediately available to IMAP clients. It's designed to work seamlessly with Postfix and other MTAs that support LMTP.

### Key Features

- **LMTP Protocol Support**: Full RFC 2033 LMTP implementation
- **Flexible Connectivity**: Supports both UNIX socket and TCP connections
- **Stateless Design**: No session state between deliveries
- **Per-Recipient Response**: LMTP provides individual delivery status for each recipient
- **Quota Management**: Optional quota checking and enforcement
- **Domain Filtering**: Configurable allowed domains for relay control
- **Message Validation**: Comprehensive email parsing and validation
- **SQLite Storage**: Direct integration with Raven's database

---

## Architecture

```
┌─────────────┐
│   Postfix   │
│     MTA     │
└──────┬──────┘
       │ LMTP
       ▼
┌─────────────────────────────────┐
│   Raven Delivery Service        │
│                                  │
│  ┌─────────────────────────┐   │
│  │  LMTP Server            │   │
│  │  - UNIX Socket / TCP    │   │
│  │  - Protocol Handler     │   │
│  └───────────┬─────────────┘   │
│              │                  │
│  ┌───────────▼─────────────┐   │
│  │  Email Parser           │   │
│  │  - Message validation   │   │
│  │  - Header extraction    │   │
│  └───────────┬─────────────┘   │
│              │                  │
│  ┌───────────▼─────────────┐   │
│  │  Storage Layer          │   │
│  │  - User management      │   │
│  │  - Quota checking       │   │
│  │  - Database operations  │   │
│  └───────────┬─────────────┘   │
└──────────────┼─────────────────┘
               │
               ▼
        ┌─────────────┐
        │   SQLite    │
        │  Database   │
        └─────────────┘
```

---

## Directory Structure

```
cmd/delivery/              - Delivery service entry point
internal/delivery/
  ├── config/             - Configuration management
  ├── lmtp/               - LMTP protocol implementation
  │   ├── server.go       - Server setup and connection handling
  │   └── session.go      - LMTP session and command handling
  ├── parser/             - Email parsing and validation
  └── storage/            - Message storage operations
test/delivery/            - Delivery service tests
```

---

## Configuration

### Configuration File (`delivery.yaml`)

```yaml
lmtp:
  unix_socket: "/var/run/raven/lmtp.sock"   # UNIX socket path
  tcp_address: "127.0.0.1:24"                # TCP address (optional)
  max_size: 52428800                         # Max message size (50MB)
  timeout: 300                               # Connection timeout (seconds)
  hostname: "mail.example.com"               # Server hostname
  max_recipients: 100                        # Max recipients per transaction

database:
  path: "data/mails.db"                      # SQLite database path

delivery:
  default_folder: "INBOX"                    # Default delivery folder
  quota_enabled: false                       # Enable quota checking
  quota_limit: 1073741824                    # Quota limit (1GB)
  allowed_domains:                           # Allowed recipient domains
    - "example.com"
    - "mail.example.com"
  reject_unknown_user: false                 # Reject mail for unknown users

logging:
  level: "info"                              # Log level (debug/info/warn/error)
  format: "text"                             # Log format (text/json)
```

### Command-Line Options

```bash
./raven-delivery [options]

Options:
  -config string
        Path to configuration file (default "/etc/raven/delivery.yaml")
  -socket string
        Path to UNIX socket (default "/var/run/raven/lmtp.sock")
  -tcp string
        TCP address to bind (e.g., 127.0.0.1:24)
  -db string
        Path to SQLite database (default "data/mails.db")
```

---

## Integration with Postfix

### Step 1: Configure Postfix Main.cf

Add the following to `/etc/postfix/main.cf`:

```
# Use LMTP for local delivery
mailbox_transport = lmtp:unix:/var/run/raven/lmtp.sock

# Or use TCP if preferred
# mailbox_transport = lmtp:127.0.0.1:24

# Virtual domains
virtual_mailbox_domains = example.com, mail.example.com
virtual_transport = lmtp:unix:/var/run/raven/lmtp.sock
```

### Step 2: Restart Postfix

```bash
sudo systemctl restart postfix
```

### Step 3: Test Delivery

```bash
echo "Test message" | mail -s "Test" user@example.com
```

---

## Docker Deployment

### Single Container (IMAP + Delivery)

The default Dockerfile builds and runs both services:

```bash
# Build the image
docker build -t raven:latest .

# Run the container
docker run -d --rm \
  --name raven \
  -p 143:143 \
  -p 993:993 \
  -p 24:24 \
  -v $(pwd)/config:/etc/raven \
  -v $(pwd)/data:/app/data \
  -v $(pwd)/certs:/certs \
  raven:latest
```

### Docker Compose

```yaml
version: '3.8'

services:
  raven:
    build: .
    ports:
      - "143:143"   # IMAP
      - "993:993"   # IMAPS
      - "24:24"     # LMTP
    volumes:
      - ./config:/etc/raven
      - ./data:/app/data
      - ./certs:/certs
      - /var/run/raven:/var/run/raven
    restart: unless-stopped
```

---

## LMTP Protocol Flow

### Typical LMTP Session

```
S: 220 mail.example.com LMTP Service ready
C: LHLO client.example.com
S: 250-mail.example.com
S: 250-PIPELINING
S: 250-ENHANCEDSTATUSCODES
S: 250-SIZE 52428800
S: 250 8BITMIME
C: MAIL FROM:<sender@example.com>
S: 250 2.1.0 Sender OK
C: RCPT TO:<recipient@example.com>
S: 250 2.1.5 Recipient OK
C: DATA
S: 354 Start mail input; end with <CRLF>.<CRLF>
C: From: sender@example.com
C: To: recipient@example.com
C: Subject: Test
C:
C: Message body
C: .
S: 250 2.0.0 Message accepted for delivery to <recipient@example.com>
C: QUIT
S: 221 Bye
```

### Supported Commands

- **LHLO** - Identify client and list capabilities
- **MAIL FROM** - Specify sender
- **RCPT TO** - Specify recipient(s)
- **DATA** - Send message data
- **RSET** - Reset session state
- **NOOP** - No operation
- **QUIT** - Close connection
- **VRFY** - Verify address (disabled by default)
- **HELP** - Show available commands

---

## Testing

### Run Delivery Service Tests

```bash
# Run all delivery tests
make test-delivery

# Run specific test
go test -v ./test/delivery -run TestParseMessage
```

### Manual Testing with telnet

```bash
# Connect via TCP
telnet localhost 24

# Example session
LHLO test
MAIL FROM:<test@example.com>
RCPT TO:<user@example.com>
DATA
From: test@example.com
To: user@example.com
Subject: Test

Test message
.
QUIT
```

### Testing with swaks

```bash
# Install swaks
sudo apt-get install swaks

# Send test message
swaks --to user@example.com \
      --from sender@example.com \
      --server 127.0.0.1:24 \
      --protocol LMTP
```

---

## Security Considerations

1. **Socket Permissions**: Ensure UNIX socket has appropriate permissions
2. **Network Binding**: Bind TCP to localhost (127.0.0.1) unless remote access needed
3. **Quota Limits**: Enable quota checking to prevent disk space exhaustion
4. **Domain Filtering**: Use `allowed_domains` to prevent open relay
5. **User Validation**: Enable `reject_unknown_user` to reject invalid recipients
6. **Message Size**: Set appropriate `max_size` limits

---

## Monitoring and Logging

### Log Levels

- **debug**: Detailed protocol and processing information
- **info**: Normal operational messages (default)
- **warn**: Warning messages
- **error**: Error conditions

### Log Format

Set `format: "json"` in configuration for structured logging:

```json
{
  "level": "info",
  "time": "2024-01-15T10:30:00Z",
  "message": "Message delivered successfully",
  "recipient": "user@example.com",
  "size": 1234
}
```

### Health Checks

The delivery service exposes health checks via the Docker HEALTHCHECK:

```bash
# Check if LMTP is responding
nc -z localhost 24
```

---

## Troubleshooting

### Common Issues

#### Socket Permission Denied

```bash
# Check socket permissions
ls -l /var/run/raven/lmtp.sock

# Fix permissions
chmod 666 /var/run/raven/lmtp.sock
```

#### Connection Refused

- Verify service is running: `ps aux | grep raven-delivery`
- Check logs for errors
- Ensure correct address/socket path in configuration

#### Messages Not Delivered

- Check Postfix logs: `tail -f /var/log/mail.log`
- Verify database permissions
- Check delivery service logs
- Test with manual LMTP session

#### Quota Exceeded

- Check user quota: Query `mails_<username>` table
- Increase quota limit in configuration
- Clean up old messages

---

## Contributing

See [CONTRIBUTING.md](CONTRIBUTING.md) for development guidelines.

---

## License

See [LICENSE](../LICENSE) for license information.
