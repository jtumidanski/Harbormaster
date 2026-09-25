# MinIO server image for the nightly integration suite's floor leg.
#
# The floor (README "MinIO floor version") is upstream
# RELEASE.2025-09-07T16-13-09Z. MinIO no longer publishes pullable server
# images: quay.io/minio/minio and docker.io/minio/minio both return
# 401 UNAUTHORIZED to anonymous pulls, for pinned tags and `latest` alike.
# Only the source is still published, so the floor is compiled from its
# release tag. `go install <module>@<tag>` resolves the tag through the Go
# module proxy and verifies it against sum.golang.org.
#
# The testcontainers minio module runs `server /data` as the container
# command, so the entrypoint must be the minio binary.
ARG MINIO_VERSION=RELEASE.2025-09-07T16-13-09Z

FROM golang:1.25.12-alpine3.24 AS build
ARG MINIO_VERSION
RUN CGO_ENABLED=0 GOBIN=/out go install -trimpath -ldflags "-s -w" \
        "github.com/minio/minio@${MINIO_VERSION}"

FROM alpine:3.24
COPY --from=build /out/minio /usr/bin/minio
EXPOSE 9000
ENTRYPOINT ["minio"]
