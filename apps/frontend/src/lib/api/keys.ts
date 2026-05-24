export const authKeys = {
  me: () => ["auth", "me"] as const,
  setupStatus: () => ["setup", "status"] as const,
  mcAliases: () => ["setup", "mc-aliases"] as const,
  csrf: () => ["auth", "csrf"] as const,
};

export const connectionKeys = {
  detail: () => ["connection", "detail"] as const,
};
