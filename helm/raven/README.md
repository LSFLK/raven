# Raven Helm Chart

A production-ready Helm chart for deploying [Raven](https://github.com/LSFLK/raven) - an open-source IMAP4 email server with LMTP delivery, SASL authentication, and Postfix socketmap support.

## Overview

This chart deploys Raven as a multi-service mail server in Kubernetes, providing:

- **IMAP** (port 143) and **IMAPS** (port 993) - Email access protocol
- **LMTP** (port 24) - Local Mail Transfer Protocol for message delivery
- **SASL** - Authentication service for Postfix
- **Socketmap** - Virtual mailbox maps for Postfix virtual domain routing
- **Persistent Storage** - SQLite database backed by Kubernetes PVC
- **Configuration Management** - YAML-based config via ConfigMaps
- **TLS Support** - Optional IMAPS encryption

## Quick Start

### Prerequisites

- Kubernetes 1.20+
- Helm 3.0+
- A storage class for PVC (default or custom)

### Installation

1. **Basic Installation** (ClusterIP, development):
   ```bash
   helm install raven oci://ghcr.io/lsflk/charts/raven \
     -f helm/raven/examples/values-dev.yaml
   ```

2. **Production Installation** (LoadBalancer, high availability):
   ```bash
   helm install raven oci://ghcr.io/lsflk/charts/raven \
     -f helm/raven/examples/values-prod.yaml \
     --set raven.domain=mail.example.com
   ```

3. **With Custom Configuration**:
   ```bash
   helm install raven oci://ghcr.io/lsflk/charts/raven \
     --set raven.domain=mail.yourdomain.com \
     --set delivery.deliveryConfig.quotaEnabled=true \
     --set persistence.size=50Gi
   ```

### Verify Installation

```bash
# Check deployment status
kubectl get deployment raven

# View pod logs
kubectl logs -f deployment/raven

# Check service endpoints
kubectl get svc raven

# View configuration
kubectl get cm raven-config -o yaml
```

## Configuration Reference

### Core Settings

| Parameter | Default | Description |
|-----------|---------|-------------|
| `replicaCount` | `1` | Number of Raven replicas |
| `image.registry` | `ghcr.io` | Container registry |
| `image.repository` | `lsflk/raven` | Image repository |
| `image.tag` | Chart AppVersion | Override image tag |
| `image.pullPolicy` | `IfNotPresent` | Image pull policy |

### Service Configuration

| Parameter | Default | Description |
|-----------|---------|-------------|
| `service.type` | `ClusterIP` | Service type (ClusterIP, NodePort, LoadBalancer) |
| `service.ports[*].name` | - | Named port for each service |
| `service.ports[*].port` | - | External port number |
| `service.ports[*].targetPort` | - | Container port number |

**Default Ports:**
- `imap`: 143
- `imaps`: 993
- `lmtp`: 24
- `admin`: 12345

### Persistence

| Parameter | Default | Description |
|-----------|---------|-------------|
| `persistence.enabled` | `true` | Enable persistent storage |
| `persistence.size` | `5Gi` | PVC storage size |
| `persistence.storageClassName` | `` | Storage class name (empty = default) |
| `persistence.mountPath` | `/app/data` | Container mount path |
| `persistence.accessModes` | `["ReadWriteOnce"]` | PVC access modes |

### Raven Configuration

These map to `raven.yaml` config:

| Parameter | Default | Description |
|-----------|---------|-------------|
| `raven.domain` | `example.com` | Mail domain name |
| `raven.authServerUrl` | `http://auth-service:8080` | External auth service URL |
| `raven.saslScope` | `all` | SASL access scope (all, tcp_only, unix_socket_only) |
| `raven.blobStorage.enabled` | `false` | Enable S3 blob storage |
| `raven.blobStorage.endpoint` | - | S3-compatible endpoint URL |
| `raven.blobStorage.bucket` | - | Storage bucket name |

### Delivery Configuration

These map to `delivery.yaml` config:

| Parameter | Default | Description |
|-----------|---------|-------------|
| `delivery.lmtp.tcpAddress` | `0.0.0.0:24` | LMTP listen address |
| `delivery.lmtp.maxSize` | `52428800` | Max message size (bytes, 50MB) |
| `delivery.lmtp.timeout` | `300` | Request timeout (seconds) |
| `delivery.database.path` | `/app/data/databases` | Database folder path |
| `delivery.deliveryConfig.defaultFolder` | `INBOX` | Default mailbox folder |
| `delivery.deliveryConfig.quotaEnabled` | `false` | Enable mailbox quotas |
| `delivery.deliveryConfig.rejectUnknownUser` | `false` | Reject unknown recipients |

### Certificates (TLS/SSL)

| Parameter | Default | Description |
|-----------|---------|-------------|
| `certificates.enabled` | `false` | Enable certificate mounting |
| `certificates.mountPath` | `/certs` | Certificate mount path |
| `certificates.existingSecret` | `` | Use existing Kubernetes secret |
| `certificates.hostPath` | `` | Or use host path mount |

To enable IMAPS (SSL), create a TLS secret first:
```bash
kubectl create secret tls raven-tls-secret \
  --cert=path/to/cert.pem \
  --key=path/to/key.pem
```

Then install with:
```bash
helm install raven oci://ghcr.io/lsflk/charts/raven \
  --set certificates.enabled=true \
  --set certificates.existingSecret=raven-tls-secret
```

### Resource Limits

| Parameter | Default | Description |
|-----------|---------|-------------|
| `resources.requests.cpu` | `500m` | CPU request |
| `resources.requests.memory` | `512Mi` | Memory request |
| `resources.limits.cpu` | `2000m` | CPU limit |
| `resources.limits.memory` | `2Gi` | Memory limit |

For production, increase limits:
```bash
helm install raven oci://ghcr.io/lsflk/charts/raven \
  --set resources.limits.cpu=4000m \
  --set resources.limits.memory=4Gi
```

### Health Probes

| Parameter | Default | Description |
|-----------|---------|-------------|
| `probes.liveness.enabled` | `true` | Enable liveness probe |
| `probes.liveness.initialDelaySeconds` | `30` | Initial delay (seconds) |
| `probes.liveness.periodSeconds` | `10` | Check interval (seconds) |
| `probes.readiness.enabled` | `true` | Enable readiness probe |
| `probes.readiness.initialDelaySeconds` | `10` | Initial delay (seconds) |

### Ingress

| Parameter | Default | Description |
|-----------|---------|-------------|
| `ingress.enabled` | `false` | Enable Ingress |
| `ingress.className` | `nginx` | Ingress class name |
| `ingress.hosts` | `[]` | Host names |
| `ingress.tls` | `[]` | TLS configuration |

**Note:** Standard HTTP Ingress doesn't handle IMAP/LMTP (TCP) ports. For TCP proxying, use:
- **Nginx-ingress TCP services** - Configure TCP service mapping in nginx-ingress values
- **Service with LoadBalancer** - Direct cloud load balancer (recommended)
- **NodePort** - For internal Kubernetes networking

See [examples/values-ingress-enabled.yaml](examples/values-ingress-enabled.yaml) for TCP ingress setup.

## Examples

### Example 1: Development Setup

```bash
helm install raven oci://ghcr.io/lsflk/charts/raven \
  --namespace mail --create-namespace \
  -f helm/raven/examples/values-dev.yaml
```

Port forward to access locally:
```bash
kubectl port-forward svc/raven 143:143 993:993 24:24 12345:12345 -n mail
```

### Example 2: Production with LoadBalancer

```bash
helm install raven oci://ghcr.io/lsflk/charts/raven \
  --namespace mail --create-namespace \
  -f helm/raven/examples/values-prod.yaml \
  --set raven.domain=mail.example.com \
  --set service.type=LoadBalancer \
  --set certificates.enabled=true \
  --set certificates.existingSecret=raven-tls
```

### Example 3: Multi-Region with S3 Blob Storage

```bash
helm install raven oci://ghcr.io/lsflk/charts/raven \
  -f helm/raven/examples/values-prod.yaml \
  --set raven.domain=mail.example.com \
  --set raven.blobStorage.enabled=true \
  --set raven.blobStorage.endpoint=https://s3-us-west-2.amazonaws.com \
  --set raven.blobStorage.bucket=raven-attachments \
  --set raven.blobStorage.accessKey=AKIAIOSFODNN7EXAMPLE \
  --set raven.blobStorage.secretKey='wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY'
```

## Upgrading

List current releases:
```bash
helm list -n mail
```

Upgrade to a new version:
```bash
helm upgrade raven oci://ghcr.io/lsflk/charts/raven \
  --version 0.2.0 \
  -n mail
```

Rollback if needed:
```bash
helm rollback raven -n mail
```

## Troubleshooting

### Pod stuck in Pending

Check for PVC binding issues:
```bash
kubectl describe pvc raven-data-pvc
kubectl get storageclass
```

### Pod crashes or restarts

Check logs:
```bash
kubectl logs -f deployment/raven
kubectl describe pod <pod-name>
```

### Configuration not applied

Update ConfigMap and restart pod:
```bash
kubectl rollout restart deployment/raven
```

### No external connectivity

For LoadBalancer service, check external IP assignment:
```bash
kubectl get svc raven -o wide
```

For ClusterIP, use port forwarding or expose as LoadBalancer:
```bash
kubectl patch svc raven -p '{"spec": {"type": "LoadBalancer"}}'
```

## Uninstalling

```bash
helm uninstall raven -n mail
```

**Note:** This does NOT delete the PVC. To clean up data:
```bash
kubectl delete pvc raven-data-pvc -n mail
```

## Support

For issues or questions:
- [GitHub Issues](https://github.com/LSFLK/raven/issues)
- [Raven Documentation](https://github.com/LSFLK/raven/tree/main/docs)
- [Helm Documentation](https://helm.sh/docs/)
