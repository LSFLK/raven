# 📬 Kestrel (Silver Go IMAP Server)

A lightweight and efficient IMAP server implementation in Go, designed for Silver Mail with support for core IMAP functionalities.

---

## 🚀 Quick Start

### Option 1: Pull from GitHub Container Registry (Recommended)

```bash
docker pull ghcr.io/lsflk/kestrel:latest
docker run -d --rm \
  --name kestrel \
  -p 143:143 -p 993:993 \
  -v $(pwd)/config:/etc/kestrel \
  -v $(pwd)/data:/app/data \
  -v $(pwd)/certs:/certs \
  ghcr.io/lsflk/kestrel:latest
```

### Option 2: Build from Source

1. Clone the repository:
```bash
git clone https://github.com/LSFLK/kestrel.git
cd kestrel
```

2. Build and run:
```bash
docker build -t kestrel .
docker run -d --rm \
  --name kestrel \
  -p 143:143 -p 993:993 \
  -v $(pwd)/config:/etc/kestrel \
  -v $(pwd)/data:/app/data \
  -v $(pwd)/certs:/certs \
  kestrel
```

The server will start and listen on:
- **Port 143** - IMAP
- **Port 993** - IMAPS

Connect using any IMAP client to start managing your emails.

---

## 📂 Required Volume Mounts

| Volume | Path | Description |
|--------|------|-------------|
| **Configuration** | `-v $(pwd)/config:/etc/kestrel` | Configuration directory containing `kestrel.yaml` |
| **Data** | `-v $(pwd)/data:/app/data` | Data directory for SQLite database (`mail.db`) and mail storage |
| **Certificates** | `-v $(pwd)/certs:/certs` | TLS/SSL certificates directory containing `fullchain.pem` and `privkey.pem` for IMAPS and STARTTLS |

---

## 🔐 Certificate Requirements

Your `/certs` directory must contain:
- `fullchain.pem` - Full certificate chain
- `privkey.pem` - Private key

These certificates are required for secure connections on port 993 and STARTTLS functionality.
