import { BrowserRouter, Route, Routes } from 'react-router-dom'
import { ToastProvider } from './components/ui'
import AppShell from './layouts/AppShell'
import Avisos from './pages/Avisos'
import Gestores from './pages/Gestores'
import Login from './pages/Login'
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
            <Route path="gestores" element={<Gestores />} />
            <Route path="avisos" element={<Avisos />} />
            <Route path="whatsapp" element={<WhatsApp />} />
          </Route>
        </Routes>
      </BrowserRouter>
    </ToastProvider>
  )
}
