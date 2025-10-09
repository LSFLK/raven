# 📬 Raven (Silver Go IMAP Server)

A lightweight and efficient IMAP server implementation in Go, designed for Silver Mail with support for core IMAP functionalities.

---

## 🚀 Quick Start

### Option 1: Pull from GitHub Container Registry (Recommended)

```bash
docker pull ghcr.io/lsflk/raven:latest
docker run -d --rm \
  --name raven \
  -p 143:143 -p 993:993 \
  -v $(pwd)/config:/etc/raven \
  -v $(pwd)/data:/app/data \
  -v $(pwd)/certs:/certs \
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
  raven
```

The server will start and listen on:
- **Port 143** - IMAP
- **Port 993** - IMAPS

Connect using any IMAP client to start managing your emails.

---

## 📂 Required Volume Mounts

| Volume | Path | Description |
|--------|------|-------------|
| **Configuration** | `-v $(pwd)/config:/etc/raven` | Configuration directory containing `raven.yaml` |
| **Data** | `-v $(pwd)/data:/app/data` | Data directory for SQLite database (`mail.db`) and mail storage |
| **Certificates** | `-v $(pwd)/certs:/certs` | TLS/SSL certificates directory containing `fullchain.pem` and `privkey.pem` for IMAPS and STARTTLS |

---

## 🔐 Certificate Requirements

Your `/certs` directory must contain:
- `fullchain.pem` - Full certificate chain
- `privkey.pem` - Private key

These certificates are required for secure connections on port 993 and STARTTLS functionality.
