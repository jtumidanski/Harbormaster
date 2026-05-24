import { useCallback, useRef, useState } from "react";
import { AppError, parseErrorResponse } from "@/lib/api/errors";
import { readCsrfCookie } from "@/lib/api/csrf";

export type EmptyDoneState = { deletedTotal: number; durationMs: number };

export type ParseHandlers = {
  onProgress: (deleted: number) => void;
  onDone: (state: EmptyDoneState) => void;
  onError: (message: string) => void;
  onActivity: () => void;
};

/**
 * Parse SSE frames from a single decoded text chunk plus any leftover from the
 * previous read. Returns the new leftover buffer.
 *
 * Exported for unit testing.
 */
export function parseSseChunk(chunk: string, leftover: string, handlers: ParseHandlers): string {
  const buf = leftover + chunk;
  const frames = buf.split("\n\n");
  const tail = frames.pop() ?? "";
  for (const frame of frames) {
    const trimmed = frame.trim();
    if (!trimmed) continue;
    if (trimmed.startsWith(":")) continue;
    handlers.onActivity();
    const lines = frame.split("\n");
    const event = lines
      .find((l) => l.startsWith("event:"))
      ?.slice(6)
      .trim();
    const data = lines
      .find((l) => l.startsWith("data:"))
      ?.slice(5)
      .trim();
    if (!event || !data) continue;
    let parsed: unknown;
    try {
      parsed = JSON.parse(data);
    } catch {
      continue;
    }
    if (event === "progress") {
      const deleted = (parsed as { deleted?: unknown }).deleted;
      if (typeof deleted === "number") handlers.onProgress(deleted);
    } else if (event === "done") {
      const p = parsed as { deleted_total?: unknown; duration_ms?: unknown };
      handlers.onDone({
        deletedTotal: typeof p.deleted_total === "number" ? p.deleted_total : 0,
        durationMs: typeof p.duration_ms === "number" ? p.duration_ms : 0,
      });
    } else if (event === "error") {
      const message = (parsed as { message?: unknown }).message;
      handlers.onError(typeof message === "string" ? message : "Unknown error");
    }
  }
  return tail;
}

/**
 * Drive the SSE stream from a ReadableStream reader. Extracted so tests can
 * provide a synthetic stream without going through `fetch`.
 */
export async function consumeEmptyBucketStream(
  reader: ReadableStreamDefaultReader<Uint8Array>,
  handlers: ParseHandlers,
): Promise<void> {
  const dec = new TextDecoder();
  let leftover = "";
  while (true) {
    const { done: end, value } = await reader.read();
    if (end) break;
    const text = dec.decode(value, { stream: true });
    leftover = parseSseChunk(text, leftover, handlers);
  }
}

export type UseEmptyBucketResult = {
  start: (confirmName: string, purgeVersions: boolean) => Promise<void>;
  reset: () => void;
  progress: number;
  done: EmptyDoneState | null;
  errorMsg: string | null;
  stalled: boolean;
  isRunning: boolean;
};

const STALL_THRESHOLD_MS = 30_000;
const STALL_POLL_MS = 5_000;

export function useEmptyBucket(name: string): UseEmptyBucketResult {
  const [progress, setProgress] = useState(0);
  const [done, setDone] = useState<EmptyDoneState | null>(null);
  const [errorMsg, setErrorMsg] = useState<string | null>(null);
  const [stalled, setStalled] = useState(false);
  const [isRunning, setIsRunning] = useState(false);
  const lastActivityRef = useRef<number>(0);

  const reset = useCallback(() => {
    setProgress(0);
    setDone(null);
    setErrorMsg(null);
    setStalled(false);
    setIsRunning(false);
  }, []);

  const start = useCallback(
    async (confirmName: string, purgeVersions: boolean) => {
      setProgress(0);
      setDone(null);
      setErrorMsg(null);
      setStalled(false);
      setIsRunning(true);
      lastActivityRef.current = Date.now();
      const stallTimer = setInterval(() => {
        if (Date.now() - lastActivityRef.current > STALL_THRESHOLD_MS) {
          setStalled(true);
        }
      }, STALL_POLL_MS);
      try {
        const res = await fetch(`/api/v1/buckets/${encodeURIComponent(name)}/empty`, {
          method: "POST",
          credentials: "include",
          headers: {
            "Content-Type": "application/json",
            "X-CSRF-Token": readCsrfCookie(),
            Accept: "text/event-stream",
          },
          body: JSON.stringify({ confirm_name: confirmName, purge_versions: purgeVersions }),
        });
        if (!res.ok) {
          const err = await parseErrorResponse(res);
          throw err;
        }
        const body = res.body;
        if (!body) {
          throw new AppError({
            status: 500,
            code: "no_stream",
            message: "Server did not return a stream.",
          });
        }
        const reader = body.getReader();
        await consumeEmptyBucketStream(reader, {
          onProgress: (n) => setProgress(n),
          onDone: (s) => setDone(s),
          onError: (m) => setErrorMsg(m),
          onActivity: () => {
            lastActivityRef.current = Date.now();
            setStalled(false);
          },
        });
      } catch (err) {
        if (err instanceof AppError) {
          setErrorMsg(err.message);
        } else if (err instanceof Error) {
          setErrorMsg(err.message);
        } else {
          setErrorMsg("Stream failed.");
        }
      } finally {
        clearInterval(stallTimer);
        setIsRunning(false);
      }
    },
    [name],
  );

  return { start, reset, progress, done, errorMsg, stalled, isRunning };
}
