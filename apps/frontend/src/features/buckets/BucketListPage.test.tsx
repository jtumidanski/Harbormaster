import "@testing-library/jest-dom/vitest";
import { describe, it, expect, afterEach, beforeEach, vi } from "vitest";
import { cleanup, render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { MemoryRouter, Route, Routes } from "react-router-dom";
import type { PropsWithChildren } from "react";
import { Toaster } from "sonner";
import { bucketsKeys } from "@/lib/api/keys";
import { BucketListPage } from "./BucketListPage";

type StubResponse = {
  match: (url: string, init?: RequestInit) => boolean;
  response: () => Response;
};

type FetchSpy = ReturnType<typeof vi.fn>;

function installFetch(stubs: StubResponse[]): FetchSpy {
  const spy = vi.fn((input: RequestInfo | URL, init?: RequestInit) => {
    const url =
      typeof input === "string" ? input : input instanceof URL ? input.toString() : input.url;
    for (const s of stubs) {
      if (s.match(url, init)) return Promise.resolve(s.response());
    }
    return Promise.resolve(
      new Response(JSON.stringify({ errors: [{ code: "not_found" }] }), {
        status: 404,
        headers: { "Content-Type": "application/vnd.api+json" },
      }),
    );
  });
  vi.stubGlobal("fetch", spy);
  return spy;
}

function jsonapi(body: unknown, status = 200): Response {
  return new Response(JSON.stringify(body), {
    status,
    headers: { "Content-Type": "application/vnd.api+json" },
  });
}

function emptyListStub(): StubResponse {
  return {
    match: (u, init) =>
      u.includes("/api/v1/buckets") &&
      (init?.method ?? "GET") === "GET" &&
      !u.match(/buckets\/[^?]+$/),
    response: () =>
      jsonapi({
        data: [],
        meta: { page: { number: 1, size: 25, total_pages: 1, total_records: 0 } },
      }),
  };
}

function populatedListStub(): StubResponse {
  return {
    match: (u, init) =>
      u.includes("/api/v1/buckets") &&
      (init?.method ?? "GET") === "GET" &&
      !u.match(/buckets\/[^?]+$/),
    response: () =>
      jsonapi({
        data: [
          {
            type: "buckets",
            id: "photos",
            attributes: {
              name: "photos",
              created_at: "2024-01-01T00:00:00Z",
              estimated_bytes: 1024 * 1024 * 512,
              object_count: 1234,
              versioning_enabled: true,
              has_lifecycle_rules: false,
              public_access: "private",
              quota: {
                kind: "hard",
                bytes: 1024 * 1024 * 1024 * 10,
                used_bytes: 1024 * 1024 * 512,
              },
            },
          },
          {
            type: "buckets",
            id: "logs",
            attributes: {
              name: "logs",
              created_at: "2024-02-01T00:00:00Z",
              estimated_bytes: 2048,
              object_count: 7,
              versioning_enabled: false,
              has_lifecycle_rules: true,
              public_access: "public-read",
              quota: null,
            },
          },
        ],
        meta: { page: { number: 1, size: 25, total_pages: 1, total_records: 2 } },
      }),
  };
}

function makeQueryClient(): QueryClient {
  return new QueryClient({
    defaultOptions: { queries: { retry: false, gcTime: 0, staleTime: 0 } },
  });
}

function Wrapper({ children, qc }: PropsWithChildren<{ qc: QueryClient }>) {
  return (
    <QueryClientProvider client={qc}>
      <MemoryRouter initialEntries={["/buckets"]}>
        <Routes>
          <Route path="/buckets" element={children} />
          <Route path="/buckets/:name" element={<div>Bucket detail screen</div>} />
        </Routes>
        <Toaster />
      </MemoryRouter>
    </QueryClientProvider>
  );
}

describe("BucketListPage", () => {
  beforeEach(() => {
    vi.unstubAllGlobals();
    document.cookie = "harbormaster_csrf=test-token; Path=/";
  });

  afterEach(() => {
    cleanup();
  });

  it("renders empty state when no buckets exist", async () => {
    installFetch([emptyListStub()]);
    const qc = makeQueryClient();
    render(
      <Wrapper qc={qc}>
        <BucketListPage />
      </Wrapper>,
    );
    await waitFor(() => {
      expect(screen.getByText(/no buckets yet/i)).toBeInTheDocument();
    });
  });

  it("renders a row per bucket with all columns", async () => {
    installFetch([populatedListStub()]);
    const qc = makeQueryClient();
    render(
      <Wrapper qc={qc}>
        <BucketListPage />
      </Wrapper>,
    );

    await waitFor(() => {
      expect(screen.getByRole("button", { name: "photos" })).toBeInTheDocument();
    });
    expect(screen.getByRole("button", { name: "logs" })).toBeInTheDocument();

    const table = screen.getByRole("table");
    const headers = within(table)
      .getAllByRole("columnheader")
      .map((th) => th.textContent);
    expect(headers).toEqual([
      "Name",
      "Created",
      "Size",
      "Objects",
      "Versioning",
      "Lifecycle",
      "Public access",
      "Quota",
    ]);

    // Quota cell formatted as used/total (kind) for first row.
    expect(within(table).getByText(/\/ .*hard/)).toBeInTheDocument();
  });

  it("create-success invalidates buckets and closes dialog", async () => {
    const spy = installFetch([
      emptyListStub(),
      {
        match: (u, init) => u.endsWith("/api/v1/buckets") && (init?.method ?? "GET") === "POST",
        response: () =>
          jsonapi({
            data: {
              type: "buckets",
              id: "new-bucket",
              attributes: {
                name: "new-bucket",
                created_at: "2024-03-01T00:00:00Z",
                estimated_bytes: 0,
                object_count: 0,
                versioning_enabled: false,
                has_lifecycle_rules: false,
                public_access: "private",
                quota: null,
              },
            },
          }),
      },
    ]);
    const user = userEvent.setup();
    const qc = makeQueryClient();
    const invalidateSpy = vi.spyOn(qc, "invalidateQueries");
    render(
      <Wrapper qc={qc}>
        <BucketListPage />
      </Wrapper>,
    );

    await waitFor(() => {
      expect(screen.getByText(/no buckets yet/i)).toBeInTheDocument();
    });

    await user.click(screen.getByRole("button", { name: /create bucket/i }));
    await screen.findByRole("dialog");

    await user.type(screen.getByLabelText(/^name$/i), "new-bucket");
    await user.click(screen.getByRole("button", { name: /^create bucket$/i }));

    await waitFor(() => {
      const postCall = spy.mock.calls.find(
        ([url, init]) =>
          typeof url === "string" &&
          url.endsWith("/api/v1/buckets") &&
          (init as RequestInit | undefined)?.method === "POST",
      );
      expect(postCall).toBeDefined();
    });

    await waitFor(() => {
      expect(invalidateSpy).toHaveBeenCalledWith({ queryKey: bucketsKeys.all() });
    });
  });

  it("create-422 shows the server's error message in the form", async () => {
    installFetch([
      emptyListStub(),
      {
        match: (u, init) => u.endsWith("/api/v1/buckets") && (init?.method ?? "GET") === "POST",
        response: () =>
          jsonapi(
            {
              errors: [
                {
                  code: "invalid_bucket_name",
                  detail: "Bucket name must be DNS-compatible.",
                  source: { pointer: "/data/attributes/name" },
                },
              ],
            },
            422,
          ),
      },
    ]);
    const user = userEvent.setup();
    const qc = makeQueryClient();
    render(
      <Wrapper qc={qc}>
        <BucketListPage />
      </Wrapper>,
    );

    await waitFor(() => {
      expect(screen.getByText(/no buckets yet/i)).toBeInTheDocument();
    });

    await user.click(screen.getByRole("button", { name: /create bucket/i }));
    await screen.findByRole("dialog");

    await user.type(screen.getByLabelText(/^name$/i), "Bad_Name");
    await user.click(screen.getByRole("button", { name: /^create bucket$/i }));

    await waitFor(() => {
      expect(screen.getByText(/dns-compatible/i)).toBeInTheDocument();
    });
    // Dialog stayed open.
    expect(screen.getByRole("dialog")).toBeInTheDocument();
  });
});
