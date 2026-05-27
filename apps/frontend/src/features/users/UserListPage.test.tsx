import "@testing-library/jest-dom/vitest";
import { describe, it, expect, afterEach, beforeEach, vi } from "vitest";
import { cleanup, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { MemoryRouter, Route, Routes } from "react-router-dom";
import type { PropsWithChildren } from "react";
import { Toaster } from "sonner";
import { UserListPage } from "./UserListPage";

type StubResponse = {
  match: (url: string, init?: RequestInit) => boolean;
  response: () => Response;
};

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

function jsonapi(body: unknown, status = 200): Response {
  return new Response(JSON.stringify(body), {
    status,
    headers: { "Content-Type": "application/vnd.api+json" },
  });
}

function usersListStub(): StubResponse {
  return {
    match: (u, init) =>
      u.includes("/api/v1/users") && (init?.method ?? "GET") === "GET" && !/users\/[^?]+/.test(u),
    response: () =>
      jsonapi({
        data: [
          {
            type: "users",
            id: "alice",
            attributes: {
              access_key: "alice",
              status: "enabled",
              attached_templates: [{ name: "readwrite", params: { bucket: "photos" } }],
              other_policies: [],
            },
          },
          {
            type: "users",
            id: "bob",
            attributes: {
              access_key: "bob",
              status: "disabled",
              attached_templates: [{ name: "readonly", params: { bucket: "logs" } }],
              other_policies: ["legacy-policy"],
            },
          },
          {
            type: "users",
            id: "carol",
            attributes: {
              access_key: "carol",
              status: "enabled",
              attached_templates: [],
              other_policies: [],
            },
          },
        ],
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
      <MemoryRouter initialEntries={["/users"]}>
        <Routes>
          <Route path="/users" element={children} />
          <Route path="/users/:accessKey" element={<div>User detail screen</div>} />
        </Routes>
        <Toaster />
      </MemoryRouter>
    </QueryClientProvider>
  );
}

describe("UserListPage", () => {
  beforeEach(() => {
    vi.unstubAllGlobals();
    document.cookie = "harbormaster_csrf=test-token; Path=/";
  });

  afterEach(() => {
    cleanup();
  });

  it("renders a row per user with access key, status, and template chips", async () => {
    installFetch([usersListStub()]);
    const qc = makeQueryClient();
    render(
      <Wrapper qc={qc}>
        <UserListPage />
      </Wrapper>,
    );

    await waitFor(() => {
      expect(screen.getByRole("button", { name: "alice" })).toBeInTheDocument();
    });
    expect(screen.getByRole("button", { name: "bob" })).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "carol" })).toBeInTheDocument();

    // Status badges
    expect(screen.getAllByText(/^Enabled$/)).toHaveLength(2);
    expect(screen.getByText(/^Disabled$/)).toBeInTheDocument();

    // Template chips render with params
    expect(screen.getByText(/readwrite \(bucket=photos\)/)).toBeInTheDocument();
    expect(screen.getByText(/readonly \(bucket=logs\)/)).toBeInTheDocument();
  });

  it("search input filters rows by access key", async () => {
    installFetch([usersListStub()]);
    const user = userEvent.setup();
    const qc = makeQueryClient();
    render(
      <Wrapper qc={qc}>
        <UserListPage />
      </Wrapper>,
    );

    await waitFor(() => {
      expect(screen.getByRole("button", { name: "alice" })).toBeInTheDocument();
    });

    await user.type(screen.getByLabelText(/search users/i), "ali");

    await waitFor(() => {
      expect(screen.queryByRole("button", { name: "bob" })).not.toBeInTheDocument();
    });
    expect(screen.getByRole("button", { name: "alice" })).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "carol" })).not.toBeInTheDocument();
  });
});
