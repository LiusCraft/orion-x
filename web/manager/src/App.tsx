import { BrowserRouter, Routes, Route, Navigate } from 'react-router-dom'
import { useAuthStore } from '@/lib/store'
import LoginPage from '@/pages/LoginPage'
import VoicebotListPage from '@/pages/VoicebotListPage'
import VoicebotDetailPage from '@/pages/VoicebotDetailPage'
import Layout from '@/components/Layout'

import AgentPlazaPage from '@/pages/agents/AgentPlazaPage'
import AgentListPage from '@/pages/agents/AgentListPage'
import AgentDetailPage from '@/pages/agents/AgentDetailPage'

import McpPage from '@/pages/components/McpPage'
import PluginsPage from '@/pages/components/PluginsPage'
import SkillsPage from '@/pages/components/SkillsPage'

import MemoryPage from '@/pages/data/MemoryPage'
import KnowledgePage from '@/pages/data/KnowledgePage'
import SourcesPage from '@/pages/data/SourcesPage'

import VoicePlazaPage from '@/pages/voice/VoicePlazaPage'
import VoiceClonePage from '@/pages/voice/VoiceClonePage'
import VoiceListPage from '@/pages/voice/VoiceListPage'

import ModelMonitorPage from '@/pages/models/ModelMonitorPage'
import MyModelsPage from '@/pages/models/MyModelsPage'
import ProvidersPage from '@/pages/models/ProvidersPage'

import UsagePage from '@/pages/billing/UsagePage'
import ResourcesPage from '@/pages/billing/ResourcesPage'

import AccountPage from '@/pages/AccountPage'
import ApiKeysPage from '@/pages/ApiKeysPage'

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

        {/* Legacy voicebot routes */}
        <Route path="/voicebots" element={<RequireAuth><VoicebotListPage /></RequireAuth>} />
        <Route path="/voicebots/:id" element={<RequireAuth><VoicebotDetailPage /></RequireAuth>} />

        {/* Main manager routes (all inside Layout) */}
        <Route element={<RequireAuth><Layout /></RequireAuth>}>
          {/* Agents */}
          <Route path="/agents/plaza" element={<AgentPlazaPage />} />
          <Route path="/agents" element={<AgentListPage />} />
          <Route path="/agents/:id" element={<AgentDetailPage />} />

          {/* Components */}
          <Route path="/components/mcp" element={<McpPage />} />
          <Route path="/components/plugins" element={<PluginsPage />} />
          <Route path="/components/skills" element={<SkillsPage />} />

          {/* Data */}
          <Route path="/data/memory" element={<MemoryPage />} />
          <Route path="/data/knowledge" element={<KnowledgePage />} />
          <Route path="/data/sources" element={<SourcesPage />} />

          {/* Voice */}
          <Route path="/voice/plaza" element={<VoicePlazaPage />} />
          <Route path="/voice/clone" element={<VoiceClonePage />} />
          <Route path="/voice" element={<VoiceListPage />} />

          {/* Models */}
          <Route path="/models/providers" element={<ProvidersPage />} />
          <Route path="/models/monitor" element={<ModelMonitorPage />} />
          <Route path="/models" element={<MyModelsPage />} />

          {/* Billing */}
          <Route path="/billing/usage" element={<UsagePage />} />
          <Route path="/billing/resources" element={<ResourcesPage />} />

          {/* Account & Keys */}
          <Route path="/account" element={<AccountPage />} />
          <Route path="/apikeys" element={<ApiKeysPage />} />
        </Route>

        <Route path="*" element={<Navigate to="/agents" replace />} />
      </Routes>
    </BrowserRouter>
  )
}
