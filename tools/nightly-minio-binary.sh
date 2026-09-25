#!/usr/bin/env bash
# tools/nightly-minio-binary.sh — provide the MinIO server binary one leg of
# the nightly integration suite runs against, without Docker.
#
# Usage: tools/nightly-minio-binary.sh floor|latest
#
#   floor   upstream MinIO at FLOOR_VERSION (the README's supported floor),
#           compiled from source with `go install`. MinIO no longer publishes
#           binaries or pullable images for it; the module proxy still serves
#           the source, and sum.golang.org verifies it.
#   latest  the newest release of pgsty/silo (formerly pgsty/minio), the
#           community fork that still cuts releases of the same codebase.
#           The linux_amd64 tarball is checked against the release's
#           published sha256 checksums before use.
#
# Prints the binary's path and `--version`. Under Actions (GITHUB_ENV set) it
# also exports HARBORMASTER_MINIO_BINARY for later steps; locally, export it
# yourself from the printed path.
set -euo pipefail

FLOOR_VERSION=RELEASE.2025-09-07T16-13-09Z
FORK_REPO=pgsty/silo

leg="${1:-}"
dir="$(mktemp -d "${TMPDIR:-/tmp}/harbormaster-minio.XXXXXX")"

case "$leg" in
    floor)
        CGO_ENABLED=0 GOBIN="$dir" go install -trimpath "github.com/minio/minio@${FLOOR_VERSION}"
        bin="$dir/minio"
        ;;
    latest)
        rel="$(curl -fsSL "https://api.github.com/repos/${FORK_REPO}/releases/latest")"
        tgz_name="$(jq -r '.assets[].name | select(endswith("_linux_amd64.tar.gz"))' <<<"$rel")"
        sums_name="$(jq -r '.assets[].name | select(endswith("_checksums.txt") and (contains("packages") | not))' <<<"$rel")"
        if [ -z "$tgz_name" ] || [ -z "$sums_name" ]; then
            echo "nightly-minio-binary: release $(jq -r .tag_name <<<"$rel") has no linux_amd64 tarball or checksums file" >&2
            exit 1
        fi
        url() { jq -r --arg n "$1" '.assets[] | select(.name == $n) | .browser_download_url' <<<"$rel"; }
        curl -fsSL -o "$dir/$tgz_name" "$(url "$tgz_name")"
        curl -fsSL -o "$dir/checksums.txt" "$(url "$sums_name")"
        # checksums.txt lists every asset (incl. "<tarball>.sbom.json"); check
        # exactly ours. No matching line fails the step.
        sum_line="$(awk -v n="$tgz_name" '$2 == n' "$dir/checksums.txt")"
        if [ -z "$sum_line" ]; then
            echo "nightly-minio-binary: $sums_name has no line for $tgz_name" >&2
            exit 1
        fi
        (cd "$dir" && sha256sum -c - <<<"$sum_line")
        tar -xzf "$dir/$tgz_name" -C "$dir" silo
        bin="$dir/silo"
        ;;
    *)
        echo "usage: $0 floor|latest" >&2
        exit 2
        ;;
esac

echo "HARBORMASTER_MINIO_BINARY=$bin"
"$bin" --version
if [ -n "${GITHUB_ENV:-}" ]; then
    echo "HARBORMASTER_MINIO_BINARY=$bin" >> "$GITHUB_ENV"
fi
