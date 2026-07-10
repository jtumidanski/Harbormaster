import { describe, it, expect, afterEach, vi } from "vitest";
import { act, renderHook, waitFor } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { createElement, type PropsWithChildren } from "react";
import { authKeys } from "@/lib/api/keys";
import {
  consumeEmptyBucketStream,
  parseSseChunk,
  useEmptyBucket,
  type ParseHandlers,
} from "./useEmptyBucket";

function makeHandlers(): {
  handlers: ParseHandlers;
  progress: number[];
  done: Array<{ deletedTotal: number; durationMs: number }>;
  errors: string[];
  activity: number;
} {
  const progress: number[] = [];
  const done: Array<{ deletedTotal: number; durationMs: number }> = [];
  const errors: string[] = [];
  let activity = 0;
  return {
    progress,
    done,
    errors,
    get activity() {
      return activity;
    },
    handlers: {
      onProgress: (n) => progress.push(n),
      onDone: (s) => done.push(s),
      onError: (m) => errors.push(m),
      onActivity: () => {
        activity++;
      },
    },
  };
}

function streamOf(frames: string[]): ReadableStreamDefaultReader<Uint8Array> {
  const enc = new TextEncoder();
  const stream = new ReadableStream<Uint8Array>({
    start(controller) {
      for (const f of frames) controller.enqueue(enc.encode(f));
      controller.close();
    },
  });
  return stream.getReader();
}

describe("parseSseChunk", () => {
  it("parses a progress frame", () => {
    const h = makeHandlers();
    const leftover = parseSseChunk(`event: progress\ndata: {"deleted":100}\n\n`, "", h.handlers);
    expect(leftover).toBe("");
    expect(h.progress).toEqual([100]);
    expect(h.activity).toBe(1);
  });

  it("parses a done frame and populates terminal state", () => {
    const h = makeHandlers();
    parseSseChunk(
      `event: done\ndata: {"deleted_total":500,"duration_ms":1234}\n\n`,
      "",
      h.handlers,
    );
    expect(h.done).toEqual([{ deletedTotal: 500, durationMs: 1234 }]);
  });

  it("parses an error frame", () => {
    const h = makeHandlers();
    parseSseChunk(`event: error\ndata: {"message":"x"}\n\n`, "", h.handlers);
    expect(h.errors).toEqual(["x"]);
  });

  it("ignores comment-only keepalive frames", () => {
    const h = makeHandlers();
    parseSseChunk(`: keepalive\n\n`, "", h.handlers);
    expect(h.progress).toEqual([]);
    expect(h.done).toEqual([]);
    expect(h.errors).toEqual([]);
    expect(h.activity).toBe(0);
  });

  it("buffers split frames across chunks", () => {
    const h = makeHandlers();
    let leftover = parseSseChunk(`event: progress\ndata: {"deleted":`, "", h.handlers);
    expect(leftover).toContain("event: progress");
    expect(h.progress).toEqual([]);
    leftover = parseSseChunk(`42}\n\n`, leftover, h.handlers);
    expect(leftover).toBe("");
    expect(h.progress).toEqual([42]);
  });
});

describe("consumeEmptyBucketStream", () => {
  it("processes a sequence of progress + done frames from a ReadableStream", async () => {
    const h = makeHandlers();
    const reader = streamOf([
      `event: progress\ndata: {"deleted":10}\n\n`,
      `: keepalive\n\n`,
      `event: progress\ndata: {"deleted":20}\n\n`,
      `event: done\ndata: {"deleted_total":20,"duration_ms":500}\n\n`,
    ]);
    await consumeEmptyBucketStream(reader, h.handlers);
    expect(h.progress).toEqual([10, 20]);
    expect(h.done).toEqual([{ deletedTotal: 20, durationMs: 500 }]);
    expect(h.errors).toEqual([]);
  });

  it("handles error frames", async () => {
    const h = makeHandlers();
    const reader = streamOf([`event: error\ndata: {"message":"boom"}\n\n`]);
    await consumeEmptyBucketStream(reader, h.handlers);
    expect(h.errors).toEqual(["boom"]);
  });

  it("calls onActivity once per non-comment frame", async () => {
    const h = makeHandlers();
    const reader = streamOf([
      `event: progress\ndata: {"deleted":1}\n\n`,
      `: keepalive\n\n`,
      `event: progress\ndata: {"deleted":2}\n\n`,
    ]);
    await consumeEmptyBucketStream(reader, h.handlers);
    expect(h.activity).toBe(2);
  });
});

describe("useEmptyBucket session expiry", () => {
  afterEach(() => {
    vi.unstubAllGlobals();
  });

  it("hands a 401 unauthenticated response to the global handler, no local error", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn(() =>
        Promise.resolve(
          new Response(
            JSON.stringify({
              errors: [{ code: "unauthenticated", detail: "Authentication required." }],
            }),
            { status: 401, headers: { "Content-Type": "application/vnd.api+json" } },
          ),
        ),
      ),
    );
    // Default gcTime: with gcTime 0 the observer-less seeded auth.me query
    // would be garbage-collected out from under the expiry handler.
    const qc = new QueryClient({
      defaultOptions: { queries: { retry: false, staleTime: 0 } },
    });
    qc.setQueryData(authKeys.me(), {
      username: "alice",
      session_expires_at: "2030-01-01T00:00:00Z",
    });
    const wrapper = ({ children }: PropsWithChildren) =>
      createElement(QueryClientProvider, { client: qc }, children);

    const { result } = renderHook(() => useEmptyBucket("b1"), { wrapper });
    await act(async () => {
      await result.current.start("b1", false);
    });

    // The raw-fetch SSE channel must reach the global expiry handler (route
    // gate flips to /login) instead of stranding the user on a local error.
    await waitFor(() => expect(qc.getQueryData(authKeys.me())).toBeNull());
    expect(result.current.errorMsg).toBeNull();
  });
});

describe("BucketDetailPage tests use parser indirectly", () => {
  it("vi is wired (sanity)", () => {
    const fn = vi.fn();
    fn();
    expect(fn).toHaveBeenCalled();
  });
});
