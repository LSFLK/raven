# Socketmap Service

Postfix socketmap service for dynamic virtual mailbox maps backed by Thunder IDP.

## Quick Start

```bash
# Build and run
docker compose up socketmap-server -d

# View logs
docker compose logs -f socketmap-server
```

## Configuration

Environment variables (set in docker-compose.yaml):

```yaml
SOCKETMAP_HOST: "0.0.0.0"
SOCKETMAP_PORT: "9100"
THUNDER_HOST: "thunder-server"
THUNDER_PORT: "8090"
CACHE_TTL_SECONDS: "300"           # 5 minutes
TOKEN_REFRESH_SECONDS: "3300"      # 55 minutes
```

## Postfix Configuration

Add to `/etc/postfix/main.cf`:

```bash
virtual_mailbox_domains = socketmap:inet:socketmap-server:9100:virtual-domains
virtual_mailbox_maps = socketmap:inet:socketmap-server:9100:user-exists
virtual_alias_maps = socketmap:inet:socketmap-server:9100:virtual-aliases
```

## Supported Maps

| Map | Purpose | Response |
|-----|---------|----------|
| `user-exists` | Validate users and group addresses via Thunder IDP | `OK email` or `NOTFOUND` |
| `virtual-domains` | Validate domains via Thunder OUs | `OK 1` or `NOTFOUND` |
| `virtual-aliases` | Resolve aliases | `OK target` or `NOTFOUND` |

Group address format: `<groupname>-group@domainname`.

**Caching:** Positive results cached for 5 minutes. Negative results NOT cached (new users/groups immediately accessible).

## Troubleshooting

```bash
# Check service
docker compose ps socketmap-server
docker compose logs -f socketmap-server

# Test connectivity
docker exec smtp nc -zv socketmap-server 9100

# Rebuild if needed
docker compose build --no-cache socketmap-server
```