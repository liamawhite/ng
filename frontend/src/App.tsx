import { useEffect, useState } from 'react'
import { projects, type Project } from './lib/api'
import Nav from './components/Nav'

export default function App() {
  const [items, setItems] = useState<Project[]>([])
  const [error, setError] = useState<string | null>(null)
  const [loading, setLoading] = useState(true)

  useEffect(() => {
    projects.list()
      .then(res => setItems(res.projects ?? []))
      .catch(err => setError(String(err)))
      .finally(() => setLoading(false))
  }, [])

  return (
    <div className="min-h-screen bg-background text-foreground">
      <Nav />
      <main className="p-8">
        <h1 className="text-2xl font-bold mb-6">Projects</h1>
        {loading && <p className="text-muted-foreground">Loading…</p>}
        {error && <p className="text-destructive">Error: {error}</p>}
        {!loading && !error && items.length === 0 && (
          <p className="text-muted-foreground">No projects yet.</p>
        )}
        {!loading && !error && items.length > 0 && (
          <ul className="space-y-2">
            {items.map(p => (
              <li key={p.id} className="text-sm">{p.title}</li>
            ))}
          </ul>
        )}
      </main>
    </div>
  )
}
