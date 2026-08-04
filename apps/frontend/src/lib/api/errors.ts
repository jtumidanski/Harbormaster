export type AppErrorDetails = Record<string, unknown>;

export class AppError extends Error {
  status: number;
  code: string;
  details?: AppErrorDetails;
  pointer?: string;

  constructor(opts: {
    status: number;
    code: string;
    message: string;
    details?: AppErrorDetails;
    pointer?: string;
  }) {
    super(opts.message);
    this.status = opts.status;
    this.code = opts.code;
    if (opts.details !== undefined) {
      this.details = opts.details;
    }
    if (opts.pointer !== undefined) {
      this.pointer = opts.pointer;
    }
  }
}

// JSON:API error members are untyped, so coerce them to a display string
// without letting a stray object stringify to "[object Object]".
function errorText(value: unknown, fallback: string): string {
  if (typeof value === "string") return value;
  if (typeof value === "number" || typeof value === "boolean") return String(value);
  return fallback;
}

export async function parseErrorResponse(res: Response): Promise<AppError> {
  let body: unknown = {};
  try {
    body = await res.json();
  } catch {
    /* ignore */
  }
  const b = body as Record<string, unknown>;
  if (Array.isArray(b.errors) && b.errors.length > 0) {
    const e = b.errors[0] as Record<string, unknown>;
    const pointer = (e.source as Record<string, unknown> | undefined)?.pointer as
      | string
      | undefined;
    const details = e.meta as AppErrorDetails | undefined;
    return new AppError({
      status: res.status,
      code: errorText(e.code, "unknown"),
      message: errorText(e.detail, errorText(e.title, res.statusText)),
      ...(pointer !== undefined ? { pointer } : {}),
      ...(details !== undefined ? { details } : {}),
    });
  }
  if (b.error && typeof b.error === "object") {
    const e = b.error as Record<string, unknown>;
    const details = e.details as AppErrorDetails | undefined;
    return new AppError({
      status: res.status,
      code: errorText(e.code, "unknown"),
      message: errorText(e.message, res.statusText),
      ...(details !== undefined ? { details } : {}),
    });
  }
  return new AppError({ status: res.status, code: "unknown", message: res.statusText });
}
