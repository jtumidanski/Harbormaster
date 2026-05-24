import "@testing-library/jest-dom/vitest";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { cleanup, render, screen, waitFor } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { MemoryRouter } from "react-router-dom";
import type { PropsWithChildren } from "react";
import { Toaster } from "sonner";
import { LifecycleRulesTab } from "./LifecycleRulesTab";

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

function Wrapper({ children, qc }: PropsWithChildren<{ qc: QueryClient }>) {
  return (
    <QueryClientProvider client={qc}>
      <MemoryRouter>
        {children}
        <Toaster />
      </MemoryRouter>
    </QueryClientProvider>
  );
}

describe("LifecycleRulesTab", () => {
  beforeEach(() => {
    vi.unstubAllGlobals();
    document.cookie = "harbormaster_csrf=test-token; Path=/";
  });

  afterEach(() => {
    cleanup();
  });

  it("renders managed and unmanaged sections from the stubbed list response", async () => {
    installFetch([
      {
        match: (u, init) =>
          u.includes("/api/v1/buckets/photos/lifecycle-rules") && (init?.method ?? "GET") === "GET",
        response: () =>
          jsonapi({
            data: [
              {
                type: "lifecycle_rules",
                id: "managed-1",
                attributes: { managed: true, kind: "expiration", days: 30, prefix: "logs/" },
              },
              {
                type: "lifecycle_rules",
                id: "external-xml",
                attributes: { managed: false, summary: "External rule (XML import)" },
              },
            ],
          }),
      },
    ]);
    const qc = makeQueryClient();
    render(
      <Wrapper qc={qc}>
        <LifecycleRulesTab bucket="photos" />
      </Wrapper>,
    );

    await waitFor(() => {
      expect(screen.getByText("Managed rules")).toBeInTheDocument();
    });
    expect(screen.getByText("managed-1")).toBeInTheDocument();
    expect(screen.getByText(/expire after 30 day/i)).toBeInTheDocument();
    expect(screen.getByText("external-xml")).toBeInTheDocument();
    expect(screen.getByText(/external rule \(xml import\)/i)).toBeInTheDocument();
  });

  it("shows empty-state copy when there are no managed rules", async () => {
    installFetch([
      {
        match: (u, init) =>
          u.includes("/api/v1/buckets/photos/lifecycle-rules") && (init?.method ?? "GET") === "GET",
        response: () => jsonapi({ data: [] }),
      },
    ]);
    const qc = makeQueryClient();
    render(
      <Wrapper qc={qc}>
        <LifecycleRulesTab bucket="photos" />
      </Wrapper>,
    );

    await waitFor(() => expect(screen.getByText(/no managed rules/i)).toBeInTheDocument());
  });
});
