# syntax=docker/dockerfile:1.7

# ---- Stage 1: frontend build ----
FROM node:20-alpine AS frontend
WORKDIR /src
COPY apps/frontend/package.json apps/frontend/package-lock.json ./
RUN npm ci
COPY apps/frontend/ ./
RUN npm run build

# ---- Stage 2: backend build ----
FROM golang:1.25-alpine AS backend
WORKDIR /src
COPY apps/backend/go.mod apps/backend/go.sum ./
RUN go mod download
COPY apps/backend/ ./
# Embed the SPA into the binary
COPY --from=frontend /src/dist ./internal/server/spa-dist
ENV CGO_ENABLED=0
ARG TARGETOS
ARG TARGETARCH
ARG VERSION=dev
ENV GOOS=$TARGETOS GOARCH=$TARGETARCH
ENV VERSION=$VERSION
RUN go build -trimpath -ldflags="-s -w -X main.version=${VERSION}" -o /out/harbormaster ./cmd/harbormaster

# ---- Stage 3: runtime ----
FROM gcr.io/distroless/static-debian12:nonroot

# OCI image labels (https://github.com/opencontainers/image-spec/blob/main/annotations.md)
ARG VERSION=dev
LABEL org.opencontainers.image.title="Harbormaster" \
      org.opencontainers.image.description="Self-hosted MinIO admin UI for homelab and small-cluster operators." \
      org.opencontainers.image.source="https://github.com/jtumidanski/harbormaster" \
      org.opencontainers.image.url="https://github.com/jtumidanski/harbormaster" \
      org.opencontainers.image.documentation="https://github.com/jtumidanski/harbormaster/tree/main/docs" \
      org.opencontainers.image.vendor="Harbormaster" \
      org.opencontainers.image.licenses="AGPL-3.0-or-later" \
      org.opencontainers.image.version="${VERSION}"

COPY --from=backend /out/harbormaster /usr/local/bin/harbormaster
USER nonroot:nonroot
EXPOSE 8080

# Distroless has no shell or curl; the `version` subcommand is a pure
# process-aliveness probe — it exits 0 without touching the network or DB.
HEALTHCHECK --interval=30s --timeout=3s --start-period=5s --retries=3 \
  CMD ["/usr/local/bin/harbormaster", "version"]

ENTRYPOINT ["/usr/local/bin/harbormaster"]
CMD ["serve"]
