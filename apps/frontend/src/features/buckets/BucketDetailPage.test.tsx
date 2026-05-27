import "@testing-library/jest-dom/vitest";
import { describe, it, expect, afterEach, beforeEach, vi } from "vitest";
import { cleanup, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { MemoryRouter, Route, Routes } from "react-router-dom";
import type { PropsWithChildren } from "react";
import { Toaster } from "sonner";
import { BucketDetailPage } from "./BucketDetailPage";

type StubResponse = {
  match: (url: string, init?: RequestInit) => boolean;
  response: () => Response;
};

function installFetch(stubs: StubResponse[]): ReturnType<typeof vi.fn> {
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

function makeQueryClient(): QueryClient {
  return new QueryClient({
    defaultOptions: { queries: { retry: false, gcTime: 0, staleTime: 0 } },
  });
}

function bucketStub(overrides: Record<string, unknown> = {}): StubResponse {
  return {
    match: (u, init) => u.includes("/api/v1/buckets/photos") && (init?.method ?? "GET") === "GET",
    response: () =>
      jsonapi({
        data: {
          type: "buckets",
          id: "photos",
          attributes: {
            name: "photos",
            created_at: "2024-01-01T00:00:00Z",
            estimated_bytes: 1024 * 1024 * 100,
            object_count: 5,
            versioning_enabled: false,
            has_lifecycle_rules: false,
            public_access: "private",
            quota: null,
            ...overrides,
          },
        },
      }),
  };
}

function Wrapper({ children, qc }: PropsWithChildren<{ qc: QueryClient }>) {
  return (
    <QueryClientProvider client={qc}>
      <MemoryRouter initialEntries={["/buckets/photos"]}>
        <Routes>
          <Route path="/buckets/:name" element={children} />
          <Route path="/buckets" element={<div>Bucket list screen</div>} />
        </Routes>
        <Toaster />
      </MemoryRouter>
    </QueryClientProvider>
  );
}

describe("BucketDetailPage", () => {
  beforeEach(() => {
    vi.unstubAllGlobals();
    document.cookie = "harbormaster_csrf=test-token; Path=/";
  });

  afterEach(() => {
    cleanup();
  });

  it("renders the bucket name and overview metadata", async () => {
    installFetch([bucketStub()]);
    const qc = makeQueryClient();
    render(
      <Wrapper qc={qc}>
        <BucketDetailPage />
      </Wrapper>,
    );

    await waitFor(() => {
      expect(screen.getByRole("heading", { name: "photos" })).toBeInTheDocument();
    });
    expect(screen.getByText(/object count/i)).toBeInTheDocument();
  });

  it("disables Delete bucket when objects exist and shows Empty-first link", async () => {
    installFetch([bucketStub({ object_count: 10 })]);
    const qc = makeQueryClient();
    render(
      <Wrapper qc={qc}>
        <BucketDetailPage />
      </Wrapper>,
    );

    await waitFor(() => {
      expect(screen.getByRole("heading", { name: "photos" })).toBeInTheDocument();
    });
    const deleteBtn = screen.getByRole("button", { name: /delete bucket/i });
    expect(deleteBtn).toBeDisabled();
    expect(screen.getByRole("button", { name: /empty this bucket first/i })).toBeInTheDocument();
  });

  it("opens the public access dialog from the Edit public access button", async () => {
    installFetch([bucketStub()]);
    const user = userEvent.setup();
    const qc = makeQueryClient();
    render(
      <Wrapper qc={qc}>
        <BucketDetailPage />
      </Wrapper>,
    );

    await waitFor(() => {
      expect(screen.getByRole("heading", { name: "photos" })).toBeInTheDocument();
    });

    await user.click(screen.getByRole("button", { name: /edit public access/i }));
    const dialog = await screen.findByRole("dialog");
    expect(dialog).toBeInTheDocument();
    // Save button starts enabled when current mode == "private" (no confirm required).
    expect(screen.getByRole("button", { name: /^save$/i })).toBeEnabled();
  });
});
