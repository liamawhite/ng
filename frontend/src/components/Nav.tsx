import { Home, FolderKanban } from 'lucide-react'

export default function Nav() {
  return (
    <nav className="flex items-center gap-6 border-b border-border px-6 py-3 bg-card text-card-foreground">
      <a href="/" className="flex items-center gap-2 text-sm font-medium hover:text-primary transition-colors">
        <Home size={18} />
      </a>
      <a href="/projects" className="flex items-center gap-2 text-sm font-medium hover:text-primary transition-colors">
        <FolderKanban size={18} />
        Projects
      </a>
    </nav>
  )
}
