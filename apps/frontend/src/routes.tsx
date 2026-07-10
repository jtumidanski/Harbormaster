import { Navigate, Route, Routes, useLocation } from "react-router-dom";
import { useAuth } from "@/context/AuthContext";
import { useSetupStatus } from "@/lib/hooks/api/useSetupStatus";
import { AppShell } from "@/components/AppShell";
import { LoginPage } from "@/features/auth/LoginPage";
import { ChangePasswordPage } from "@/features/auth/ChangePasswordPage";
import { SetupWizard } from "@/features/setup/SetupWizard";
import { BucketListPage } from "@/features/buckets/BucketListPage";
import { BucketDetailPage } from "@/features/buckets/BucketDetailPage";
import { ConnectionSettingsPage } from "@/features/connection/ConnectionSettingsPage";
import { UserListPage } from "@/features/users/UserListPage";
import { UserDetailPage } from "@/features/users/UserDetailPage";
import { PoliciesPage } from "@/features/policies/PoliciesPage";
import { DashboardPage } from "@/features/dashboard/DashboardPage";
import { ActivityFeedPage } from "@/features/activity/ActivityFeedPage";
import { MetricsPage } from "@/features/metrics/MetricsPage";

function RedirectToLogin() {
  const location = useLocation();
  // Remember where the user was headed so RedirectAfterLogin can send them back.
  const from = location.pathname + location.search;
  return <Navigate to="/login" replace state={{ from }} />;
}

function safeReturnPath(state: unknown): string {
  const from = (state as { from?: unknown } | null)?.from;
  // Only follow same-app paths; anything else falls back to the default.
  if (typeof from === "string" && from.startsWith("/") && !from.startsWith("//")) {
    return from;
  }
  return "/buckets";
}

// Rendered when an authenticated user sits on /login (i.e. right after a
// successful sign-in flipped the gate). Declarative on purpose: an imperative
// navigate() from LoginPage races the gate re-render — the still-mounted
// unauthenticated routes bounce the target straight back to /login.
function RedirectAfterLogin() {
  const location = useLocation();
  return <Navigate to={safeReturnPath(location.state)} replace />;
}

export function AppRoutes() {
  const { me, isLoading: meLoading } = useAuth();
  const status = useSetupStatus();
  if (status.isLoading || meLoading) return <div className="p-8">Loading…</div>;
  if (!status.data?.initialized) {
    return (
      <Routes>
        <Route path="/setup" element={<SetupWizard />} />
        <Route path="*" element={<Navigate to="/setup" replace />} />
      </Routes>
    );
  }
  if (!me) {
    return (
      <Routes>
        <Route path="/login" element={<LoginPage />} />
        <Route path="*" element={<RedirectToLogin />} />
      </Routes>
    );
  }
  return (
    <Routes>
      <Route path="/login" element={<RedirectAfterLogin />} />
      <Route element={<AppShell />}>
        <Route path="/" element={<Navigate to="/dashboard" replace />} />
        <Route path="/dashboard" element={<DashboardPage />} />
        <Route path="/metrics" element={<MetricsPage />} />
        <Route path="/buckets" element={<BucketListPage />} />
        <Route path="/buckets/:name" element={<BucketDetailPage />} />
        <Route path="/users" element={<UserListPage />} />
        <Route path="/users/:accessKey" element={<UserDetailPage />} />
        <Route path="/policies" element={<PoliciesPage />} />
        <Route path="/activity" element={<ActivityFeedPage />} />
        <Route path="/settings/account" element={<ChangePasswordPage />} />
        <Route path="/settings/connection" element={<ConnectionSettingsPage />} />
        <Route path="*" element={<div className="p-8">Not found</div>} />
      </Route>
    </Routes>
  );
}
