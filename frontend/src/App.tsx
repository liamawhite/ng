import { BrowserRouter, Routes, Route, Navigate } from 'react-router-dom'
import Nav from './components/Nav'
import ProjectsPage from './pages/ProjectsPage'
import { VimProvider } from './lib/vim'

export default function App() {
  return (
    <VimProvider>
      <BrowserRouter>
        <div className="h-screen flex flex-col bg-background text-foreground">
          <Nav />
          <main className="flex-1 overflow-auto">
            <Routes>
              <Route path="/" element={<Navigate to="/projects" replace />} />
              <Route path="/projects" element={<ProjectsPage />} />
            </Routes>
          </main>
        </div>
      </BrowserRouter>
    </VimProvider>
  )
}
