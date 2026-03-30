import { Link, useLocation } from 'react-router-dom'
import { Home, FolderKanban } from 'lucide-react'
import { cn } from '../lib/utils'
import { useVimStatus } from '../lib/vim'

export default function Nav() {
  const { pathname } = useLocation()
  const { mode, sequence } = useVimStatus()

  return (
    <nav className="flex items-center gap-1 border-b border-border px-4 py-2 bg-card text-card-foreground">
      <Link
        to="/"
        className={cn(
          'flex items-center gap-2 px-3 py-1.5 rounded-md text-sm font-medium hover:bg-accent transition-colors',
          pathname === '/' && 'bg-accent text-accent-foreground',
        )}
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
      <div className="ml-auto flex items-center gap-3 font-mono text-xs">
        <span className="text-muted-foreground min-w-[4ch] text-right">{sequence}</span>
        <span className="px-1.5 py-0.5 rounded bg-muted text-muted-foreground">{mode}</span>
      </div>
    </nav>
  )
}
