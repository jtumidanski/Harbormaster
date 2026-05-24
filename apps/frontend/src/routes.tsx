import { Navigate, Route, Routes } from "react-router-dom";
import { useAuth } from "@/context/AuthContext";
import { useSetupStatus } from "@/lib/hooks/api/useSetupStatus";
import { AppShell } from "@/components/AppShell";
import { LoginPage } from "@/features/auth/LoginPage";
import { ChangePasswordPage } from "@/features/auth/ChangePasswordPage";
import { SetupWizard } from "@/features/setup/SetupWizard";
import { BucketsPlaceholder } from "@/features/buckets/BucketsPlaceholder";

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
        <Route path="/buckets" element={<BucketsPlaceholder />} />
        <Route path="/settings/account" element={<ChangePasswordPage />} />
        <Route path="*" element={<div className="p-8">Not found</div>} />
      </Route>
    </Routes>
  );
}
