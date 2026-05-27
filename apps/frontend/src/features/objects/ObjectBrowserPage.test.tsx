import "@testing-library/jest-dom/vitest";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { MemoryRouter, Route, Routes } from "react-router-dom";
import type { PropsWithChildren } from "react";
import { Toaster } from "sonner";
import { ObjectBrowserPage } from "./ObjectBrowserPage";

// jsdom doesn't ship ResizeObserver; the virtualiser guards on its
// presence but installs a no-op fallback when missing, so we provide a
// stub so `useVirtualizer` exercises the same code path it would in a
// browser without throwing on unrelated reflows during the test.
class ResizeObserverStub {
  observe(): void {}
  unobserve(): void {}
  disconnect(): void {}
}
(globalThis as unknown as { ResizeObserver: typeof ResizeObserverStub }).ResizeObserver =
  ResizeObserverStub;

// jsdom's getBoundingClientRect always returns zeroes, which makes
// useVirtualizer think the viewport has no height and skips rendering
// every row. Override at the prototype level so the scroller and any
// row-measure elements report a reasonable size during tests.
// eslint-disable-next-line @typescript-eslint/unbound-method -- stashing the prototype function intentionally for fallback delegation.
const originalGetBoundingClientRect = Element.prototype.getBoundingClientRect;
function patchedGetBoundingClientRect(this: Element): DOMRect {
  const self = this as HTMLElement;
  if (self.dataset && self.dataset.testid === "object-list-scroller") {
    return {
      width: 800,
      height: 480,
      top: 0,
      left: 0,
      bottom: 480,
      right: 800,
      x: 0,
      y: 0,
      toJSON: () => ({}),
    } as DOMRect;
  }
  return originalGetBoundingClientRect.call(this);
}
Element.prototype.getBoundingClientRect = patchedGetBoundingClientRect;

type FetchSpy = ReturnType<typeof vi.fn>;

type StubResponse = {
  match: (url: string, init?: RequestInit) => boolean;
  response: () => Response;
};

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

function entries(n: number, opts: { withNextToken?: string; startIdx?: number } = {}): unknown {
  const start = opts.startIdx ?? 0;
  const data = Array.from({ length: n }, (_, i) => {
    const idx = start + i;
    return {
      type: "object_entries",
      id: `obj-${idx}`,
      attributes: {
        key: `obj-${idx}.txt`,
        size: 100 + idx,
        last_modified: "2024-01-01T00:00:00Z",
        content_type: "text/plain",
        etag: `etag-${idx}`,
      },
    };
  });
  const meta: { page: { size: number; next_token?: string } } = { page: { size: 100 } };
  if (opts.withNextToken) meta.page.next_token = opts.withNextToken;
  return { data, meta };
}

function makeQueryClient(): QueryClient {
  return new QueryClient({
    defaultOptions: { queries: { retry: false, gcTime: 0, staleTime: 0 } },
  });
}

function Wrapper({ children, qc }: PropsWithChildren<{ qc: QueryClient }>) {
  return (
    <QueryClientProvider client={qc}>
      <MemoryRouter initialEntries={["/buckets/photos"]}>
        <Routes>
          <Route path="/buckets/:name" element={children} />
        </Routes>
        <Toaster />
      </MemoryRouter>
    </QueryClientProvider>
  );
}

// In jsdom layout properties default to 0; we patch the scroller's
// geometry so the threshold math (`(scrollTop + clientHeight)/scrollHeight`)
// behaves the way it would in a real browser.
function configureScroller(opts: { scrollHeight: number; clientHeight: number }): HTMLElement {
  const el = screen.getByTestId("object-list-scroller");
  Object.defineProperty(el, "scrollHeight", { configurable: true, value: opts.scrollHeight });
  Object.defineProperty(el, "clientHeight", { configurable: true, value: opts.clientHeight });
  return el;
}

function setScrollTop(el: HTMLElement, top: number) {
  Object.defineProperty(el, "scrollTop", { configurable: true, value: top });
  fireEvent.scroll(el);
}

describe("ObjectBrowserPage virtualized list", () => {
  beforeEach(() => {
    vi.unstubAllGlobals();
    document.cookie = "harbormaster_csrf=test-token; Path=/";
  });

  afterEach(() => {
    cleanup();
  });

  it("renders 10 entries from a stubbed listObjects response", async () => {
    installFetch([
      {
        match: (u, init) =>
          u.includes("/api/v1/buckets/photos/objects") && (init?.method ?? "GET") === "GET",
        response: () => jsonapi(entries(10)),
      },
    ]);
    const qc = makeQueryClient();
    render(
      <Wrapper qc={qc}>
        <ObjectBrowserPage bucket="photos" />
      </Wrapper>,
    );

    await waitFor(() => {
      expect(screen.getByText("obj-0.txt")).toBeInTheDocument();
    });
    // Sanity: the last entry from the page is rendered as well.
    expect(screen.getByText("obj-9.txt")).toBeInTheDocument();
    expect(screen.getByText(/10 item\(s\) loaded/i)).toBeInTheDocument();
  });

  it("scroll past 90% triggers fetchNextPage exactly once and a pending fetch blocks further triggers", async () => {
    const resolveRef: { fn: (() => void) | null } = { fn: null };
    const calls = { list: 0 };
    installFetch([
      {
        match: (u, init) =>
          u.includes("/api/v1/buckets/photos/objects") && (init?.method ?? "GET") === "GET",
        response: () => {
          calls.list += 1;
          if (calls.list === 1) {
            return jsonapi(entries(10, { withNextToken: "tok-2" }));
          }
          return jsonapi(entries(10, { startIdx: 10 }));
        },
      },
    ]);
    // Spy on global fetch already installed. Wrap the second-page fetch
    // to be deferred so we can assert the "pending blocks" property.
    const realFetch = (
      globalThis as unknown as {
        fetch: (input: RequestInfo | URL, init?: RequestInit) => Promise<Response>;
      }
    ).fetch;
    const wrapped = vi.fn((input: RequestInfo | URL, init?: RequestInit) => {
      const url =
        typeof input === "string" ? input : input instanceof URL ? input.toString() : input.url;
      if (url.includes("/api/v1/buckets/photos/objects") && url.includes("page%5Btoken%5D=tok-2")) {
        return new Promise<Response>((resolve) => {
          resolveRef.fn = () => resolve(jsonapi(entries(10, { startIdx: 10 })));
        });
      }
      return realFetch(input, init);
    });
    vi.stubGlobal("fetch", wrapped);

    const qc = makeQueryClient();
    render(
      <Wrapper qc={qc}>
        <ObjectBrowserPage bucket="photos" />
      </Wrapper>,
    );
    await waitFor(() => expect(screen.getByText("obj-0.txt")).toBeInTheDocument());

    const el = configureScroller({ scrollHeight: 1000, clientHeight: 100 });
    // ratio = (820+100)/1000 = 0.92 — past the 0.9 threshold.
    setScrollTop(el, 820);

    await waitFor(() => {
      const pageCalls = wrapped.mock.calls.filter(([u]) =>
        typeof u === "string" ? u.includes("page%5Btoken%5D=tok-2") : false,
      );
      expect(pageCalls.length).toBe(1);
    });

    // Scroll again past threshold while the fetch is still in flight —
    // the one-outstanding cap must suppress this.
    setScrollTop(el, 900);
    setScrollTop(el, 850);
    await new Promise((r) => setTimeout(r, 20));
    const pendingCalls = wrapped.mock.calls.filter(([u]) =>
      typeof u === "string" ? u.includes("page%5Btoken%5D=tok-2") : false,
    );
    expect(pendingCalls.length).toBe(1);

    // Resolve and confirm no new fetch is issued because the second
    // page returns no next_token (hasNextPage flips to false).
    resolveRef.fn?.();
    await waitFor(() => {
      expect(screen.getByText(/20 item\(s\) loaded/i)).toBeInTheDocument();
    });
    const finalCalls = wrapped.mock.calls.filter(([u]) =>
      typeof u === "string" ? u.includes("page%5Btoken%5D=tok-2") : false,
    );
    expect(finalCalls.length).toBe(1);
  });

  it("manual Load more button calls fetchNextPage once", async () => {
    const calls = { list: 0 };
    installFetch([
      {
        match: (u, init) =>
          u.includes("/api/v1/buckets/photos/objects") && (init?.method ?? "GET") === "GET",
        response: () => {
          calls.list += 1;
          if (calls.list === 1) {
            return jsonapi(entries(10, { withNextToken: "tok-2" }));
          }
          return jsonapi(entries(5, { startIdx: 10 }));
        },
      },
    ]);
    const user = userEvent.setup();
    const qc = makeQueryClient();
    render(
      <Wrapper qc={qc}>
        <ObjectBrowserPage bucket="photos" />
      </Wrapper>,
    );
    await waitFor(() => expect(screen.getByText("obj-0.txt")).toBeInTheDocument());

    const btn = screen.getByRole("button", { name: /load more/i });
    expect(btn).toBeEnabled();
    await user.click(btn);

    await waitFor(() => {
      expect(screen.getByText(/15 item\(s\) loaded/i)).toBeInTheDocument();
    });
    expect(calls.list).toBe(2);
  });
});
