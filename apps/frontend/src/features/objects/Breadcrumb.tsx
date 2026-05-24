// Breadcrumb renders the current "folder" path (the trailing-slash
// prefix encoded in the URL) as a series of clickable segments. Clicking
// a segment navigates back to a shorter prefix; clicking the root drops
// the prefix entirely. The component is deliberately presentational —
// the caller wires segment clicks through to `setPrefix` rather than
// touching the URL itself, so the same control works inside or outside
// react-router.

export type BreadcrumbProps = {
  bucket: string;
  prefix: string;
  onNavigate: (nextPrefix: string) => void;
};

type Segment = { label: string; prefix: string };

function parseSegments(prefix: string): Segment[] {
  // Drop a trailing slash so "a/b/" splits into ["a","b"] instead of
  // ["a","b",""].
  const trimmed = prefix.endsWith("/") ? prefix.slice(0, -1) : prefix;
  if (!trimmed) return [];
  const parts = trimmed.split("/");
  const out: Segment[] = [];
  let acc = "";
  for (const p of parts) {
    acc = acc ? `${acc}/${p}` : p;
    out.push({ label: p, prefix: `${acc}/` });
  }
  return out;
}

export function Breadcrumb({ bucket, prefix, onNavigate }: BreadcrumbProps) {
  const segs = parseSegments(prefix);
  return (
    <nav aria-label="Object path" className="flex flex-wrap items-center gap-1 text-sm">
      <button
        type="button"
        className="font-medium text-primary hover:underline"
        onClick={() => onNavigate("")}
      >
        {bucket}
      </button>
      {segs.map((s, i) => {
        const isLast = i === segs.length - 1;
        return (
          <span key={s.prefix} className="flex items-center gap-1">
            <span className="text-muted-foreground">/</span>
            {isLast ? (
              <span className="text-foreground">{s.label}</span>
            ) : (
              <button
                type="button"
                className="text-primary hover:underline"
                onClick={() => onNavigate(s.prefix)}
              >
                {s.label}
              </button>
            )}
          </span>
        );
      })}
    </nav>
  );
}
