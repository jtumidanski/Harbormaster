import { describe, it, expect, afterEach, beforeEach, vi } from "vitest";
import { cleanup, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { MemoryRouter, Route, Routes } from "react-router-dom";
import type { PropsWithChildren } from "react";
import { Toaster } from "sonner";
import { AuthProvider } from "@/context/AuthContext";
import { authKeys } from "@/lib/api/keys";
import { LoginPage } from "./LoginPage";

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

function jsonapi(body: unknown, status: number): Response {
  return new Response(JSON.stringify(body), {
    status,
    headers: { "Content-Type": "application/vnd.api+json" },
  });
}

function noContent(): Response {
  return new Response(null, { status: 204 });
}

function unauthenticatedMeStub(): StubResponse {
  return {
    match: (u) => u.includes("/api/v1/auth/me"),
    response: () =>
      jsonapi({ errors: [{ code: "unauthenticated", detail: "not signed in" }] }, 401),
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
      <AuthProvider>
        <MemoryRouter initialEntries={["/login"]}>
          <Routes>
            <Route path="/login" element={children} />
            <Route path="/buckets" element={<div>Buckets screen</div>} />
          </Routes>
          <Toaster />
        </MemoryRouter>
      </AuthProvider>
    </QueryClientProvider>
  );
}

async function fillAndSubmit(
  user: ReturnType<typeof userEvent.setup>,
  username = "admin",
  password = "supersecretpwd",
) {
  await user.type(screen.getByLabelText(/username/i), username);
  await user.type(screen.getByLabelText(/password/i), password);
  await user.click(screen.getByRole("button", { name: /sign in/i }));
}

describe("LoginPage", () => {
  beforeEach(() => {
    vi.unstubAllGlobals();
    document.cookie = "harbormaster_csrf=test-token; Path=/";
  });

  afterEach(() => {
    cleanup();
  });

  it("shows a toast and stays on /login when credentials are invalid (401)", async () => {
    installFetch([
      unauthenticatedMeStub(),
      {
        match: (u, init) => u.endsWith("/api/v1/auth/login") && (init?.method ?? "GET") === "POST",
        response: () =>
          jsonapi(
            { errors: [{ code: "invalid_credentials", detail: "invalid username or password" }] },
            401,
          ),
      },
    ]);
    const user = userEvent.setup();
    const qc = makeQueryClient();
    render(
      <Wrapper qc={qc}>
        <LoginPage />
      </Wrapper>,
    );

    await fillAndSubmit(user);

    await waitFor(() => {
      const statuses = screen.getAllByRole("status");
      const text = statuses.map((n) => n.textContent ?? "").join(" ");
      expect(text.toLowerCase()).toContain("invalid");
    });

    // Still on the login form.
    expect(screen.getByLabelText(/username/i)).toBeInTheDocument();
    expect(screen.queryByText(/buckets screen/i)).not.toBeInTheDocument();
  });

  it("shows the server's message when rate-limited (429 too_many_attempts)", async () => {
    installFetch([
      unauthenticatedMeStub(),
      {
        match: (u, init) => u.endsWith("/api/v1/auth/login") && (init?.method ?? "GET") === "POST",
        response: () =>
          jsonapi(
            {
              errors: [
                {
                  code: "too_many_attempts",
                  detail: "Too many login attempts, please try again in 5 minutes.",
                },
              ],
            },
            429,
          ),
      },
    ]);
    const user = userEvent.setup();
    const qc = makeQueryClient();
    render(
      <Wrapper qc={qc}>
        <LoginPage />
      </Wrapper>,
    );

    await fillAndSubmit(user);

    await waitFor(() => {
      const statuses = screen.getAllByRole("status");
      const text = statuses.map((n) => n.textContent ?? "").join(" ");
      expect(text).toContain("Too many login attempts");
    });
    expect(screen.queryByText(/buckets screen/i)).not.toBeInTheDocument();
  });

  it("on 204 success, invalidates auth.me and navigates to /buckets", async () => {
    installFetch([
      unauthenticatedMeStub(),
      {
        match: (u, init) => u.endsWith("/api/v1/auth/login") && (init?.method ?? "GET") === "POST",
        response: () => noContent(),
      },
    ]);
    const user = userEvent.setup();
    const qc = makeQueryClient();
    const invalidateSpy = vi.spyOn(qc, "invalidateQueries");
    render(
      <Wrapper qc={qc}>
        <LoginPage />
      </Wrapper>,
    );

    await fillAndSubmit(user);

    await waitFor(() => {
      expect(screen.getByText(/buckets screen/i)).toBeInTheDocument();
    });
    expect(invalidateSpy).toHaveBeenCalledWith({ queryKey: authKeys.me() });
  });

  it("shows field-level errors when username and password are empty", async () => {
    installFetch([unauthenticatedMeStub()]);
    const user = userEvent.setup();
    const qc = makeQueryClient();
    render(
      <Wrapper qc={qc}>
        <LoginPage />
      </Wrapper>,
    );

    await user.click(screen.getByRole("button", { name: /sign in/i }));

    await waitFor(() => {
      expect(screen.getAllByText("required").length).toBeGreaterThanOrEqual(2);
    });
  });
});
