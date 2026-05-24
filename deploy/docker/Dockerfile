# syntax=docker/dockerfile:1.7

# ---- Stage 1: frontend build ----
FROM node:20-alpine AS frontend
WORKDIR /src
COPY apps/frontend/package.json apps/frontend/package-lock.json ./
RUN npm ci
COPY apps/frontend/ ./
RUN npm run build

# ---- Stage 2: backend build ----
FROM golang:1.24-alpine AS backend
WORKDIR /src
COPY apps/backend/go.mod apps/backend/go.sum ./
RUN go mod download
COPY apps/backend/ ./
# Embed the SPA into the binary
COPY --from=frontend /src/dist ./internal/server/spa-dist
ENV CGO_ENABLED=0
ARG TARGETOS
ARG TARGETARCH
ENV GOOS=$TARGETOS GOARCH=$TARGETARCH
RUN go build -trimpath -ldflags="-s -w -X main.version=${VERSION:-dev}" -o /out/harbormaster ./cmd/harbormaster

# ---- Stage 3: runtime ----
FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=backend /out/harbormaster /usr/local/bin/harbormaster
USER nonroot:nonroot
EXPOSE 8080
ENTRYPOINT ["/usr/local/bin/harbormaster"]
CMD ["serve"]
