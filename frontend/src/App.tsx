import { useEffect, useMemo, useState } from 'react'
import { BrowserRouter, Routes, Route, useNavigate } from 'react-router-dom'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import Nav from './components/Nav'
import WhichKey from './components/WhichKey'
import HomePage from './pages/HomePage'
import ProjectsPage from './pages/ProjectsPage'
import CreateProjectDialog from './components/CreateProjectDialog'
import { VimProvider, useVimBindings } from './lib/vim'
import { Toaster } from 'sonner'
import type { Project } from './lib/api'

const queryClient = new QueryClient({
  defaultOptions: {
    queries: {
      staleTime: 30_000,
      gcTime: 5 * 60_000,
      retry: 1,
      refetchOnWindowFocus: false,
    },
  },
})

function AppLayout() {
  const [whichKeyVisible, setWhichKeyVisible] = useState(true)
  const [showCreate, setShowCreate] = useState(false)
  const navigate = useNavigate()

  useEffect(() => {
    const handler = (e: KeyboardEvent) => {
      const target = e.target
      if (target instanceof HTMLElement && (
        target.tagName === 'INPUT' || target.tagName === 'TEXTAREA' || target.contentEditable === 'true'
      )) return
      if (e.key === '?') {
        e.preventDefault()
        setWhichKeyVisible(v => !v)
      }
    }
    document.addEventListener('keydown', handler)
    return () => document.removeEventListener('keydown', handler)
  }, [])

  useVimBindings(useMemo(() => [
    { key: '+p', description: 'Create project', handler: () => setShowCreate(true) },
  ], []), { global: true })

  function handleCreated(project: Project) {
    setShowCreate(false)
    navigate(`/projects?project=${project.id}`)
  }

  return (
    <div className="h-screen flex flex-col bg-background text-foreground">
      <Nav />
      <main className="flex-1 overflow-auto">
        <Routes>
          <Route path="/" element={<HomePage />} />
          <Route path="/projects" element={<ProjectsPage />} />
        </Routes>
      </main>
      {whichKeyVisible && <WhichKey />}
      {showCreate && (
        <CreateProjectDialog
          onClose={() => setShowCreate(false)}
          onCreated={handleCreated}
        />
      )}
      <Toaster />
    </div>
  )
}

export default function App() {
  return (
    <QueryClientProvider client={queryClient}>
      <VimProvider>
        <BrowserRouter>
          <AppLayout />
        </BrowserRouter>
      </VimProvider>
    </QueryClientProvider>
  )
}
