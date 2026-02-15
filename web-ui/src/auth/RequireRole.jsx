import { Navigate, Outlet } from 'react-router-dom';
import { normalizeRole } from './sessionModel.js';
import { useAuth } from './AuthProvider.jsx';

export default function RequireRole({ roles }) {
  const { user } = useAuth();
  const currentRole = normalizeRole(user?.role);

  if (!roles.includes(currentRole)) {
    return <Navigate to="/forbidden" replace />;
  }

  return <Outlet />;
}
