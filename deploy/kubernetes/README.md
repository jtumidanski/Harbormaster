# Harbormaster — Kubernetes manifests

Raw manifests for deploying Harbormaster on Kubernetes. A Helm chart is
intentionally out of scope for v1 (see `docs/tasks/task-001-harbormaster-mvp-v1/risks.md`
R7); these manifests are heavily commented so operators can tune them
without one.

## Files

| File                     | Purpose                                                  |
| ------------------------ | -------------------------------------------------------- |
| `deployment.yaml`        | Single-replica Deployment, distroless image, nonroot.    |
| `service.yaml`           | ClusterIP Service exposing port 8080 inside the cluster. |
| `pvc.yaml`               | PersistentVolumeClaim for `/var/lib/harbormaster`.       |
| `ingress.example.yaml`   | TLS-terminating Ingress with SSE-aware annotations.      |
| `secret.example.yaml`    | Optional Secret seeding `HARBORMASTER_SESSION_SECRET`.   |

## Apply

A standard install creates a namespace and applies the four core
manifests; the ingress and secret are optional starter templates.

```bash
kubectl create namespace harbormaster
kubectl -n harbormaster apply -f pvc.yaml
kubectl -n harbormaster apply -f deployment.yaml
kubectl -n harbormaster apply -f service.yaml

# Optional: ingress (edit hostname + TLS first).
kubectl -n harbormaster apply -f ingress.example.yaml

# Optional: session secret (generate + edit first; do NOT commit it).
kubectl -n harbormaster apply -f secret.example.yaml
```

Verify:

```bash
kubectl -n harbormaster get pods,svc,pvc,ingress
kubectl -n harbormaster logs deploy/harbormaster -f
```

Once the pod is ready, open the Ingress hostname in a browser and walk
through the first-run setup wizard. Wizard answers (MinIO endpoint, root
credentials, operator account) are written to the encrypted SQLite DB on
the PVC.

## Why `replicas: 1` + `strategy: Recreate`?

Harbormaster v1 keeps several pieces of state in-process:

- The **login rate limiter** is a per-pod token bucket. Running multiple
  replicas would split the bucket per pod, defeating the limit.
- The **empty-bucket worker** holds a goroutine per active job and
  streams progress to the requesting browser. A second replica would
  double-issue MinIO delete batches.
- The **audit-retention sweeper** is a singleton background job. Two
  replicas would do duplicate retention work and potentially race.
- The **SQLite database** on the PVC uses `ReadWriteOnce` and cannot be
  attached to two nodes simultaneously.

For v1, this is by design — Harbormaster targets homelabs and small
clusters where a 5-10s gap during pod restart is acceptable. The
`Recreate` strategy ensures the old pod terminates fully (releasing the
PVC) before the new one starts. See `risks.md` R6 for the upgrade path
to leadership election if HA becomes a v2 goal.

## PVC sizing

Default: **1Gi**. This holds:

- the SQLite database (operators, audit events, share-links, lifecycle
  rules, service-account metadata),
- the locally-generated encryption key file,
- transient WAL/journal files.

In practice a busy small cluster generates roughly **5-20 MiB of audit
events per month** at default verbosity. 1Gi covers ~5 years of audit
history before the retention sweeper has anything to do. Bump to **5Gi**
if you keep long retention or expect heavy bucket-management activity.

To change later: edit `pvc.yaml`, `kubectl apply`, then have your CSI
driver resize the underlying volume (`AllowVolumeExpansion: true` on the
StorageClass). Most modern CSI drivers support online expansion.

## Ingress integration

`ingress.example.yaml` is written for **ingress-nginx** because that's
the most common controller in homelab clusters. The non-default
annotations matter:

| Annotation                  | Why                                            |
| --------------------------- | ---------------------------------------------- |
| `proxy-buffering: "off"`    | Required for the empty-bucket SSE stream.      |
| `proxy-read-timeout: 3600`  | Empty-bucket runs on million-object buckets.   |
| `proxy-send-timeout: 3600`  | Same — symmetrical for proxied request body.   |
| `proxy-body-size: 200m`     | Match upload cap + headroom.                   |

If you use Traefik, Caddy, Contour, or HAProxy, translate the
annotations to the controller's equivalent (see
`docs/operator/reverse-proxy.md` for nginx, Caddy, and Traefik snippets).

cert-manager is the easiest TLS option: deploy a `ClusterIssuer`
(letsencrypt-prod or your private CA), then add
`cert-manager.io/cluster-issuer: "letsencrypt-prod"` to the Ingress
annotations.

## Admin commands (exec one-liners)

When you need to perform out-of-band administration, exec into the pod.
Because the pod has `readOnlyRootFilesystem: true`, all writable state
lives on the PVC and is preserved across the exec session.

Reset an operator's password (e.g., after lockout):

```bash
kubectl -n harbormaster exec -it deploy/harbormaster -- \
  /usr/local/bin/harbormaster admin reset-password --username <operator>
```

Re-key the encryption key (emergency rotation — destroys ALL stored
secrets including MinIO credentials; the wizard must be re-run):

```bash
kubectl -n harbormaster exec -it deploy/harbormaster -- \
  /usr/local/bin/harbormaster admin reset-encryption --confirm
```

Print the running version:

```bash
kubectl -n harbormaster exec deploy/harbormaster -- \
  /usr/local/bin/harbormaster version
```

## Upgrades

1. Bump the `image:` tag in `deployment.yaml` (or use
   `kubectl set image deployment/harbormaster harbormaster=ghcr.io/jtumidanski/harbormaster:vX`).
2. `Recreate` strategy will terminate the old pod, then start the new one
   on the same PVC. Migrations run automatically on first boot.
3. Watch logs with `kubectl logs -f` and confirm the readiness probe
   recovers within ~10s. Roll back by re-applying the old tag.

## Backup

The PVC contents are the only state you need to back up. A typical
approach is a CronJob that uses your CSI driver's snapshot CRD
(`VolumeSnapshot`) or that mounts the PVC `ReadOnly` to a sidecar and
rsyncs to off-cluster storage. For a guaranteed consistent snapshot,
briefly scale the Deployment to zero before snapshotting:

```bash
kubectl -n harbormaster scale deploy/harbormaster --replicas=0
# ... take VolumeSnapshot or copy out the PVC contents ...
kubectl -n harbormaster scale deploy/harbormaster --replicas=1
```
