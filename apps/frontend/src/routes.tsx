import { Navigate, Route, Routes } from "react-router-dom";
import { useAuth } from "@/context/AuthContext";
import { useSetupStatus } from "@/lib/hooks/api/useSetupStatus";
import { AppShell } from "@/components/AppShell";
import { LoginPage } from "@/features/auth/LoginPage";
import { ChangePasswordPage } from "@/features/auth/ChangePasswordPage";
import { SetupWizard } from "@/features/setup/SetupWizard";
import { BucketListPage } from "@/features/buckets/BucketListPage";
import { BucketDetailPage } from "@/features/buckets/BucketDetailPage";
import { ConnectionSettingsPage } from "@/features/connection/ConnectionSettingsPage";

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
        <Route path="*" element={<Navigate to="/login" replace />} />
      </Routes>
    );
  }
  return (
    <Routes>
      <Route element={<AppShell />}>
        <Route path="/" element={<Navigate to="/buckets" replace />} />
        <Route path="/buckets" element={<BucketListPage />} />
        <Route path="/buckets/:name" element={<BucketDetailPage />} />
        <Route path="/settings/account" element={<ChangePasswordPage />} />
        <Route path="/settings/connection" element={<ConnectionSettingsPage />} />
        <Route path="*" element={<div className="p-8">Not found</div>} />
      </Route>
    </Routes>
  );
}
