import { Skeleton } from "@/components/ui/skeleton";

/**
 * TableSkeleton renders a placeholder grid while a list query is loading.
 * Per the frontend guidelines we use skeletons rather than spinners in
 * content areas.
 */
export function TableSkeleton({ rows = 6, columns = 4 }: { rows?: number; columns?: number }) {
  return (
    <div className="rounded-md border p-4" aria-hidden="true">
      <div className="space-y-3">
        {Array.from({ length: rows }).map((_, r) => (
          <div key={r} className="flex gap-4">
            {Array.from({ length: columns }).map((_, c) => (
              <Skeleton key={c} className="h-5 flex-1" />
            ))}
          </div>
        ))}
      </div>
    </div>
  );
}
