import { Link, useLocation } from 'react-router-dom'
import { Home, FolderKanban } from 'lucide-react'
import { cn } from '../lib/utils'

export default function Nav() {
  const { pathname } = useLocation()

  return (
    <nav className="flex items-center gap-1 border-b border-border px-4 py-2 bg-card text-card-foreground">
      <Link
        to="/"
        className="flex items-center gap-2 px-3 py-1.5 rounded-md text-sm font-medium hover:bg-accent transition-colors"
      >
        <Home size={16} />
      </Link>
      <Link
        to="/projects"
        className={cn(
          'flex items-center gap-2 px-3 py-1.5 rounded-md text-sm font-medium hover:bg-accent transition-colors',
          pathname.startsWith('/projects') && 'bg-accent text-accent-foreground',
        )}
      >
        <FolderKanban size={16} />
        Projects
      </Link>
    </nav>
  )
}
