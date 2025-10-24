# 📬 Raven Mail Server

A lightweight mail server implementation in Go with IMAP access and LMTP delivery.

---

## 🏗️ Architecture Overview

Raven consists of two main components:

### 1. **IMAP Server** (Port 143 & 993)
- Allows email clients to access and manage mailboxes
- Supports standard IMAP commands (SELECT, FETCH, SEARCH, STORE, etc.)
- Handles user authentication and TLS encryption

### 2. **Delivery Service** (LMTP over Unix socket or TCP( Port 24))
- Receives incoming emails from mail transfer agents (like Postfix)
- Parses and stores messages in the database
- Routes messages to user mailboxes

Both services share a single **SQLite database** for storing users, mailboxes, and messages.

---

## 🚀 Quick Start

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
  -p 143:143 -p 993:993 \
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
| **Data** | `-v $(pwd)/data:/app/data` | Data directory for SQLite database (`mail.db`) and mail storage |
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
