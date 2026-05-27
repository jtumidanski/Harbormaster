import { describe, it, expect, afterEach, beforeEach, vi } from "vitest";
import { cleanup, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { MemoryRouter, Route, Routes } from "react-router-dom";
import type { PropsWithChildren } from "react";
import { Toaster } from "sonner";
import { connectionKeys } from "@/lib/api/keys";
import { ConnectionSettingsPage } from "./ConnectionSettingsPage";

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

function json(body: unknown, status = 200): Response {
  return new Response(JSON.stringify(body), {
    status,
    headers: { "Content-Type": "application/json" },
  });
}

function jsonapi(body: unknown, status: number): Response {
  return new Response(JSON.stringify(body), {
    status,
    headers: { "Content-Type": "application/vnd.api+json" },
  });
}

function noContent(): Response {
  return new Response(null, { status: 204 });
}

function detailStub(): StubResponse {
  return {
    match: (u, init) => u.endsWith("/api/v1/connection") && (init?.method ?? "GET") === "GET",
    response: () =>
      json({
        endpoint_url: "https://minio.lan:9000",
        tls_skip_verify: false,
        access_key_masked: "AKIA****PLE",
        secret_key_present: true,
        custom_ca_pem_present: false,
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
      <MemoryRouter initialEntries={["/settings/connection"]}>
        <Routes>
          <Route path="/settings/connection" element={children} />
        </Routes>
        <Toaster />
      </MemoryRouter>
    </QueryClientProvider>
  );
}

async function clickEdit(user: ReturnType<typeof userEvent.setup>) {
  const editBtn = await screen.findByRole("button", { name: /^edit$/i });
  await user.click(editBtn);
  await screen.findByLabelText(/minio endpoint url/i);
}

describe("ConnectionSettingsPage", () => {
  beforeEach(() => {
    vi.unstubAllGlobals();
    document.cookie = "harbormaster_csrf=test-token; Path=/";
  });

  afterEach(() => {
    cleanup();
  });

  it("renders detail view with masked access key", async () => {
    installFetch([detailStub()]);
    const qc = makeQueryClient();
    render(
      <Wrapper qc={qc}>
        <ConnectionSettingsPage />
      </Wrapper>,
    );

    await waitFor(() => {
      expect(screen.getByText("AKIA****PLE")).toBeInTheDocument();
    });
    expect(screen.getByText("https://minio.lan:9000")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: /^edit$/i })).toBeInTheDocument();
  });

  it("Edit reveals form populated with endpoint_url and tls_skip_verify, secret/access blank", async () => {
    installFetch([detailStub()]);
    const user = userEvent.setup();
    const qc = makeQueryClient();
    render(
      <Wrapper qc={qc}>
        <ConnectionSettingsPage />
      </Wrapper>,
    );

    await clickEdit(user);

    const endpoint = screen.getByLabelText<HTMLInputElement>(/minio endpoint url/i);
    const accessKey = screen.getByLabelText<HTMLInputElement>(/access key/i);
    const secretKey = screen.getByLabelText<HTMLInputElement>(/secret key/i);
    const tls = screen.getByLabelText<HTMLInputElement>(/skip tls verification/i);

    expect(endpoint.value).toBe("https://minio.lan:9000");
    expect(tls.checked).toBe(false);
    expect(accessKey.value).toBe("");
    expect(secretKey.value).toBe("");
  });

  it("Submit PUTs snake_case payload; on success invalidates query and collapses form", async () => {
    const spy = installFetch([
      detailStub(),
      {
        match: (u, init) => u.endsWith("/api/v1/connection") && (init?.method ?? "GET") === "PUT",
        response: () => noContent(),
      },
    ]);
    const user = userEvent.setup();
    const qc = makeQueryClient();
    const invalidateSpy = vi.spyOn(qc, "invalidateQueries");
    render(
      <Wrapper qc={qc}>
        <ConnectionSettingsPage />
      </Wrapper>,
    );

    await clickEdit(user);

    await user.type(screen.getByLabelText(/access key/i), "AKIA-EXAMPLE");
    await user.type(screen.getByLabelText(/secret key/i), "supersecretvalue");
    await user.click(screen.getByRole("button", { name: /^save$/i }));

    await waitFor(() => {
      const putCall = spy.mock.calls.find(
        ([url, init]) =>
          typeof url === "string" &&
          url.endsWith("/api/v1/connection") &&
          (init as RequestInit | undefined)?.method === "PUT",
      );
      expect(putCall).toBeDefined();
    });

    const putCall = spy.mock.calls.find(
      ([url, init]) =>
        typeof url === "string" &&
        url.endsWith("/api/v1/connection") &&
        (init as RequestInit | undefined)?.method === "PUT",
    )!;
    const init = putCall[1] as RequestInit;
    const body = JSON.parse(init.body as string) as Record<string, unknown>;
    expect(body).toEqual({
      endpoint_url: "https://minio.lan:9000",
      access_key: "AKIA-EXAMPLE",
      secret_key: "supersecretvalue",
      tls_skip_verify: false,
      custom_ca_pem: null,
    });

    await waitFor(() => {
      expect(invalidateSpy).toHaveBeenCalledWith({ queryKey: connectionKeys.detail() });
    });

    // Form collapses; Edit button returns.
    await waitFor(() => {
      expect(screen.getByRole("button", { name: /^edit$/i })).toBeInTheDocument();
    });
    expect(screen.queryByLabelText(/minio endpoint url/i)).not.toBeInTheDocument();
  });

  it("Test connection while editing POSTs to /api/v1/connection/test and renders three booleans + version", async () => {
    const spy = installFetch([
      detailStub(),
      {
        match: (u, init) =>
          u.endsWith("/api/v1/connection/test") && (init?.method ?? "GET") === "POST",
        response: () =>
          json({
            tcp_connect: "ok",
            list_buckets: "ok",
            admin_ping: { failed: "permission denied" },
            minio_version: "RELEASE.2024-01-01T00-00-00Z",
          }),
      },
    ]);
    const user = userEvent.setup();
    const qc = makeQueryClient();
    render(
      <Wrapper qc={qc}>
        <ConnectionSettingsPage />
      </Wrapper>,
    );

    await clickEdit(user);

    await user.type(screen.getByLabelText(/access key/i), "AKIA-EXAMPLE");
    await user.type(screen.getByLabelText(/secret key/i), "supersecretvalue");
    await user.click(screen.getByRole("button", { name: /test connection/i }));

    await waitFor(() => {
      const postCall = spy.mock.calls.find(
        ([url, init]) =>
          typeof url === "string" &&
          url.endsWith("/api/v1/connection/test") &&
          (init as RequestInit | undefined)?.method === "POST",
      );
      expect(postCall).toBeDefined();
    });

    const postCall = spy.mock.calls.find(
      ([url, init]) =>
        typeof url === "string" &&
        url.endsWith("/api/v1/connection/test") &&
        (init as RequestInit | undefined)?.method === "POST",
    )!;
    const init = postCall[1] as RequestInit;
    const body = JSON.parse(init.body as string) as Record<string, unknown>;
    expect(body).toEqual({
      endpoint_url: "https://minio.lan:9000",
      access_key: "AKIA-EXAMPLE",
      secret_key: "supersecretvalue",
      tls_skip_verify: false,
      custom_ca_pem: null,
    });

    await waitFor(() => {
      expect(screen.getByTestId("check-tcp_connect")).toHaveTextContent("ok");
    });
    expect(screen.getByTestId("check-list_buckets")).toHaveTextContent("ok");
    expect(screen.getByTestId("check-admin_ping")).toHaveTextContent("failed: permission denied");
    expect(screen.getByText(/RELEASE\.2024-01-01T00-00-00Z/)).toBeInTheDocument();
  });

  it("422 with minio_unreachable shows toast and stays in edit mode", async () => {
    installFetch([
      detailStub(),
      {
        match: (u, init) => u.endsWith("/api/v1/connection") && (init?.method ?? "GET") === "PUT",
        response: () =>
          jsonapi(
            { errors: [{ code: "minio_unreachable", detail: "MinIO endpoint unreachable" }] },
            422,
          ),
      },
    ]);
    const user = userEvent.setup();
    const qc = makeQueryClient();
    render(
      <Wrapper qc={qc}>
        <ConnectionSettingsPage />
      </Wrapper>,
    );

    await clickEdit(user);

    await user.type(screen.getByLabelText(/access key/i), "AKIA-EXAMPLE");
    await user.type(screen.getByLabelText(/secret key/i), "supersecretvalue");
    await user.click(screen.getByRole("button", { name: /^save$/i }));

    await waitFor(() => {
      const statuses = screen.getAllByRole("status");
      const text = statuses.map((n) => n.textContent ?? "").join(" ");
      expect(text.toLowerCase()).toContain("minio");
    });

    // Still in edit mode.
    expect(screen.getByLabelText(/minio endpoint url/i)).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: /^edit$/i })).not.toBeInTheDocument();
  });
});
