import { useEffect, useMemo, useRef, useState } from 'react'
import { FolderKanban } from 'lucide-react'
import { projects, type Project } from '../lib/api'
import { useVimBindings } from '../lib/vim'

export default function ProjectsPage() {
  const [projectList, setProjectList] = useState<Project[]>([])
  const [error, setError] = useState<string | null>(null)
  const [loading, setLoading] = useState(true)
  const [selectedIndex, setSelectedIndex] = useState(0)
  const itemRefs = useRef<(HTMLLIElement | null)[]>([])

  useEffect(() => {
    projects.list()
      .then(res => setProjectList(res.projects ?? []))
      .catch(err => setError(String(err)))
      .finally(() => setLoading(false))
  }, [])

  // Keep selected item scrolled into view
  useEffect(() => {
    itemRefs.current[selectedIndex]?.scrollIntoView({ block: 'nearest' })
  }, [selectedIndex])

  const bindings = useMemo(() => [
    {
      key: 'j',
      description: 'Move selection down',
      handler: () => setSelectedIndex(i => Math.min(i + 1, projectList.length - 1)),
    },
    {
      key: 'k',
      description: 'Move selection up',
      handler: () => setSelectedIndex(i => Math.max(i - 1, 0)),
    },
    {
      key: 'gg',
      description: 'Go to first project',
      handler: () => setSelectedIndex(0),
    },
    {
      key: 'G',
      description: 'Go to last project',
      handler: () => setSelectedIndex(Math.max(0, projectList.length - 1)),
    },
    {
      key: 'Enter',
      description: 'Open selected project',
      handler: () => {
        const project = projectList[selectedIndex]
        if (project) console.log('open project', project.id)
      },
    },
  ], [projectList, selectedIndex])

  useVimBindings(bindings)

  if (loading) return <div className="p-6 text-muted-foreground">Loading…</div>
  if (error) return <div className="p-6 text-destructive">Error: {error}</div>

  return (
    <div className="p-6 max-w-3xl">
      <h1 className="text-2xl font-semibold mb-6">Projects</h1>
      {projectList.length === 0 ? (
        <p className="text-muted-foreground">No projects yet.</p>
      ) : (
        <ul className="space-y-2">
          {projectList.map((project, i) => (
            <li
              key={project.id}
              ref={el => { itemRefs.current[i] = el }}
              onClick={() => setSelectedIndex(i)}
              className={`flex items-center gap-3 px-4 py-3 rounded-lg border transition-colors cursor-pointer ${
                i === selectedIndex
                  ? 'border-primary bg-accent'
                  : 'border-border hover:bg-accent'
              }`}
            >
              <FolderKanban size={16} className="text-muted-foreground shrink-0" />
              <span className="font-medium">{project.title}</span>
            </li>
          ))}
        </ul>
      )}
    </div>
  )
}
