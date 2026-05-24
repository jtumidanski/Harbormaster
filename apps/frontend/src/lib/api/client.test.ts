import { describe, it, expect, beforeEach, vi } from "vitest";
import { api } from "./client";

describe("api", () => {
  beforeEach(() => {
    document.cookie = "harbormaster_csrf=test-token; Path=/";
    vi.stubGlobal(
      "fetch",
      vi.fn((_input: RequestInfo, _init?: RequestInit) => {
        return Promise.resolve(
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
      }),
    );
  });

  it("sends CSRF header on POST", async () => {
    await api.post("/api/v1/buckets", { data: {} });
    const call = (fetch as unknown as ReturnType<typeof vi.fn>).mock.calls[0];
    const init = call[1] as RequestInit;
    expect((init.headers as Record<string, string>)["X-CSRF-Token"]).toBe("test-token");
  });

  it("does not send CSRF header on GET", async () => {
    await api.get("/api/v1/buckets");
    const init = (fetch as unknown as ReturnType<typeof vi.fn>).mock.calls[0][1] as RequestInit;
    expect((init.headers as Record<string, string>)["X-CSRF-Token"]).toBeUndefined();
  });
});
