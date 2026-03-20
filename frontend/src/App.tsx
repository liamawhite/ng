import { useEffect, useState } from 'react'
import { projects, type Project } from './lib/api'

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

  if (loading) return <p>Loading…</p>
  if (error) return <p style={{ color: 'red' }}>Error: {error}</p>

  return (
    <main style={{ fontFamily: 'sans-serif', padding: '2rem' }}>
      <h1>ng</h1>
      <h2>Projects</h2>
      {items.length === 0 ? (
        <p>No projects yet.</p>
      ) : (
        <ul>
          {items.map(p => (
            <li key={p.id}>{p.title}</li>
          ))}
        </ul>
      )}
    </main>
  )
}
