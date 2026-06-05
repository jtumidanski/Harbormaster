// JSON:API wire types for the object browser. The list endpoint returns
// a heterogeneous data[] array mixing object_entries (real keys) and
// object_prefixes (common-prefix "folders"). The discriminant is the
// JSON:API `type` field; see apps/backend/internal/objects/rest.go.

export type ObjectEntry = {
  key: string;
  size: number;
  last_modified: string;
  content_type: string;
  etag: string;
};

export type ObjectPrefix = {
  prefix: string;
};

export type ObjectListItem =
  | { type: "object_entries"; id: string; attributes: ObjectEntry }
  | { type: "object_prefixes"; id: string; attributes: ObjectPrefix };

export type ObjectListResponse = {
  data: ObjectListItem[];
  meta?: {
    page?: {
      size: number;
      next_token?: string;
    };
  };
};

// JSON:API wire types for the version history endpoint.
// size is null for delete markers (they have no object content).
export type ObjectVersionAttributes = {
  key: string;
  version_id: string;
  size: number | null;
  last_modified: string;
  etag?: string;
  content_type?: string;
  is_latest: boolean;
  is_delete_marker: boolean;
};

export type ObjectVersionItem = {
  type: "object_versions";
  id: string; // "<key>@<version_id>"
  attributes: ObjectVersionAttributes;
};

export type ObjectVersionListResponse = {
  data: ObjectVersionItem[];
  meta?: {
    page?: {
      size: number;
      next_token?: string;
    };
  };
};
