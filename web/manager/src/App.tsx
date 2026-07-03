import { BrowserRouter, Routes, Route, Navigate } from 'react-router-dom'
import { useAuthStore } from '@/lib/store'
import LoginPage from '@/pages/LoginPage'
import VoicebotListPage from '@/pages/VoicebotListPage'
import VoicebotDetailPage from '@/pages/VoicebotDetailPage'

function RequireAuth({ children }: { children: React.ReactNode }) {
  const { token } = useAuthStore()
  if (!token) return <Navigate to="/login" replace />
  return <>{children}</>
}

export default function App() {
  return (
    <BrowserRouter>
      <Routes>
        <Route path="/login" element={<LoginPage />} />
        <Route
          path="/voicebots"
          element={
            <RequireAuth>
              <VoicebotListPage />
            </RequireAuth>
          }
        />
        <Route
          path="/voicebots/:id"
          element={
            <RequireAuth>
              <VoicebotDetailPage />
            </RequireAuth>
          }
        />
        <Route path="*" element={<Navigate to="/voicebots" replace />} />
      </Routes>
    </BrowserRouter>
  )
}
