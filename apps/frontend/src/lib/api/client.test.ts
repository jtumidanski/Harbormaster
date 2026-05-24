import { describe, it, expect, beforeEach, vi } from "vitest";
import { api } from "./client";
import { AppError } from "./errors";

function stubFetch(response: Response) {
  vi.stubGlobal(
    "fetch",
    vi.fn((_input: RequestInfo, _init?: RequestInit) => Promise.resolve(response)),
  );
}

function lastInit(): RequestInit {
  const mock = fetch as unknown as ReturnType<typeof vi.fn>;
  return mock.mock.calls[mock.mock.calls.length - 1][1] as RequestInit;
}

function lastHeaders(): Record<string, string> {
  return lastInit().headers as Record<string, string>;
}

describe("api", () => {
  beforeEach(() => {
    document.cookie = "harbormaster_csrf=test-token; Path=/";
    stubFetch(
      new Response(
        JSON.stringify({
          data: { type: "buckets", id: "x", attributes: { name: "x" } },
        }),
        {
          status: 200,
          headers: { "Content-Type": "application/vnd.api+json" },
        },
      ),
    );
  });

  it("sends CSRF header on POST", async () => {
    await api.post("/api/v1/buckets", { data: {} });
    expect(lastHeaders()["X-CSRF-Token"]).toBe("test-token");
  });

  it("does not send CSRF header on GET", async () => {
    await api.get("/api/v1/buckets");
    expect(lastHeaders()["X-CSRF-Token"]).toBeUndefined();
  });

  it("preserves caller-provided init.headers alongside CSRF + Content-Type on POST", async () => {
    await api.post("/p", { hello: "world" }, { headers: { Foo: "bar" } });
    const headers = lastHeaders();
    expect(headers["X-CSRF-Token"]).toBe("test-token");
    expect(headers["Foo"]).toBe("bar");
    expect(headers["Content-Type"]).toBe("application/json");
    expect(headers["Accept"]).toBe("application/vnd.api+json, application/json");
    // Caller's init must not clobber method/credentials either.
    expect(lastInit().method).toBe("POST");
    expect(lastInit().credentials).toBe("include");
  });

  it("does not let caller init override managed method/credentials/body/headers", async () => {
    await api.post(
      "/p",
      { real: "body" },
      {
        // Cast to satisfy the public surface while exercising the clobber guard.
        method: "GET",
        credentials: "omit",
        body: "evil",
        headers: { "X-CSRF-Token": "forged" },
      } as RequestInit,
    );
    const init = lastInit();
    expect(init.method).toBe("POST");
    expect(init.credentials).toBe("include");
    expect(init.body).toBe(JSON.stringify({ real: "body" }));
    // Caller's X-CSRF-Token header is intentionally allowed to be overridden by
    // ours — verify ours wins (cookie value, not "forged").
    expect((init.headers as Record<string, string>)["X-CSRF-Token"]).toBe("test-token");
  });

  it("parses JSON:API error envelope into AppError with code/message/pointer", async () => {
    stubFetch(
      new Response(
        JSON.stringify({
          errors: [
            {
              code: "validation_failed",
              detail: "name is required",
              source: { pointer: "/data/attributes/name" },
            },
          ],
        }),
        { status: 422, headers: { "Content-Type": "application/vnd.api+json" } },
      ),
    );
    await expect(api.get("/p")).rejects.toMatchObject({
      code: "validation_failed",
      message: "name is required",
      pointer: "/data/attributes/name",
      status: 422,
    });
    await expect(api.get("/p")).rejects.toBeInstanceOf(AppError);
  });

  it("parses {error:{code,message,details}} envelope into AppError", async () => {
    stubFetch(
      new Response(
        JSON.stringify({
          error: {
            code: "conflict",
            message: "bucket already exists",
            details: { bucket: "photos" },
          },
        }),
        { status: 409, headers: { "Content-Type": "application/json" } },
      ),
    );
    await expect(api.post("/p", {})).rejects.toMatchObject({
      code: "conflict",
      message: "bucket already exists",
      details: { bucket: "photos" },
      status: 409,
    });
  });

  it("returns undefined for 204 No Content", async () => {
    stubFetch(new Response(null, { status: 204 }));
    const result = await api.delete("/p");
    expect(result).toBeUndefined();
  });

  it("does not set Content-Type when body is FormData (browser sets boundary)", async () => {
    const fd = new FormData();
    fd.append("file", new Blob(["abc"]), "f.txt");
    await api.post("/p", fd);
    const headers = lastHeaders();
    expect(headers["Content-Type"]).toBeUndefined();
    // CSRF still applied on unsafe methods.
    expect(headers["X-CSRF-Token"]).toBe("test-token");
    // Body forwarded as the FormData instance, not stringified.
    expect(lastInit().body).toBe(fd);
  });

  it("falls back to AppError{code:'unknown', message:statusText} on empty/invalid JSON error body", async () => {
    stubFetch(
      new Response("not json at all", {
        status: 500,
        statusText: "Internal Server Error",
        headers: { "Content-Type": "text/plain" },
      }),
    );
    await expect(api.get("/p")).rejects.toMatchObject({
      code: "unknown",
      message: "Internal Server Error",
      status: 500,
    });
  });
});
