import * as React from 'react';
import { Navigate, Route, Routes } from 'react-router-dom';
import { AuthProvider } from './auth/AuthProvider.jsx';
import RequireAuth from './auth/RequireAuth.jsx';
import RequireRole from './auth/RequireRole.jsx';
import { ROLE_ADMIN } from './auth/sessionModel.js';
import AdminLayout from './layout/AdminLayout.jsx';
import DashboardPage from './pages/DashboardPage.jsx';
import ForbiddenPage from './pages/ForbiddenPage.jsx';
import LoginPage from './pages/LoginPage.jsx';
import LegacyVoiceChatPage from './pages/LegacyVoiceChatPage.jsx';
import NotFoundPage from './pages/NotFoundPage.jsx';
import PlatformResourcesPage from './pages/PlatformResourcesPage.jsx';
import RegisterPage from './pages/RegisterPage.jsx';
import ToolMarketPage from './pages/ToolMarketPage.jsx';
import VoicebotDevicePage from './pages/VoicebotDevicePage.jsx';

export default function App() {
  return (
    <AuthProvider>
      <Routes>
        <Route path="/login" element={<LoginPage />} />
        <Route path="/register" element={<RegisterPage />} />
        <Route path="/forbidden" element={<ForbiddenPage />} />
        <Route path="/voice-chat" element={<LegacyVoiceChatPage />} />

        <Route element={<RequireAuth />}>
          <Route element={<AdminLayout />}>
            <Route index element={<Navigate to="/dashboard" replace />} />
            <Route path="/dashboard" element={<DashboardPage />} />
            <Route path="/tool-market" element={<ToolMarketPage />} />
            <Route path="/voicebots-devices" element={<VoicebotDevicePage />} />

            <Route element={<RequireRole roles={[ROLE_ADMIN]} />}>
              <Route
                path="/platform-resources"
                element={<PlatformResourcesPage />}
              />
            </Route>
          </Route>
        </Route>

        <Route path="*" element={<NotFoundPage />} />
      </Routes>
    </AuthProvider>
  );
}
