import "@testing-library/jest-dom/vitest";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { cleanup, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { MemoryRouter } from "react-router-dom";
import type { PropsWithChildren } from "react";
import { Toaster } from "sonner";
import { ObjectVersionsSheet } from "./ObjectVersionsSheet";

// ─── Stubs ────────────────────────────────────────────────────────────────────

type StubResponse = {
  match: (url: string, init?: RequestInit) => boolean;
  response: () => Response;
};

function jsonapi(body: unknown, status = 200): Response {
  return new Response(JSON.stringify(body), {
    status,
    headers: { "Content-Type": "application/vnd.api+json" },
  });
}

function installFetch(stubs: StubResponse[]) {
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

// Two versions: one a delete marker (latest), one a regular object version
function twoVersionsResponse(): unknown {
  return {
    data: [
      {
        type: "object_versions",
        id: "photos/cat.jpg@vid-001",
        attributes: {
          key: "photos/cat.jpg",
          version_id: "vid-001",
          size: null,
          last_modified: "2024-06-01T12:00:00Z",
          is_latest: true,
          is_delete_marker: true,
        },
      },
      {
        type: "object_versions",
        id: "photos/cat.jpg@vid-002",
        attributes: {
          key: "photos/cat.jpg",
          version_id: "vid-002",
          size: 1024,
          last_modified: "2024-05-30T10:00:00Z",
          etag: "abc123",
          content_type: "image/jpeg",
          is_latest: false,
          is_delete_marker: false,
        },
      },
    ],
    meta: { page: { size: 50 } },
  };
}

function restoreResponse(): unknown {
  return {
    key: "photos/cat.jpg",
    version_id: "vid-002",
    restored_from: "vid-002",
  };
}

// ─── Helpers ─────────────────────────────────────────────────────────────────

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

// ─── Tests ────────────────────────────────────────────────────────────────────

describe("ObjectVersionsSheet", () => {
  beforeEach(() => {
    vi.unstubAllGlobals();
    document.cookie = "harbormaster_csrf=test-token; Path=/";
  });

  afterEach(() => {
    cleanup();
  });

  it("renders version rows with Latest and Delete-marker badges", async () => {
    installFetch([
      {
        match: (u) => u.includes("/api/v1/buckets/photos/objects/versions"),
        response: () => jsonapi(twoVersionsResponse()),
      },
    ]);

    const qc = makeQueryClient();
    render(
      <Wrapper qc={qc}>
        <ObjectVersionsSheet
          bucket="photos"
          objectKey="photos/cat.jpg"
          prefix=""
          open
          onOpenChange={() => undefined}
        />
      </Wrapper>,
    );

    // Both version IDs should appear (truncated to 8 chars in the component)
    await waitFor(() => {
      expect(screen.getByText(/vid-001/i)).toBeInTheDocument();
    });
    expect(screen.getByText(/vid-002/i)).toBeInTheDocument();

    // Badges
    expect(screen.getByText("Latest")).toBeInTheDocument();
    expect(screen.getByText("Delete marker")).toBeInTheDocument();
  });

  it("Restore button is disabled for delete-marker rows", async () => {
    installFetch([
      {
        match: (u) => u.includes("/api/v1/buckets/photos/objects/versions"),
        response: () => jsonapi(twoVersionsResponse()),
      },
    ]);

    const qc = makeQueryClient();
    render(
      <Wrapper qc={qc}>
        <ObjectVersionsSheet
          bucket="photos"
          objectKey="photos/cat.jpg"
          prefix=""
          open
          onOpenChange={() => undefined}
        />
      </Wrapper>,
    );

    await waitFor(() => {
      expect(screen.getByText(/vid-001/i)).toBeInTheDocument();
    });

    // The delete-marker row (vid-001) should have its Restore button disabled.
    // The regular row (vid-002) should have its Restore button enabled.
    const restoreButtons = screen.getAllByRole("button", { name: /restore/i });
    // There should be at least two restore buttons (one per non-folder row).
    // The one corresponding to the delete-marker row should be disabled.
    const disabledRestore = restoreButtons.find((b) => b.hasAttribute("disabled"));
    expect(disabledRestore).toBeDefined();
  });

  it("clicking Restore on a non-marker row, then confirming, calls restoreVersion", async () => {
    const fetchSpy = installFetch([
      {
        match: (u) => u.includes("/api/v1/buckets/photos/objects/versions"),
        response: () => jsonapi(twoVersionsResponse()),
      },
      {
        match: (u, init) =>
          u.includes("/api/v1/buckets/photos/objects/restore-version") &&
          (init?.method ?? "POST") === "POST",
        response: () => jsonapi(restoreResponse()),
      },
    ]);

    const user = userEvent.setup();
    const qc = makeQueryClient();
    render(
      <Wrapper qc={qc}>
        <ObjectVersionsSheet
          bucket="photos"
          objectKey="photos/cat.jpg"
          prefix=""
          open
          onOpenChange={() => undefined}
        />
      </Wrapper>,
    );

    await waitFor(() => {
      expect(screen.getByText(/vid-002/i)).toBeInTheDocument();
    });

    // Find the enabled Restore button (the non-marker row vid-002)
    const restoreButtons = screen.getAllByRole("button", { name: /restore/i });
    const enabledRestore = restoreButtons.find((b) => !b.hasAttribute("disabled"));
    expect(enabledRestore).toBeDefined();
    await user.click(enabledRestore!);

    // Confirmation dialog should appear
    await waitFor(() => {
      expect(screen.getByRole("dialog", { name: /restore version/i })).toBeInTheDocument();
    });

    // Confirm the restore
    const confirmBtn = screen.getByRole("button", { name: /^restore$/i });
    await user.click(confirmBtn);

    // restoreVersion POST should have been called
    await waitFor(() => {
      const postCalls = fetchSpy.mock.calls.filter(
        ([u, init]) =>
          typeof u === "string" &&
          u.includes("/objects/restore-version") &&
          (init as RequestInit)?.method === "POST",
      );
      expect(postCalls.length).toBeGreaterThan(0);
    });
  });

  it("shows an Undelete button when the latest version is a delete marker", async () => {
    installFetch([
      {
        match: (u) => u.includes("/api/v1/buckets/photos/objects/versions"),
        response: () => jsonapi(twoVersionsResponse()),
      },
    ]);

    const qc = makeQueryClient();
    render(
      <Wrapper qc={qc}>
        <ObjectVersionsSheet
          bucket="photos"
          objectKey="photos/cat.jpg"
          prefix=""
          open
          onOpenChange={() => undefined}
        />
      </Wrapper>,
    );

    await waitFor(() => {
      expect(screen.getByText(/vid-001/i)).toBeInTheDocument();
    });

    // Undelete button should be present because latest version is a delete marker
    expect(screen.getByRole("button", { name: /undelete/i })).toBeInTheDocument();
  });
});
