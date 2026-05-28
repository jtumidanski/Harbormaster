import { useMemo, useState } from "react";
import { useNavigate } from "react-router-dom";
import { useQuery } from "@tanstack/react-query";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table";
import { TableSkeleton } from "@/components/common/TableSkeleton";
import { AppError } from "@/lib/api/errors";
import { usersKeys } from "@/lib/api/keys";
import { listUsers } from "./api";
import type { TemplateRef, User, UserStatus } from "./types";
import { CreateUserDialog } from "./CreateUserDialog";

function statusBadgeClass(status: UserStatus): string {
  return status === "enabled"
    ? "bg-emerald-100 text-emerald-900 dark:bg-emerald-900/30 dark:text-emerald-200"
    : "bg-muted text-muted-foreground";
}

function TemplateChip({ tpl }: { tpl: TemplateRef }) {
  const params =
    tpl.params && Object.keys(tpl.params).length > 0
      ? ` (${Object.entries(tpl.params)
          .map(([k, v]) => `${k}=${v}`)
          .join(", ")})`
      : "";
  return (
    <Badge variant="outline" className="font-mono font-normal">
      {tpl.name}
      {params}
    </Badge>
  );
}

export function UserListPage() {
  const navigate = useNavigate();
  const [createOpen, setCreateOpen] = useState(false);
  const [search, setSearch] = useState("");

  const q = useQuery({
    queryKey: usersKeys.list(),
    queryFn: listUsers,
  });

  const users: User[] = useMemo(() => q.data ?? [], [q.data]);
  const filtered = useMemo(() => {
    const needle = search.trim().toLowerCase();
    if (!needle) return users;
    return users.filter((u) => u.access_key.toLowerCase().includes(needle));
  }, [users, search]);

  return (
    <div className="p-6">
      <div className="mb-4 flex items-center justify-between gap-3">
        <h1 className="text-2xl font-semibold">Users</h1>
        <Button onClick={() => setCreateOpen(true)}>New user</Button>
      </div>

      <div className="mb-3">
        <Input
          aria-label="Search users"
          placeholder="Search access keys…"
          value={search}
          onChange={(e) => setSearch(e.target.value)}
          className="max-w-sm"
        />
      </div>

      {q.isLoading ? (
        <TableSkeleton columns={3} />
      ) : q.isError ? (
        <p className="text-destructive">
          {q.error instanceof AppError ? q.error.message : "Failed to load users."}
        </p>
      ) : users.length === 0 ? (
        <div className="rounded-md border p-8 text-center text-muted-foreground">
          No users yet — create one to get started.
        </div>
      ) : filtered.length === 0 ? (
        <div className="rounded-md border p-8 text-center text-muted-foreground">
          No users match your search.
        </div>
      ) : (
        <div className="overflow-x-auto rounded-md border">
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead>Access key</TableHead>
                <TableHead>Status</TableHead>
                <TableHead>Attached templates</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {filtered.map((u) => (
                <TableRow
                  key={u.access_key}
                  className="cursor-pointer"
                  onClick={() => navigate(`/users/${encodeURIComponent(u.access_key)}`)}
                >
                  <TableCell className="font-medium">
                    <button
                      type="button"
                      className="text-primary hover:underline"
                      onClick={(e) => {
                        e.stopPropagation();
                        navigate(`/users/${encodeURIComponent(u.access_key)}`);
                      }}
                    >
                      {u.access_key}
                    </button>
                  </TableCell>
                  <TableCell>
                    <Badge variant="outline" className={statusBadgeClass(u.status)}>
                      {u.status === "enabled" ? "Enabled" : "Disabled"}
                    </Badge>
                  </TableCell>
                  <TableCell>
                    {u.attached_templates.length === 0 ? (
                      <span className="text-xs text-muted-foreground">None</span>
                    ) : (
                      <div className="flex flex-wrap gap-1">
                        {u.attached_templates.map((t) => (
                          <TemplateChip key={t.name} tpl={t} />
                        ))}
                      </div>
                    )}
                  </TableCell>
                </TableRow>
              ))}
            </TableBody>
          </Table>
        </div>
      )}

      <CreateUserDialog open={createOpen} onOpenChange={setCreateOpen} />
    </div>
  );
}
