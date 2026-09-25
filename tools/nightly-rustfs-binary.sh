#!/usr/bin/env bash
# tools/nightly-rustfs-binary.sh — provide the RustFS server binary for the
# nightly integration suite's rustfs leg, without Docker.
#
# Downloads the pinned RustFS release's linux-x86_64-musl asset (a static
# binary, so it runs on any glibc/musl runner) from the GitHub release,
# verifies it against the release's published SHA256SUMS asset when one is
# present, and unzips the `rustfs` binary.
#
# Usage: tools/nightly-rustfs-binary.sh
#
# Prints the binary's path and `--version`. Under Actions (GITHUB_ENV set) it
# also exports HARBORMASTER_RUSTFS_BINARY for later steps; locally, export it
# yourself from the printed path.
set -euo pipefail

RUSTFS_VERSION=1.0.0
REPO=rustfs/rustfs
ASSET_NAME="rustfs-linux-x86_64-musl-v${RUSTFS_VERSION}.zip"
ASSET_SIZE=194469895

# Recorded once by downloading ASSET_NAME for RUSTFS_VERSION and hashing it,
# because the release's SHA256SUMS asset (checked below when present) is the
# preferred source of truth. Used as a fallback if that asset ever
# disappears from the release.
ASSET_SHA256_FALLBACK=c30a95b76546f25122c9ca387090ddb30c391ca5605621b0d7c881703c0f21c8

dir="$(mktemp -d "${TMPDIR:-/tmp}/harbormaster-rustfs.XXXXXX")"

rel="$(curl -fsSL "https://api.github.com/repos/${REPO}/releases/tags/${RUSTFS_VERSION}")"
asset_url="$(jq -r --arg n "$ASSET_NAME" '.assets[] | select(.name == $n) | .browser_download_url' <<<"$rel")"
if [ -z "$asset_url" ]; then
    echo "nightly-rustfs-binary: release ${RUSTFS_VERSION} has no asset named ${ASSET_NAME}" >&2
    exit 1
fi
sums_url="$(jq -r '.assets[] | select(.name == "SHA256SUMS") | .browser_download_url' <<<"$rel")"

curl -fsSL -o "$dir/$ASSET_NAME" "$asset_url"

actual_size="$(stat -c%s "$dir/$ASSET_NAME" 2>/dev/null || stat -f%z "$dir/$ASSET_NAME")"
if [ "$actual_size" != "$ASSET_SIZE" ]; then
    echo "nightly-rustfs-binary: $ASSET_NAME is $actual_size bytes, expected $ASSET_SIZE" >&2
    exit 1
fi

if [ -n "$sums_url" ]; then
    curl -fsSL -o "$dir/SHA256SUMS" "$sums_url"
    sum_line="$(awk -v n="$ASSET_NAME" '$2 == n' "$dir/SHA256SUMS")"
    if [ -z "$sum_line" ]; then
        echo "nightly-rustfs-binary: SHA256SUMS has no line for $ASSET_NAME" >&2
        exit 1
    fi
    (cd "$dir" && sha256sum -c - <<<"$sum_line")
else
    echo "nightly-rustfs-binary: release ${RUSTFS_VERSION} publishes no SHA256SUMS asset; verifying against the recorded fallback checksum" >&2
    echo "$ASSET_SHA256_FALLBACK  $dir/$ASSET_NAME" | sha256sum -c -
fi

unzip -q -o "$dir/$ASSET_NAME" -d "$dir" rustfs
bin="$dir/rustfs"
chmod +x "$bin"

echo "HARBORMASTER_RUSTFS_BINARY=$bin"
"$bin" --version
if [ -n "${GITHUB_ENV:-}" ]; then
    echo "HARBORMASTER_RUSTFS_BINARY=$bin" >> "$GITHUB_ENV"
fi
