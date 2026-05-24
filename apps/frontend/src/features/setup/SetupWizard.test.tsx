import { describe, it, expect, afterEach, beforeEach, vi } from "vitest";
import { cleanup, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { MemoryRouter, Route, Routes } from "react-router-dom";
import type { PropsWithChildren } from "react";
import { Toaster } from "sonner";
import { SetupWizard } from "./SetupWizard";
import { authKeys } from "@/lib/api/keys";

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

function makeQueryClient(): QueryClient {
  return new QueryClient({
    defaultOptions: { queries: { retry: false, gcTime: 0, staleTime: 0 } },
  });
}

function Wrapper({ children, qc }: PropsWithChildren<{ qc: QueryClient }>) {
  return (
    <QueryClientProvider client={qc}>
      <MemoryRouter initialEntries={["/setup"]}>
        <Routes>
          <Route path="/setup" element={children} />
          <Route path="/login" element={<div>Login screen lands in T2.12</div>} />
        </Routes>
        <Toaster />
      </MemoryRouter>
    </QueryClientProvider>
  );
}

// Polyfills for Radix Select in jsdom (which lacks pointer capture & scrollIntoView).
function installRadixSelectShims() {
  if (!Element.prototype.hasPointerCapture) {
    Element.prototype.hasPointerCapture = () => false;
  }
  if (!Element.prototype.releasePointerCapture) {
    Element.prototype.releasePointerCapture = () => {};
  }
  if (!Element.prototype.scrollIntoView) {
    Element.prototype.scrollIntoView = () => {};
  }
}

async function fillAdminAndContinue(user: ReturnType<typeof userEvent.setup>) {
  await user.type(screen.getByLabelText(/admin username/i), "admin");
  await user.type(screen.getByLabelText(/^password$/i), "correcthorsebattery!");
  await user.type(screen.getByLabelText(/confirm password/i), "correcthorsebattery!");
  await user.click(screen.getByRole("button", { name: /continue/i }));
}

describe("SetupWizard", () => {
  beforeEach(() => {
    vi.unstubAllGlobals();
    document.cookie = "harbormaster_csrf=test-token; Path=/";
    installRadixSelectShims();
  });

  afterEach(() => {
    cleanup();
  });

  it("renders the admin step first and advances to the MinIO step on valid submit", async () => {
    installFetch([
      {
        match: (u) => u.includes("/api/v1/setup/mc-aliases"),
        response: () => json({ aliases: [] }),
      },
    ]);
    const user = userEvent.setup();
    const qc = makeQueryClient();
    render(
      <Wrapper qc={qc}>
        <SetupWizard />
      </Wrapper>,
    );

    expect(screen.getByLabelText(/admin username/i)).toBeInTheDocument();
    expect(screen.queryByLabelText(/minio endpoint url/i)).not.toBeInTheDocument();

    await fillAdminAndContinue(user);

    await waitFor(() => {
      expect(screen.getByLabelText(/minio endpoint url/i)).toBeInTheDocument();
    });
    expect(screen.queryByLabelText(/admin username/i)).not.toBeInTheDocument();
  });

  it("renders the mc-alias select when aliases are returned and pre-fills endpoint/access key on selection", async () => {
    installFetch([
      {
        match: (u) => u.includes("/api/v1/setup/mc-aliases"),
        response: () =>
          json({
            aliases: [
              {
                name: "myminio",
                endpoint: "https://minio.lan:9000",
                access_key: "AKIAEXAMPLE",
                tls_skip_verify: false,
              },
            ],
          }),
      },
    ]);
    const user = userEvent.setup();
    const qc = makeQueryClient();
    render(
      <Wrapper qc={qc}>
        <SetupWizard />
      </Wrapper>,
    );

    await fillAdminAndContinue(user);

    const trigger = await screen.findByLabelText(/import from mc alias/i);
    await user.click(trigger);
    const option = await screen.findByRole("option", { name: /myminio/i });
    await user.click(option);

    const endpoint = screen.getByLabelText<HTMLInputElement>(/minio endpoint url/i);
    const accessKey = screen.getByLabelText<HTMLInputElement>(/access key/i);
    await waitFor(() => {
      expect(endpoint.value).toBe("https://minio.lan:9000");
      expect(accessKey.value).toBe("AKIAEXAMPLE");
    });
  });

  it("POSTs combined payload to /api/v1/setup and shows a toast on 422 minio_unreachable (stays on step 2)", async () => {
    const spy = installFetch([
      {
        match: (u) => u.includes("/api/v1/setup/mc-aliases"),
        response: () => json({ aliases: [] }),
      },
      {
        match: (u, init) => u.endsWith("/api/v1/setup") && (init?.method ?? "GET") === "POST",
        response: () =>
          new Response(
            JSON.stringify({
              errors: [{ code: "minio_unreachable", detail: "MinIO endpoint unreachable" }],
            }),
            {
              status: 422,
              headers: { "Content-Type": "application/vnd.api+json" },
            },
          ),
      },
    ]);
    const user = userEvent.setup();
    const qc = makeQueryClient();
    render(
      <Wrapper qc={qc}>
        <SetupWizard />
      </Wrapper>,
    );

    await fillAdminAndContinue(user);

    await user.type(await screen.findByLabelText(/minio endpoint url/i), "https://minio.lan:9000");
    await user.type(screen.getByLabelText(/access key/i), "AKIA-EXAMPLE");
    await user.type(screen.getByLabelText(/secret key/i), "supersecretvalue");
    await user.click(screen.getByRole("button", { name: /finish setup/i }));

    // Verify POST payload shape.
    await waitFor(() => {
      const postCall = spy.mock.calls.find(
        ([url, init]) =>
          typeof url === "string" &&
          url.endsWith("/api/v1/setup") &&
          (init as RequestInit | undefined)?.method === "POST",
      );
      expect(postCall).toBeDefined();
    });
    const postCall = spy.mock.calls.find(
      ([url, init]) =>
        typeof url === "string" &&
        url.endsWith("/api/v1/setup") &&
        (init as RequestInit | undefined)?.method === "POST",
    )!;
    const init = postCall[1] as RequestInit;
    const body = JSON.parse(init.body as string) as Record<string, unknown>;
    expect(body).toEqual({
      admin: { username: "admin", password: "correcthorsebattery!" },
      minio: {
        endpoint_url: "https://minio.lan:9000",
        access_key: "AKIA-EXAMPLE",
        secret_key: "supersecretvalue",
        tls_skip_verify: false,
        custom_ca_pem: null,
      },
    });

    // Toast surface: sonner renders into a portal with role="status".
    await waitFor(() => {
      const statuses = screen.getAllByRole("status");
      const text = statuses.map((n) => n.textContent ?? "").join(" ");
      expect(text.toLowerCase()).toContain("minio");
    });

    // Still on MinIO step.
    expect(screen.getByLabelText(/minio endpoint url/i)).toBeInTheDocument();
  });

  it("on 201 success, invalidates setupStatus and navigates to /login", async () => {
    installFetch([
      {
        match: (u) => u.includes("/api/v1/setup/mc-aliases"),
        response: () => json({ aliases: [] }),
      },
      {
        match: (u, init) => u.endsWith("/api/v1/setup") && (init?.method ?? "GET") === "POST",
        response: () => json({ initialized: true }, 201),
      },
    ]);
    const user = userEvent.setup();
    const qc = makeQueryClient();
    const invalidateSpy = vi.spyOn(qc, "invalidateQueries");
    render(
      <Wrapper qc={qc}>
        <SetupWizard />
      </Wrapper>,
    );

    await fillAdminAndContinue(user);
    await user.type(await screen.findByLabelText(/minio endpoint url/i), "https://minio.lan:9000");
    await user.type(screen.getByLabelText(/access key/i), "AKIA-EXAMPLE");
    await user.type(screen.getByLabelText(/secret key/i), "supersecretvalue");
    await user.click(screen.getByRole("button", { name: /finish setup/i }));

    await waitFor(() => {
      expect(screen.getByText(/login screen lands in t2\.12/i)).toBeInTheDocument();
    });
    expect(invalidateSpy).toHaveBeenCalledWith({ queryKey: authKeys.setupStatus() });
  });
});
