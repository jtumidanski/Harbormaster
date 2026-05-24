# Running Harbormaster behind a reverse proxy

Harbormaster terminates plain HTTP on its listen address (default `:8080`).
For any deployment exposed beyond a single trusted machine you should put a
reverse proxy in front so you can terminate TLS, host multiple services on
one IP, and apply your own access controls.

This document covers the proxy-specific knobs that matter for Harbormaster,
plus copy-pastable nginx, Caddy, and Traefik snippets.

## Why proxy config matters here

Two endpoints have non-default networking requirements:

1. **`POST /api/v1/buckets/{bucket}/empty` (Server-Sent Events).** The
   empty-bucket workflow streams progress events back to the browser over
   an SSE connection. Most reverse proxies buffer responses by default,
   which batches events into multi-KiB chunks and makes the UI look frozen
   until the run completes. Disable response buffering for this route (or
   globally) and raise the read timeout — emptying a million-object bucket
   can take well over an hour.
2. **`PUT /api/v1/buckets/{bucket}/objects` and related upload routes.**
   The backend enforces `HARBORMASTER_UPLOAD_MAX_BYTES` (default 100 MiB).
   Your proxy must permit a request body at least that large, ideally with
   a few MiB of headroom for multipart overhead. nginx's
   `client_max_body_size` defaults to 1 MiB; Caddy and Traefik default to
   no limit, but if you've globally clamped requests for other reasons
   you'll need to raise the cap here too.

## TLS termination

Always terminate TLS at the proxy in production. Harbormaster has optional
built-in TLS (`HARBORMASTER_TLS_CERT_FILE`, `HARBORMASTER_TLS_KEY_FILE`) for
single-host setups without a proxy; once a proxy is in front, leave those
unset and let the proxy handle certificates (Let's Encrypt via Caddy or
cert-manager, ACME via nginx + certbot, Traefik's built-in ACME, etc.).

When TLS is terminated upstream, set `X-Forwarded-Proto: https` so that
Harbormaster knows the original request was TLS-protected. This is needed
for the `Secure` cookie attribute on session cookies. The snippets below
all set it.

## Base-path / sub-path deployment

If you want to mount Harbormaster at e.g. `https://example.com/storage/`
instead of a dedicated subdomain, set `HARBORMASTER_BASE_PATH=/storage/`
in the container's environment **and** rewrite the proxy location so the
backend sees the path it expects. The SPA reads the base path from its
runtime config so links and asset URLs come out correct.

The simplest setup is a dedicated subdomain (`harbormaster.example.com`).
Avoid sub-paths unless you have a reason — sub-paths require extra care
with cookie scope (`/storage/` vs `/`) and with SSE proxy rules.

## nginx

Full example in [`deploy/docker/nginx.conf.example`](../../deploy/docker/nginx.conf.example).
The critical block is:

```nginx
location ~ ^/api/v1/buckets/.+/empty$ {
    proxy_pass http://harbormaster_upstream;
    proxy_buffering off;
    proxy_read_timeout 1h;
    proxy_http_version 1.1;
    proxy_set_header Host $host;
    proxy_set_header X-Real-IP $remote_addr;
    proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
    proxy_set_header X-Forwarded-Proto $scheme;
}

location / {
    proxy_pass http://harbormaster_upstream;
    proxy_set_header Host $host;
    proxy_set_header X-Real-IP $remote_addr;
    proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
    proxy_set_header X-Forwarded-Proto $scheme;
    proxy_http_version 1.1;
    client_max_body_size 200M;
}
```

For TLS, add a `listen 443 ssl;` server block, point at your certs, and
let port 80 redirect to 443.

## Caddy

Full example in [`deploy/docker/caddy.example.Caddyfile`](../../deploy/docker/caddy.example.Caddyfile).
Caddy's `reverse_proxy` directive understands SSE out of the box when
`flush_interval -1` is set:

```caddy
harbormaster.example.com {
    reverse_proxy harbormaster:8080 {
        flush_interval -1
        transport http {
            response_header_timeout 1h
        }
    }

    request_body {
        max_size 200MB
    }
}
```

Caddy provisions and renews Let's Encrypt certificates automatically for
public hostnames — no extra config is needed for TLS.

## Traefik

Traefik is configured via labels (or its dynamic config file). The
important knobs:

- `traefik.http.services.<name>.loadbalancer.responseforwarding.flushinterval=-1ms`
  disables response buffering so SSE flushes promptly. (Traefik defaults
  to 100ms which is acceptable for most SSE but adds visible jitter.)
- `traefik.http.middlewares.<name>.buffering.maxRequestBodyBytes=209715200`
  on a `buffering` middleware (or simply leave the default unbounded
  body size, which is Traefik's default behaviour).
- Standard `entrypoints.web` + `entrypoints.websecure` + `certresolver`
  setup for TLS.

Example labels on the `harbormaster` service in `docker-compose.yml`:

```yaml
labels:
  - "traefik.enable=true"
  - "traefik.http.routers.harbormaster.rule=Host(`harbormaster.example.com`)"
  - "traefik.http.routers.harbormaster.entrypoints=websecure"
  - "traefik.http.routers.harbormaster.tls.certresolver=letsencrypt"
  - "traefik.http.services.harbormaster.loadbalancer.server.port=8080"
  - "traefik.http.services.harbormaster.loadbalancer.responseforwarding.flushinterval=-1ms"
```

## Verifying SSE works end-to-end

After putting the proxy in front, trigger an empty-bucket run from the UI
on a bucket with a few hundred objects. Open browser devtools → Network →
EventStream for the request to `/api/v1/buckets/<name>/empty` and confirm
that events arrive incrementally (one per ~500-object batch) rather than
in one large dump at the end. If they batch, your proxy is still
buffering — re-check the location-specific rules above.

For Kubernetes ingress equivalents, see
[`deploy/kubernetes/ingress.example.yaml`](../../deploy/kubernetes/ingress.example.yaml)
and [`deploy/kubernetes/README.md`](../../deploy/kubernetes/README.md).
