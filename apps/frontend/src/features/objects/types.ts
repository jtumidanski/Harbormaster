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
