# Delivery Service

The Raven Delivery Service handles incoming mail via LMTP (Local Mail Transfer Protocol), storing messages in the SQLite database for IMAP access.

## Configuration

Create a `delivery.yaml` file with the following settings:

```yaml
lmtp:
  unix_socket: "/var/run/raven/lmtp.sock"   # UNIX socket path
  tcp_address: "127.0.0.1:24"                # TCP address (optional)
  max_size: 52428800                         # Max message size (50MB)
  timeout: 300                               # Connection timeout (seconds)
  hostname: "mail.example.com"               # Server hostname

database:
  path: "data"                               # Database directory path

delivery:
  default_folder: "INBOX"                    # Default delivery folder
  allowed_domains:                           # Allowed recipient domains
    - "example.com"

logging:
  level: "info"                              # Log level (debug/info/warn/error)
  format: "text"                             # Log format (text/json)
```

## Postfix Integration

Add to `/etc/postfix/main.cf`:

```
# Use LMTP for local delivery
mailbox_transport = lmtp:unix:/var/run/raven/lmtp.sock

# Or use TCP
# mailbox_transport = lmtp:127.0.0.1:24

# Virtual domains
virtual_mailbox_domains = example.com
virtual_transport = lmtp:unix:/var/run/raven/lmtp.sock
```

Restart Postfix:
```bash
sudo systemctl restart postfix
```

## Docker Setup

The delivery service runs automatically in the Raven Docker container:

```bash
docker run -d --rm \
  --name raven \
  -p 143:143 -p 993:993 -p 24:24 \
  -v $(pwd)/config:/etc/raven \
  -v $(pwd)/data:/app/data \
  -v $(pwd)/certs:/certs \
  -v $(pwd)/delivery.yaml:/app/delivery.yaml \
  ghcr.io/lsflk/raven:latest
```

See the [example delivery.yaml](../config/delivery.yaml) for reference.
