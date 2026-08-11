import { BrowserRouter, Route, Routes } from 'react-router-dom'
import { ToastProvider } from './components/ui'
import AppShell from './layouts/AppShell'
import Avisos from './pages/Avisos'
import ColaboradorHistorico from './pages/ColaboradorHistorico'
import Colaboradores from './pages/Colaboradores'
import Empresa from './pages/Empresa'
import Gestores from './pages/Gestores'
import Indicadores from './pages/Indicadores'
import Login from './pages/Login'
import Moderacao from './pages/Moderacao'
import Painel from './pages/Painel'
import WhatsApp from './pages/WhatsApp'

export default function App() {
  return (
    <ToastProvider>
      <BrowserRouter>
        <Routes>
          <Route path="/login" element={<Login />} />
          <Route element={<AppShell />}>
            <Route index element={<Painel />} />
            <Route path="indicadores" element={<Indicadores />} />
            <Route path="colaboradores" element={<Colaboradores />} />
            <Route path="colaboradores/:secullumId" element={<ColaboradorHistorico />} />
            <Route path="gestores" element={<Gestores />} />
            <Route path="avisos" element={<Avisos />} />
            <Route path="whatsapp" element={<WhatsApp />} />
            <Route path="empresa" element={<Empresa />} />
            <Route path="moderacao" element={<Moderacao />} />
          </Route>
        </Routes>
      </BrowserRouter>
    </ToastProvider>
  )
}
