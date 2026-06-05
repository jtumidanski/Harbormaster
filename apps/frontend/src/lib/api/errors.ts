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
      code: String(e.code ?? "unknown"),
      message: String(e.detail ?? e.title ?? res.statusText),
      ...(pointer !== undefined ? { pointer } : {}),
      ...(details !== undefined ? { details } : {}),
    });
  }
  if (b.error && typeof b.error === "object") {
    const e = b.error as Record<string, unknown>;
    const details = e.details as AppErrorDetails | undefined;
    return new AppError({
      status: res.status,
      code: String(e.code ?? "unknown"),
      message: String(e.message ?? res.statusText),
      ...(details !== undefined ? { details } : {}),
    });
  }
  return new AppError({ status: res.status, code: "unknown", message: res.statusText });
}
