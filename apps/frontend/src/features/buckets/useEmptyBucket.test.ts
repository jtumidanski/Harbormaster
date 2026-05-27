import { describe, it, expect, vi } from "vitest";
import { consumeEmptyBucketStream, parseSseChunk, type ParseHandlers } from "./useEmptyBucket";

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

describe("BucketDetailPage tests use parser indirectly", () => {
  it("vi is wired (sanity)", () => {
    const fn = vi.fn();
    fn();
    expect(fn).toHaveBeenCalled();
  });
});
