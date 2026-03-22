# Raven Kubernetes Deployment Guide

This guide covers deploying Raven in Kubernetes using the official Helm chart.

## Prerequisites

- Kubernetes cluster 1.20+
- Helm 3.0+
- `kubectl` configured to access your cluster
- A storage class that supports `ReadWriteOnce` volumes

## Quick Start

### 1. Add the Raven Helm Chart Repository

The Raven Helm chart is published to GHCR (GitHub Container Registry) as an OCI artifact.

```bash
# No repository add needed for OCI charts - reference directly with oci://
helm search repo raven 2>/dev/null || echo "OCI charts are referenced directly, not through repos"
```

### 2. Install Raven

#### Minimal Installation (Development)

For a basic development setup:

```bash
helm install raven oci://ghcr.io/lsflk/charts/raven \
  --version 0.1.0 \
  --namespace mail \
  --create-namespace
```

#### Production Installation

For a production-ready deployment with high availability:

```bash
helm install raven oci://ghcr.io/lsflk/charts/raven \
  --version 0.1.0 \
  --namespace mail \
  --create-namespace \
  -f helm/raven/examples/values-prod.yaml \
  --set raven.domain=mail.example.com
```

#### With Custom Values

Create a `custom-values.yaml`:

```yaml
raven:
  domain: mail.yourdomain.com
  authServerUrl: http://auth-service:8080

delivery:
  deliveryConfig:
    quotaEnabled: true
    rejectUnknownUser: true

persistence:
  size: 50Gi
  storageClassName: fast-ssd

service:
  type: LoadBalancer
```

Install with custom values:

```bash
helm install raven oci://ghcr.io/lsflk/charts/raven \
  --version 0.1.0 \
  --namespace mail \
  --create-namespace \
  -f custom-values.yaml
```

### 3. Verify Installation

Check deployment status:

```bash
# Check pods
kubectl get pods -n mail -l app.kubernetes.io/name=raven

# Check services
kubectl get svc -n mail -l app.kubernetes.io/name=raven

# View logs
kubectl logs -n mail deployment/raven

# Check configuration
kubectl get configmap -n mail raven-config -o yaml
```

### 4. Access Raven Services

#### For ClusterIP Service (Internal Access)

Use port forwarding:

```bash
kubectl port-forward -n mail svc/raven 143:143 993:993 24:24 12345:12345
```

Then connect to `localhost` on the forwarded ports.

#### For LoadBalancer Service (External Access)

Get the external load balancer IP:

```bash
kubectl get svc -n mail raven -o jsonpath='{.status.loadBalancer.ingress[0].ip}'
```

Connect to the returned IP address on ports 143, 993, 24, or 12345.

#### For NodePort Service

Get the node port:

```bash
kubectl get svc -n mail raven -o jsonpath='{.spec.ports[0].nodePort}'
```

Connect to any cluster node IP with the returned port.

## Configuration

### Common Configuration Tasks

#### Enable TLS/SSL for IMAPS

Create a TLS secret:

```bash
kubectl create secret tls raven-tls \
  -n mail \
  --cert=/path/to/cert.pem \
  --key=/path/to/key.pem
```

Update Helm values:

```bash
helm upgrade raven oci://ghcr.io/lsflk/charts/raven \
  --namespace mail \
  --reuse-values \
  --set certificates.enabled=true \
  --set certificates.existingSecret=raven-tls
```

#### Enable Blob Storage for Attachments

For S3-compatible blob storage (e.g., AWS S3, MinIO, SeaweedFS):

```bash
helm upgrade raven oci://ghcr.io/lsflk/charts/raven \
  --namespace mail \
  --reuse-values \
  --set raven.blobStorage.enabled=true \
  --set raven.blobStorage.endpoint=https://s3.amazonaws.com \
  --set raven.blobStorage.bucket=raven-attachments \
  --set raven.blobStorage.accessKey=YOUR_ACCESS_KEY \
  --set raven.blobStorage.secretKey=YOUR_SECRET_KEY
```

#### Increase Storage Size

```bash
# Edit PVC manually (requires scale down)
kubectl patch pvc -n mail raven-data-pvc -p \
  '{"spec":{"resources":{"requests":{"storage":"100Gi"}}}}'

# Or recreate with new values
helm upgrade raven oci://ghcr.io/lsflk/charts/raven \
  --namespace mail \
  --reuse-values \
  --set persistence.size=100Gi
```

**Note:** PECs don't support shrinking, and expanding requires the storage class to support it.

#### Enable High Availability

For multi-replica deployment (requires distributed storage):

```bash
helm upgrade raven oci://ghcr.io/lsflk/charts/raven \
  --namespace mail \
  --reuse-values \
  --set replicaCount=3 \
  --set persistence.accessModes[0]=ReadWriteMany \
  --set affinity.podAntiAffinity.preferredDuringSchedulingIgnoredDuringExecution[0].weight=100
```

## Monitoring

### View Logs

Real-time logs:
```bash
kubectl logs -f -n mail deployment/raven
```

Last 100 lines:
```bash
kubectl logs -n mail deployment/raven --tail=100
```

### Check Pod Status

```bash
kubectl describe pod -n mail -l app.kubernetes.io/name=raven
```

### Monitor Resource Usage

```bash
kubectl top pod -n mail -l app.kubernetes.io/name=raven
```

### View Events

```bash
kubectl get events -n mail --sort-by='.lastTimestamp'
```

## Troubleshooting

### Pod won't start (ImagePullBackOff)

Check image availability:
```bash
kubectl describe pod -n mail -l app.kubernetes.io/name=raven
```

Verify image exists:
```bash
docker pull ghcr.io/lsflk/raven:0.5.0
```

### Pod crashes or keeps restarting

Check logs:
```bash
kubectl logs -n mail deployment/raven --previous  # Previous container logs
```

Check probe configuration:
```bash
kubectl get deployment -n mail raven -o yaml | grep -A 15 "livenessProbe"
```

### PVC not binding

Check storage:
```bash
kubectl get pvc -n mail
kubectl describe pvc -n mail raven-data-pvc
kubectl get storageclass
```

### Configuration not applied after upgrade

Restart the deployment:
```bash
kubectl rollout restart deployment/raven -n mail
kubectl rollout status deployment/raven -n mail
```

### Services not accessible

Check service status:
```bash
kubectl get svc -n mail raven
kubectl describe svc -n mail raven
```

Check network policies:
```bash
kubectl get networkpolicies -n mail
```

## Upgrading Raven

### Check available versions

```bash
helm search repo raven --devel  # Won't show for OCI charts
# Or check GitHub releases directly
```

### Perform upgrade

```bash
helm upgrade raven oci://ghcr.io/lsflk/charts/raven \
  --namespace mail \
  --version VERSION
```

Monitor the upgrade:
```bash
kubectl rollout status deployment/raven -n mail
```

### Rollback if needed

```bash
helm rollback raven -n mail
```

## Uninstalling

Remove the Helm release:
```bash
helm uninstall raven -n mail
```

**Important:** This does NOT delete persistent data. To remove the database:

```bash
kubectl delete pvc -n mail raven-data-pvc
```

To remove the entire namespace:
```bash
kubectl delete namespace mail
```

## Advanced Configuration

### Using a Different Storage Class

Check available storage classes:
```bash
kubectl get storageclass
```

Use a specific storage class:
```bash
helm install raven oci://ghcr.io/lsflk/charts/raven \
  --namespace mail \
  --create-namespace \
  --set persistence.storageClassName=fast-ssd
```

### Custom Resource Allocation

For heavy email traffic:
```bash
helm upgrade raven oci://ghcr.io/lsflk/charts/raven \
  --namespace mail \
  --reuse-values \
  --set resources.requests.cpu=2000m \
  --set resources.requests.memory=2Gi \
  --set resources.limits.cpu=4000m \
  --set resources.limits.memory=4Gi
```

### Node Affinity

Deploy only on specific nodes:
```yaml
nodeSelector:
  workload: mail-server

tolerations:
  - key: "mail-server"
    operator: "Equal"
    value: "true"
    effect: "NoSchedule"
```

Apply with:
```bash
helm upgrade raven oci://ghcr.io/lsflk/charts/raven \
  --namespace mail \
  --reuse-values \
  -f custom-affinity.yaml
```

## Helm Chart Reference

For detailed configuration options, see the [Helm Chart README](../helm/raven/README.md).

Example values:
- [Development](../helm/raven/examples/values-dev.yaml)
- [Production](../helm/raven/examples/values-prod.yaml)
- [Ingress Enabled](../helm/raven/examples/values-ingress-enabled.yaml)

## Support & Feedback

- [Report Issues](https://github.com/LSFLK/raven/issues)
- [GitHub Discussions](https://github.com/LSFLK/raven/discussions)
- [Documentation](https://github.com/LSFLK/raven/tree/main/docs)
