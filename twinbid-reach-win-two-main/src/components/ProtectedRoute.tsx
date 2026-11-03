import { useAuth } from "@/contexts/AuthContext";
import { useRole, type UserRole } from "@/contexts/RoleContext";
import { Navigate } from "react-router-dom";

export function ProtectedRoute({ children, requireRole }: { children: React.ReactNode; requireRole?: UserRole }) {
  const { user, loading } = useAuth();
  const { role } = useRole();

  if (loading) {
    return (
      <div className="min-h-screen flex items-center justify-center bg-background">
        <div className="animate-spin rounded-full h-8 w-8 border-b-2 border-primary" />
      </div>
    );
  }

  if (!user) {
    return <Navigate to={requireRole === "publisher" ? "/publisher" : "/"} replace />;
  }

  if (requireRole && role !== requireRole) {
    return <Navigate to={role === "publisher" ? "/publisher/overview" : role === "advertiser" ? "/dashboard" : "/"} replace />;
  }

  return <>{children}</>;
}
