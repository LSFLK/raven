# Raven

**_A lightweight Mail Delivery Agent written in Go, providing IMAP, LMTP, and SASL support with SQLite-based storage._**

[![License](https://img.shields.io/badge/License-Apache%202.0-blue.svg)](https://www.apache.org/licenses/LICENSE-2.0)
![CI](https://img.shields.io/github/actions/workflow/status/LSFLK/raven/release.yaml)
![Security Scan](https://img.shields.io/github/actions/workflow/status/LSFLK/raven/linters.yaml?label=security)
![Last Commit](https://img.shields.io/github/last-commit/LSFLK/raven)

Raven was built to overcome the limitations of traditional Mail Delivery Agents with outdated and complex architectures. 
Instead of relying on legacy file-based storage, Raven adopts a per-user SQLite database model and separates attachments 
into a dedicated storage system. This design significantly improves performance, scalability, backup reliability, and 
long-term maintainability—making Raven a modern, efficient solution for email delivery and management. With more modern 
email features on the way, stay tuned for updates!!!

## Architecture

Raven consists of three services:

### IMAP Server (Ports 143, 993)
- Email client access to mailboxes
- Standard IMAP commands: SELECT, FETCH, SEARCH, STORE, COPY, APPEND, EXPUNGE
- Mailbox operations: CREATE, DELETE, RENAME, LIST, SUBSCRIBE
- TLS/SSL support (STARTTLS and IMAPS)

### LMTP Delivery Service (Port 24)
- Receives incoming mail from MTAs (e.g., Postfix)
- Parses and stores messages in SQLite database
- Routes messages to user mailboxes

### SASL Authentication Service (Unix socket)
- Handles authentication via external auth server
- Integrates with MTAs for SMTP authentication

All services use a **multi-database SQLite architecture** for efficient data isolation and scalability.

## Quick Start

### Option 1: Pull from GitHub Container Registry (Recommended)

```bash
docker pull ghcr.io/lsflk/raven:latest
docker run -d --rm \
  --name raven \
  -p 143:143 -p 993:993 -p 24:24 \
  -v $(pwd)/config:/etc/raven \
  -v $(pwd)/data:/app/data \
  -v $(pwd)/certs:/certs \
  -v $(pwd)/delivery.yaml:/app/delivery.yaml \
  ghcr.io/lsflk/raven:latest
```

### Option 2: Build from Source

1. Clone the repository:
```bash
git clone https://github.com/LSFLK/raven.git
cd raven
```

2. Build and run:
```bash
docker build -t raven .
docker run -d --rm \
  --name raven \
  -p 143:143 -p 993:993 -p 24:24 \
  -v $(pwd)/config:/etc/raven \
  -v $(pwd)/data:/app/data \
  -v $(pwd)/certs:/certs \
  -v $(pwd)/delivery.yaml:/app/delivery.yaml \
  raven
```

The server will start and listen on:
- **Port 143** - IMAP
- **Port 993** - IMAPS
- **Port 24**  - LMTP

Connect using any IMAP client to start managing your emails.

---

## 📂 Required Volume Mounts

| Volume | Path | Description |
|--------|------|-------------|
| **Configuration** | `-v $(pwd)/config:/etc/raven` | Configuration directory containing `raven.yaml` |
| **Data** | `-v $(pwd)/data:/app/data` | Data directory for SQLite databases (`shared.db`, `user_db_*.db`, `role_db_*.db`) and mail storage |
| **Certificates** | `-v $(pwd)/certs:/certs` | TLS/SSL certificates directory containing `fullchain.pem` and `privkey.pem` for IMAPS and STARTTLS |
| **Delivery** | `-v $(pwd)/delivery.yaml:/app/delivery.yaml` | Delivery service configuration file |

---

## 🔐 Certificate Requirements

Your `/certs` directory must contain:
- `fullchain.pem` - Full certificate chain
- `privkey.pem` - Private key

These certificates are required for secure connections on port 993 and STARTTLS functionality.

## ⚙️ Configuration File

Raven requires a configuration file named `raven.yaml` located in `/etc/raven` inside the Docker container.

### Example
```yaml
domain: <domain name>
auth_server_url: <auth url>
```

## Fields

| Key | Description |
|-----|-------------|
| `domain` | The mail domain used in the mail system. |
| `auth_server_url` | The authentication API endpoint used to validate user credentials. |

## Delivery Service (`delivery.yaml`)

The delivery service requires a separate configuration file named `delivery.yaml`.
You can see the [example delivery.yaml](../config/delivery.yaml) for reference.

