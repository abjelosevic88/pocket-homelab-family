import React from 'react'
import ReactDOM from 'react-dom/client'
import { BrowserRouter, Navigate, Route, Routes, useLocation } from 'react-router-dom'
import './styles/base.css'
import './styles/family.css'
import './styles/app.css'
import './styles/settings.css'
import { AppProvider, useApp } from './lib/app-context'
import { FamilyProvider } from './lib/family'
import { Loading, ToastProvider } from './components/ui'
import Layout from './components/Layout'
import { ErrorBoundary } from './components/ErrorBoundary'
import Login from './pages/Login'
import Today from './pages/Today'
import Week from './pages/Week'
import Pickups from './pages/Pickups'
import Chores from './pages/Chores'
import Shopping from './pages/Shopping'
import School, { LetterPage } from './pages/School'
import Board from './pages/Board'
import FamilyPage, { Sheet } from './pages/FamilyPage'
import Settings from './pages/Settings'

function Gate({ children }: { children: React.ReactNode }) {
  const { status, user } = useApp()
  const loc = useLocation()
  if (!status) return <div className="boot"><img src="/favicon.svg" alt="" /><Loading /></div>
  if (status.needs_setup) return <Navigate to="/setup" replace />
  if (!user) return <Navigate to="/login" replace state={{ from: loc.pathname + loc.search }} />
  return <FamilyProvider>{children}</FamilyProvider>
}

function App() {
  return (
    <Routes>
      <Route path="/login" element={<Login mode="login" />} />
      <Route path="/setup" element={<Login mode="setup" />} />
      <Route path="/settings/:tab?" element={<Gate><ErrorBoundary where="settings"><Settings /></ErrorBoundary></Gate>} />
      <Route element={<Gate><ErrorBoundary where="family"><Layout /></ErrorBoundary></Gate>}>
        <Route path="/" element={<Today />} />
        <Route path="/week" element={<Week />} />
        <Route path="/pickups" element={<Pickups />} />
        <Route path="/chores" element={<Chores />} />
        <Route path="/shopping" element={<Shopping />} />
        <Route path="/shopping/:id" element={<Shopping />} />
        <Route path="/school" element={<School />} />
        <Route path="/school/letters/:id" element={<LetterPage />} />
        <Route path="/board" element={<Board />} />
        <Route path="/family" element={<FamilyPage />} />
        <Route path="/family/sheet" element={<Sheet />} />
        <Route path="*" element={<Navigate to="/" replace />} />
      </Route>
    </Routes>
  )
}

ReactDOM.createRoot(document.getElementById('root')!).render(
  <React.StrictMode>
    <BrowserRouter>
      <ErrorBoundary where="app"><ToastProvider><AppProvider><App /></AppProvider></ToastProvider></ErrorBoundary>
    </BrowserRouter>
  </React.StrictMode>,
)
