import type { AppError } from "./errors";
import { parseErrorResponse } from "./errors";

type Method = "GET" | "POST" | "PUT" | "PATCH" | "DELETE";

const UNSAFE = new Set<Method>(["POST", "PUT", "PATCH", "DELETE"]);

function readCsrfCookie(): string {
  const m = document.cookie.match(/(?:^|;\s*)harbormaster_csrf=([^;]+)/);
  return m ? decodeURIComponent(m[1]) : "";
}

async function request<T>(
  method: Method,
  path: string,
  body?: unknown,
  init?: RequestInit,
): Promise<T> {
  const headers: Record<string, string> = {
    Accept: "application/vnd.api+json, application/json",
    ...(init?.headers as Record<string, string> | undefined),
  };
  let bodyInit: BodyInit | undefined;
  if (body !== undefined) {
    if (body instanceof FormData) {
      bodyInit = body;
    } else {
      headers["Content-Type"] = "application/json";
      bodyInit = JSON.stringify(body);
    }
  }
  if (UNSAFE.has(method)) {
    const t = readCsrfCookie();
    if (t) headers["X-CSRF-Token"] = t;
  }
  const res = await fetch(path, {
    method,
    credentials: "include",
    ...(bodyInit !== undefined ? { body: bodyInit } : {}),
    headers,
    ...init,
  });
  if (!res.ok) {
    throw await parseErrorResponse(res);
  }
  if (res.status === 204) {
    return undefined as T;
  }
  const ct = res.headers.get("Content-Type") ?? "";
  if (ct.includes("text/event-stream")) {
    return res as unknown as T;
  }
  return (await res.json()) as T;
}

export const api = {
  get: <T>(path: string, init?: RequestInit) => request<T>("GET", path, undefined, init),
  post: <T>(path: string, body?: unknown, init?: RequestInit) =>
    request<T>("POST", path, body, init),
  put: <T>(path: string, body?: unknown, init?: RequestInit) =>
    request<T>("PUT", path, body, init),
  patch: <T>(path: string, body?: unknown, init?: RequestInit) =>
    request<T>("PATCH", path, body, init),
  delete: <T>(path: string, body?: unknown, init?: RequestInit) =>
    request<T>("DELETE", path, body, init),
};

export type { AppError };
