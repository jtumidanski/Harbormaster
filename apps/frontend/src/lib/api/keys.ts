export const authKeys = {
  me: () => ["auth", "me"] as const,
  setupStatus: () => ["setup", "status"] as const,
  mcAliases: () => ["setup", "mc-aliases"] as const,
  csrf: () => ["auth", "csrf"] as const,
};

export const connectionKeys = {
  detail: () => ["connection", "detail"] as const,
};

export const bucketsKeys = {
  all: () => ["buckets"] as const,
  list: (params: { page: number; size: number; sort: string }) =>
    ["buckets", "list", params] as const,
  detail: (name: string) => ["buckets", "detail", name] as const,
};

export const objectsKeys = {
  list: (bucket: string, prefix: string) => ["objects", bucket, prefix] as const,
  versions: (bucket: string, key: string) => ["objects", bucket, "versions", key] as const,
};

export const lifecycleKeys = {
  list: (bucket: string) => ["lifecycle", bucket] as const,
};

export const usersKeys = {
  all: () => ["users"] as const,
  list: () => ["users", "list"] as const,
  detail: (key: string) => ["users", "detail", key] as const,
};

export const serviceAccountsKeys = {
  forUser: (key: string) => ["service-accounts", key] as const,
};

export const policyTemplatesKeys = {
  list: () => ["policy-templates"] as const,
};

export const policiesKeys = {
  all: () => ["policies"] as const,
  list: () => ["policies", "list"] as const,
  detail: (name: string) => ["policies", "detail", name] as const,
};

export const dashboardKeys = {
  view: (window: string) => ["dashboard", "view", window] as const,
  failures: (window: string) => ["dashboard", "failures", window] as const,
};

export const metricsKeys = {
  view: (window: string) => ["metrics", "view", window] as const,
};

export type AuditFilterKey = {
  action?: string;
  target_type?: string;
  target_id?: string;
  outcome?: string;
  from?: string;
  to?: string;
};

export const activityKeys = {
  list: (filters: AuditFilterKey, page: { number: number; size: number }) =>
    ["activity", "list", filters, page] as const,
};
