import { useActiveVimBindings, useVimStatus, type VimBinding } from '../lib/vim'

const KEY_LABELS: Record<string, string> = {
  ArrowUp:    '↑',
  ArrowDown:  '↓',
  ArrowLeft:  '←',
  ArrowRight: '→',
  Enter:      '↵',
  Escape:     'Esc',
}

function formatKey(key: string): string {
  return KEY_LABELS[key] ?? key
}

interface MergedBinding {
  keys: string[]
  description: string
}

function mergeBindings(bindings: VimBinding[]): MergedBinding[] {
  const byDescription = new Map<string, string[]>()
  for (const b of bindings) {
    const existing = byDescription.get(b.description)
    if (existing) {
      existing.push(b.key)
    } else {
      byDescription.set(b.description, [b.key])
    }
  }
  return [...byDescription.entries()]
    .map(([description, keys]) => ({ keys, description }))
    .sort((a, b) => a.description.localeCompare(b.description))
}

export default function WhichKey() {
  const bindings = useActiveVimBindings()
  const { sequence } = useVimStatus()

  const visible = sequence
    ? bindings.filter(b => b.key.startsWith(sequence) && b.key !== sequence)
    : bindings

  const merged = mergeBindings(visible)

  if (merged.length === 0) return null

  return (
    <div className="border-t border-border bg-card px-4 py-2 grid grid-cols-[repeat(auto-fill,minmax(14rem,1fr))] gap-x-4 gap-y-1 font-mono text-xs">
      {merged.map(({ keys, description }) => (
        <span key={description} className="flex items-center gap-1.5 text-muted-foreground">
          <span className="flex items-center gap-0.5 shrink-0">
            {keys.map((key, i) => (
              <span key={key} className="flex items-center gap-0.5">
                {i > 0 && <span className="text-muted-foreground/50">/</span>}
                <kbd className="px-1 py-0.5 rounded bg-muted text-foreground font-mono">
                  {formatKey(sequence ? key.slice(sequence.length) : key)}
                </kbd>
              </span>
            ))}
          </span>
          <span className="truncate">{description}</span>
        </span>
      ))}
    </div>
  )
}
