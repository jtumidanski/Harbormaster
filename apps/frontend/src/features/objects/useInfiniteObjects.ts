import { useInfiniteQuery } from "@tanstack/react-query";
import { objectsKeys } from "@/lib/api/keys";
import { listObjects } from "./api";
import type { ObjectListResponse } from "./types";

// useInfiniteObjects wraps the list endpoint in a TanStack Query
// `useInfiniteQuery`. The pageParam is the opaque next_token returned by
// the previous page; "" means "first page". `getNextPageParam` returns
// `undefined` when the server omits next_token to signal exhaustion.
export function useInfiniteObjects(bucket: string, prefix: string) {
  return useInfiniteQuery({
    queryKey: objectsKeys.list(bucket, prefix),
    initialPageParam: "" as string,
    queryFn: ({ pageParam }: { pageParam: string }): Promise<ObjectListResponse> =>
      listObjects(bucket, prefix, pageParam || undefined),
    getNextPageParam: (last: ObjectListResponse): string | undefined => {
      const token = last.meta?.page?.next_token;
      return token ? token : undefined;
    },
    enabled: bucket.length > 0,
  });
}
