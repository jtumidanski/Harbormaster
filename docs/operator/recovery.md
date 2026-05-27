# Harbormaster — Recovery guide

Two destructive recovery operations are bundled with the `harbormaster`
binary. Both are safety-gated CLI subcommands intentionally not exposed
via the web UI.

| Command                                | When to use                                                      |
| -------------------------------------- | ---------------------------------------------------------------- |
| `harbormaster admin reset-password`    | Local admin password lost (no other admin can rotate it).        |
| `harbormaster admin reset-encryption --confirm` | Encryption key file lost or corrupted; Harbormaster refuses to start. |

Both commands must be run **on the host that has access to the
Harbormaster data directory** (the directory pointed at by
`HARBORMASTER_DATA_DIR`, default `/var/lib/harbormaster`).

---

## `admin reset-password`

Resets the password for an existing local admin user. The username must
already exist; the command does not create new admins.

What it preserves: the admin row, all sessions for other admins (if
any), the MinIO connection, audit history, encryption key.

What it changes: the password hash for the named admin. All sessions
for that admin are invalidated on next request (the next login issues
fresh cookies).

### Docker

```bash
docker compose exec harbormaster \
  /usr/local/bin/harbormaster admin reset-password
```

You'll be prompted twice for the new password (no echo). Minimum 12
characters; the same policy as the setup wizard.

If the username has been changed from the default, pass it explicitly:

```bash
docker compose exec harbormaster \
  /usr/local/bin/harbormaster admin reset-password --username alice
```

### Kubernetes

```bash
kubectl -n harbormaster exec -it \
  deployment/harbormaster -- \
  /usr/local/bin/harbormaster admin reset-password
```

---

## `admin reset-encryption --confirm`

Generates a new encryption key file and forgets every MinIO connection
that was encrypted with the old key. Required when the previous key file
was lost, corrupted, or the host filesystem was restored from a backup
that doesn't include the key.

### Safety gates

1. The `--confirm` flag is mandatory. Without it the command exits with
   `--confirm is mandatory; this is destructive`.
2. The SQLite database is copied to `<db>.pre-reset-<unix-ts>.bak`
   before any change.
3. The old encryption key file is renamed to
   `<key>.pre-reset-<unix-ts>.bak` (kept in case you find the original
   issue was filesystem-level, not key-level).
4. A new key is generated, its SHA-256 fingerprint is stamped into
   `app_settings.encryption_key_fingerprint`, and `setup_completed` is
   cleared so the first-run wizard runs again.

### What's preserved

- The admin user table (you keep your local admin account).
- The audit_events history.
- The bucket_empty_jobs history.

### What's lost

- Every row in `minio_connections` — the MinIO endpoint URL, access
  key, secret key, custom CA, and `tls_skip_verify` flag must be
  re-entered through the setup wizard.

### Docker

```bash
# 1. Stop the container so it doesn't race with the DB rewrite.
docker compose down

# 2. Bring up an interactive shell into the same image, mounting the
#    data volume, to run the reset.
docker compose run --rm -i harbormaster \
  /usr/local/bin/harbormaster admin reset-encryption --confirm

# 3. Bring the service back up. Visit / in your browser and re-run the
#    setup wizard.
docker compose up -d
```

### Kubernetes

```bash
# 1. Scale to 0 so the pod releases the PVC.
kubectl -n harbormaster scale deployment/harbormaster --replicas=0

# 2. Run a one-shot pod that mounts the same PVC and executes the reset.
kubectl -n harbormaster run hm-reset --rm -it \
  --image=ghcr.io/jtumidanski/harbormaster:v1 \
  --restart=Never \
  --overrides='
{
  "spec": {
    "containers": [{
      "name": "hm-reset",
      "image": "ghcr.io/jtumidanski/harbormaster:v1",
      "command": ["/usr/local/bin/harbormaster","admin","reset-encryption","--confirm"],
      "volumeMounts": [{"name":"data","mountPath":"/var/lib/harbormaster"}],
      "stdin": true, "tty": true
    }],
    "volumes": [{"name":"data","persistentVolumeClaim":{"claimName":"harbormaster-data"}}]
  }
}'

# 3. Scale back up. Visit / and re-run the setup wizard.
kubectl -n harbormaster scale deployment/harbormaster --replicas=1
```

---

## After either reset

Verify the operation succeeded:

- After `reset-password`, log in with the new password.
- After `reset-encryption`, the setup wizard appears on first visit;
  re-enter the MinIO endpoint and credentials, then log in with the
  preserved admin password.

If anything looks wrong, the `.pre-reset-<unix-ts>.bak` files in the
data directory still contain the pre-reset DB and (for
`reset-encryption`) the original key file. They are not automatically
cleaned up — remove them once you're confident the reset worked.
