import { api } from "@/lib/api/client";
import type { ObjectListResponse, ObjectVersionListResponse } from "./types";

// Page size matches the backend's clampPageSize default; the server caps
// at 1000 but 100 keeps each round-trip small enough for snappy scroll.
const PAGE_SIZE = 100;

export async function listObjects(
  bucket: string,
  prefix: string,
  pageToken?: string,
): Promise<ObjectListResponse> {
  const sp = new URLSearchParams({
    prefix,
    delimiter: "/",
    "page[size]": String(PAGE_SIZE),
  });
  if (pageToken) sp.set("page[token]", pageToken);
  return api.get<ObjectListResponse>(
    `/api/v1/buckets/${encodeURIComponent(bucket)}/objects?${sp.toString()}`,
  );
}

export async function deleteObject(bucket: string, key: string): Promise<void> {
  const sp = new URLSearchParams({ key });
  await api.delete<void>(`/api/v1/buckets/${encodeURIComponent(bucket)}/objects?${sp.toString()}`);
}

// Backend wraps the share link in a JSON:API single-resource document.
// The URL is sensitive — never log it — and the `expires_at` field is an
// ISO-8601 timestamp.
export type ShareLink = {
  url: string;
  expires_at: string;
};

export type ShareLinkResponse = {
  data: {
    type: "object_share_links";
    id: string;
    attributes: ShareLink;
  };
};

export async function createShareLink(
  bucket: string,
  key: string,
  expiresSeconds: number,
): Promise<ShareLink> {
  const res = await api.post<ShareLinkResponse>(
    `/api/v1/buckets/${encodeURIComponent(bucket)}/objects/share-links`,
    { key, expires_seconds: expiresSeconds },
  );
  return res.data.attributes;
}

// downloadURL builds a stable URL the browser can hit directly via
// <a download> or fetch with Range headers; the backend chooses proxy
// vs presigned-redirect based on the configured DownloadProxyMode.
export function downloadURL(bucket: string, key: string): string {
  const sp = new URLSearchParams({ key });
  return `/api/v1/buckets/${encodeURIComponent(bucket)}/objects/download?${sp.toString()}`;
}

// versionDownloadURL builds the download URL for a specific object version.
export function versionDownloadURL(bucket: string, key: string, versionId: string): string {
  const sp = new URLSearchParams({ key, version_id: versionId });
  return `/api/v1/buckets/${encodeURIComponent(bucket)}/objects/download?${sp.toString()}`;
}

// Version page size: smaller than object list because version history tends
// to be short and we want the first page to cover the typical "recent" view.
const VERSION_PAGE_SIZE = 50;

export async function listVersions(
  bucket: string,
  key: string,
  pageToken?: string,
): Promise<ObjectVersionListResponse> {
  const sp = new URLSearchParams({
    key,
    "page[size]": String(VERSION_PAGE_SIZE),
  });
  if (pageToken) sp.set("page[token]", pageToken);
  return api.get<ObjectVersionListResponse>(
    `/api/v1/buckets/${encodeURIComponent(bucket)}/objects/versions?${sp.toString()}`,
  );
}

export type RestoreVersionResult = {
  key: string;
  version_id: string;
  restored_from: string;
};

export async function restoreVersion(
  bucket: string,
  key: string,
  versionId: string,
): Promise<RestoreVersionResult> {
  return api.post<RestoreVersionResult>(
    `/api/v1/buckets/${encodeURIComponent(bucket)}/objects/restore-version`,
    { key, version_id: versionId },
  );
}

export async function deleteVersion(bucket: string, key: string, versionId: string): Promise<void> {
  // api.delete accepts a body as the second argument (same signature as api.post);
  // the backend reads `confirm` from the JSON body to guard against accidental calls.
  const sp = new URLSearchParams({ key, version_id: versionId });
  await api.delete<void>(
    `/api/v1/buckets/${encodeURIComponent(bucket)}/objects/version?${sp.toString()}`,
    { confirm: true },
  );
}

export type UndeleteResult = {
  key: string;
  version_id: string;
};

export async function undeleteObject(bucket: string, key: string): Promise<UndeleteResult> {
  return api.post<UndeleteResult>(
    `/api/v1/buckets/${encodeURIComponent(bucket)}/objects/undelete`,
    { key },
  );
}

// Bulk-delete wire types. The endpoint is POST .../objects/bulk-delete
// with a `dry_run` flag selecting the count preview vs. the real delete.
export type BulkDeletePreview = {
  object_count: number;
  truncated: boolean;
};

export type BulkDeleteFailure = {
  key: string;
  error: string;
};

export type BulkDeleteResult = {
  deleted_count: number;
  failures: BulkDeleteFailure[];
};

// previewBulkDelete returns the dry-run object count (exact up to 10,000,
// then truncated) for the given keys + prefixes WITHOUT deleting anything.
export async function previewBulkDelete(
  bucket: string,
  args: { keys: string[]; prefixes: string[] },
): Promise<BulkDeletePreview> {
  return api.post<BulkDeletePreview>(
    `/api/v1/buckets/${encodeURIComponent(bucket)}/objects/bulk-delete`,
    { keys: args.keys, prefixes: args.prefixes, dry_run: true },
  );
}

// bulkDelete performs the real delete of the explicit keys plus every key
// under each prefix, returning the deleted count and per-key failures.
export async function bulkDelete(
  bucket: string,
  args: { keys: string[]; prefixes: string[] },
): Promise<BulkDeleteResult> {
  return api.post<BulkDeleteResult>(
    `/api/v1/buckets/${encodeURIComponent(bucket)}/objects/bulk-delete`,
    { keys: args.keys, prefixes: args.prefixes, dry_run: false },
  );
}
