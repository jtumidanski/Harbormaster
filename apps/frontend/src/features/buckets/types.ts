export type PublicAccess = "private" | "public-read" | "public-read-write";
export type QuotaKind = "hard" | "fifo";

export type Quota = {
  kind: QuotaKind;
  bytes: number;
  used_bytes: number;
};

export type Bucket = {
  name: string;
  created_at: string;
  estimated_bytes: number;
  object_count: number;
  versioning_enabled: boolean;
  has_lifecycle_rules: boolean;
  public_access: PublicAccess;
  quota: Quota | null;
};

export type BucketResource = {
  type: "buckets";
  id: string;
  attributes: Bucket;
};

export type BucketCollectionResponse = {
  data: Array<BucketResource>;
  meta?: {
    page?: {
      number: number;
      size: number;
      total_pages: number;
      total_records: number;
    };
  };
};

export type BucketSingleResponse = {
  data: BucketResource;
};
