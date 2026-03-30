import { useMemo, useRef, useState } from 'react'
import { type Project } from '../lib/api'
import { useAreasQuery, useCreateProjectMutation } from '../state/areas'
import { useVimBindings } from '../lib/vim'

interface Props {
  onClose: () => void
  onCreated: (project: Project) => void
}

export default function CreateProjectDialog({ onClose, onCreated }: Props) {
  const [title, setTitle] = useState('')
  const [areaId, setAreaId] = useState('')
  const inputRef = useRef<HTMLInputElement>(null)

  const { data: areasData } = useAreasQuery()
  const areaList = areasData ?? []

  const createMutation = useCreateProjectMutation()

  async function submit() {
    const t = title.trim()
    if (!t || createMutation.isPending) return
    const project = await createMutation.mutateAsync({ title: t, areaId: areaId || undefined })
    onCreated(project)
  }

  useVimBindings(useMemo(() => [
    { key: 'Escape', description: 'Cancel', handler: onClose },
  ], [onClose]))

  // Focus input on mount
  const setInputRef = (el: HTMLInputElement | null) => {
    (inputRef as React.MutableRefObject<HTMLInputElement | null>).current = el
    el?.focus()
  }

  return (
    <div
      className="fixed inset-0 z-50 flex items-center justify-center bg-black/50"
      onClick={onClose}
    >
      <div
        className="bg-background border border-border rounded-xl shadow-lg p-6 w-full max-w-sm mx-4"
        onClick={e => e.stopPropagation()}
      >
        <h2 className="text-base font-semibold mb-4">New project</h2>
        <div className="flex flex-col gap-3">
          <input
            ref={setInputRef}
            type="text"
            placeholder="Project title"
            value={title}
            onChange={e => setTitle(e.target.value)}
            onKeyDown={e => { if (e.key === 'Enter') { e.preventDefault(); submit() } }}
            className="w-full px-3 py-2 text-sm rounded-md border border-border bg-background focus:outline-none focus:ring-1 focus:ring-ring"
          />
          {areaList.length > 0 && (
            <select
              value={areaId}
              onChange={e => setAreaId(e.target.value)}
              className="w-full px-3 py-2 text-sm rounded-md border border-border bg-background focus:outline-none focus:ring-1 focus:ring-ring"
            >
              <option value="">No area</option>
              {areaList.map(a => (
                <option key={a.id} value={a.id}>{a.title}</option>
              ))}
            </select>
          )}
          <div className="flex gap-2 justify-end mt-1">
            <button
              onClick={onClose}
              className="px-3 py-1.5 text-sm rounded-md border border-border hover:bg-accent transition-colors"
            >
              Cancel
            </button>
            <button
              onClick={submit}
              disabled={!title.trim() || createMutation.isPending}
              className="px-3 py-1.5 text-sm rounded-md bg-primary text-primary-foreground hover:bg-primary/90 transition-colors disabled:opacity-50"
            >
              Create
            </button>
          </div>
        </div>
      </div>
    </div>
  )
}
