import { api } from "@/lib/api/client";
import type {
  Bucket,
  BucketCollectionResponse,
  BucketSingleResponse,
  PublicAccess,
  QuotaKind,
} from "./types";

export type ListBucketsParams = { page: number; size: number; sort: string };

export type ListBucketsResult = {
  buckets: Bucket[];
  page: { number: number; size: number; total_pages: number; total_records: number } | undefined;
};

export async function listBuckets(params: ListBucketsParams): Promise<ListBucketsResult> {
  const sp = new URLSearchParams({
    "page[number]": String(params.page),
    "page[size]": String(params.size),
    sort: params.sort,
  });
  const res = await api.get<BucketCollectionResponse>(`/api/v1/buckets?${sp.toString()}`);
  return {
    buckets: res.data.map((d) => d.attributes),
    page: res.meta?.page,
  };
}

export async function getBucket(name: string): Promise<Bucket> {
  const res = await api.get<BucketSingleResponse>(`/api/v1/buckets/${encodeURIComponent(name)}`);
  return res.data.attributes;
}

export type CreateBucketRequest = {
  name: string;
  versioning_enabled: boolean;
  public_access: PublicAccess;
  quota?: { kind: QuotaKind; bytes: number };
  lifecycle_template?: string | null;
};

export async function createBucket(input: CreateBucketRequest): Promise<Bucket> {
  const res = await api.post<BucketSingleResponse>("/api/v1/buckets", {
    data: { type: "buckets", attributes: input },
  });
  return res.data.attributes;
}

export async function deleteBucket(name: string, confirmName: string): Promise<void> {
  await api.delete<void>(`/api/v1/buckets/${encodeURIComponent(name)}`, {
    confirm_name: confirmName,
  });
}

export async function setBucketVersioning(name: string, enabled: boolean): Promise<void> {
  await api.put<void>(`/api/v1/buckets/${encodeURIComponent(name)}/versioning`, { enabled });
}

export async function setBucketPublicAccess(
  name: string,
  mode: PublicAccess,
  confirmName?: string,
): Promise<void> {
  const body: { mode: PublicAccess; confirm_name?: string } = { mode };
  if (confirmName !== undefined) body.confirm_name = confirmName;
  await api.put<void>(`/api/v1/buckets/${encodeURIComponent(name)}/public-access`, body);
}

export async function setBucketQuota(
  name: string,
  kind: QuotaKind | "none",
  bytes?: number,
): Promise<void> {
  const body = kind === "none" ? { kind: "none" as const } : { kind, bytes: bytes ?? 0 };
  await api.put<void>(`/api/v1/buckets/${encodeURIComponent(name)}/quota`, body);
}
