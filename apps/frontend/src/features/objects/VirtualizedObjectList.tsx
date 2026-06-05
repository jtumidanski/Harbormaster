import { useRef } from "react";
import type { UIEvent } from "react";
import { useVirtualizer } from "@tanstack/react-virtual";
import { Download, Folder, History, Link as LinkIcon, Trash2 } from "lucide-react";
import { Button } from "@/components/ui/button";
import type { ObjectListItem } from "./types";

// Row layout constants. ESTIMATED_ROW_HEIGHT keeps the virtualizer
// honest before measurements come in; 36px is a tight admin-table-style
// row with room for icon + key + actions.
const ESTIMATED_ROW_HEIGHT = 36;

// AUTO_LOAD_THRESHOLD is the fraction of the scrollable area that, once
// scrolled past, kicks off the next page fetch. 0.9 matches the plan's
// "trigger near the bottom but not at the very bottom" behaviour so the
// next page is in-flight by the time the user reaches it.
const AUTO_LOAD_THRESHOLD = 0.9;

export type VirtualizedObjectListProps = {
  items: ObjectListItem[];
  hasNextPage: boolean;
  isFetchingNextPage: boolean;
  fetchNextPage: () => void;
  onOpenPrefix: (prefix: string) => void;
  onDownload: (key: string) => void;
  onDelete: (key: string) => void;
  onShare: (key: string) => void;
  onPreview: (key: string, contentType: string, size: number) => void;
  onVersions: (key: string) => void;
};

function formatBytes(bytes: number): string {
  if (!Number.isFinite(bytes) || bytes <= 0) return "0 B";
  const units = ["B", "KiB", "MiB", "GiB", "TiB", "PiB"];
  let i = 0;
  let n = bytes;
  while (n >= 1024 && i < units.length - 1) {
    n /= 1024;
    i++;
  }
  return `${n >= 10 || i === 0 ? n.toFixed(0) : n.toFixed(1)} ${units[i]}`;
}

function lastSegment(prefix: string): string {
  const trimmed = prefix.endsWith("/") ? prefix.slice(0, -1) : prefix;
  const i = trimmed.lastIndexOf("/");
  return i >= 0 ? trimmed.slice(i + 1) : trimmed;
}

function keyTail(key: string): string {
  const i = key.lastIndexOf("/");
  return i >= 0 ? key.slice(i + 1) : key;
}

export function VirtualizedObjectList({
  items,
  hasNextPage,
  isFetchingNextPage,
  fetchNextPage,
  onOpenPrefix,
  onDownload,
  onDelete,
  onShare,
  onPreview,
  onVersions,
}: VirtualizedObjectListProps) {
  // fetchingRef provides a stricter "one outstanding request" guarantee
  // than `isFetchingNextPage` alone: it flips on the moment we decide to
  // fetch and only flips off in `.finally()`, so back-to-back scroll
  // events inside the same React render cannot double-trigger.
  const fetchingRef = useRef(false);
  const scrollRef = useRef<HTMLDivElement | null>(null);

  const rowVirtualizer = useVirtualizer({
    count: items.length,
    getScrollElement: () => scrollRef.current,
    estimateSize: () => ESTIMATED_ROW_HEIGHT,
    overscan: 8,
  });

  const triggerFetch = () => {
    if (fetchingRef.current || isFetchingNextPage || !hasNextPage) return;
    fetchingRef.current = true;
    try {
      const ret: unknown = fetchNextPage();
      if (ret && typeof ret === "object" && "finally" in ret) {
        void (ret as Promise<unknown>).finally(() => {
          fetchingRef.current = false;
        });
      } else {
        fetchingRef.current = false;
      }
    } catch {
      fetchingRef.current = false;
    }
  };

  const onScroll = (e: UIEvent<HTMLDivElement>) => {
    const el = e.currentTarget;
    if (el.scrollHeight <= 0) return;
    const ratio = (el.scrollTop + el.clientHeight) / el.scrollHeight;
    if (ratio >= AUTO_LOAD_THRESHOLD) triggerFetch();
  };

  const virtualItems = rowVirtualizer.getVirtualItems();
  const totalSize = rowVirtualizer.getTotalSize();

  return (
    <div className="space-y-2">
      <div
        ref={scrollRef}
        onScroll={onScroll}
        className="h-[480px] w-full overflow-auto rounded border bg-background"
        data-testid="object-list-scroller"
      >
        {items.length === 0 ? (
          <div className="p-6 text-center text-sm text-muted-foreground">
            No objects in this folder.
          </div>
        ) : (
          <div style={{ height: totalSize, position: "relative" }}>
            {virtualItems.map((v) => {
              const item = items[v.index];
              if (!item) return null;
              const rowStyle = { top: v.start, height: v.size };
              if (item.type === "object_prefixes") {
                const p = item.attributes;
                return (
                  <div
                    key={`${item.type}:${item.id}`}
                    data-testid="object-row"
                    className="absolute left-0 right-0 flex items-center gap-3 border-b px-3 text-sm hover:bg-accent/40"
                    style={rowStyle}
                  >
                    <button
                      type="button"
                      className="flex flex-1 items-center gap-2 truncate text-left text-primary hover:underline"
                      onClick={() => onOpenPrefix(p.prefix)}
                    >
                      <Folder className="h-4 w-4 shrink-0" aria-hidden="true" />
                      <span className="truncate">{lastSegment(p.prefix)}/</span>
                    </button>
                  </div>
                );
              }
              const e = item.attributes;
              return (
                <div
                  key={`${item.type}:${item.id}`}
                  data-testid="object-row"
                  className="absolute left-0 right-0 flex items-center gap-3 border-b px-3 text-sm hover:bg-accent/40"
                  style={rowStyle}
                >
                  <button
                    type="button"
                    className="flex flex-1 items-center gap-2 truncate text-left hover:underline"
                    onClick={() => onPreview(e.key, e.content_type, e.size)}
                    title={e.key}
                  >
                    <span className="truncate">{keyTail(e.key)}</span>
                  </button>
                  <span className="w-24 shrink-0 text-right text-xs text-muted-foreground">
                    {formatBytes(e.size)}
                  </span>
                  <div className="flex shrink-0 items-center gap-1">
                    <Button
                      type="button"
                      variant="ghost"
                      size="sm"
                      aria-label={`Download ${e.key}`}
                      onClick={() => onDownload(e.key)}
                    >
                      <Download className="h-4 w-4" aria-hidden="true" />
                    </Button>
                    <Button
                      type="button"
                      variant="ghost"
                      size="sm"
                      aria-label={`Share ${e.key}`}
                      onClick={() => onShare(e.key)}
                    >
                      <LinkIcon className="h-4 w-4" aria-hidden="true" />
                    </Button>
                    <Button
                      type="button"
                      variant="ghost"
                      size="sm"
                      aria-label={`Versions of ${e.key}`}
                      onClick={() => onVersions(e.key)}
                    >
                      <History className="h-4 w-4" aria-hidden="true" />
                    </Button>
                    <Button
                      type="button"
                      variant="ghost"
                      size="sm"
                      aria-label={`Delete ${e.key}`}
                      onClick={() => onDelete(e.key)}
                    >
                      <Trash2 className="h-4 w-4" aria-hidden="true" />
                    </Button>
                  </div>
                </div>
              );
            })}
          </div>
        )}
      </div>

      <div className="flex items-center justify-between text-xs text-muted-foreground">
        <span>{items.length.toLocaleString()} item(s) loaded</span>
        <Button
          type="button"
          variant="outline"
          size="sm"
          onClick={triggerFetch}
          disabled={!hasNextPage || isFetchingNextPage || fetchingRef.current}
        >
          {isFetchingNextPage ? "Loading…" : hasNextPage ? "Load more" : "All loaded"}
        </Button>
      </div>
    </div>
  );
}
