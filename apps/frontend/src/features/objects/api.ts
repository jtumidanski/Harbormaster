import { api } from "@/lib/api/client";
import type { ObjectListResponse } from "./types";

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
