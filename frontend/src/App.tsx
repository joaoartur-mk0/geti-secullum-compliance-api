import { BrowserRouter, Navigate, Route, Routes } from 'react-router-dom'
import { ToastProvider } from './components/ui'
import AppShell from './layouts/AppShell'
import Auditorias from './pages/Auditorias'
import Avisos from './pages/Avisos'
import ColaboradorHistorico from './pages/ColaboradorHistorico'
import Colaboradores from './pages/Colaboradores'
import Empresa from './pages/Empresa'
import Equipamentos from './pages/Equipamentos'
import Filiais from './pages/Filiais'
import Gestores from './pages/Gestores'
import Incidentes from './pages/Incidentes'
import Indicadores from './pages/Indicadores'
import Login from './pages/Login'
import Moderacao from './pages/Moderacao'
import ReportsHistory from './pages/ReportsHistory'
import WhatsApp from './pages/WhatsApp'

export default function App() {
  return (
    <ToastProvider>
      <BrowserRouter>
        <Routes>
          <Route path="/login" element={<Login />} />
          <Route element={<AppShell />}>
            <Route index element={<Navigate to="/indicadores" replace />} />
            <Route path="auditorias" element={<Auditorias />} />
            <Route path="indicadores" element={<Indicadores />} />
            <Route path="colaboradores" element={<Colaboradores />} />
            <Route path="colaboradores/:secullumId" element={<ColaboradorHistorico />} />
            <Route path="filiais" element={<Filiais />} />
            <Route path="equipamentos" element={<Equipamentos />} />
            <Route path="gestores" element={<Gestores />} />
            <Route path="avisos" element={<Avisos />} />
            <Route path="whatsapp" element={<WhatsApp />} />
            <Route path="empresa" element={<Empresa />} />
            <Route path="reports/history" element={<ReportsHistory />} />
            <Route path="incidents" element={<Incidentes />} />
            <Route path="moderacao" element={<Moderacao />} />
          </Route>
        </Routes>
      </BrowserRouter>
    </ToastProvider>
  )
}
