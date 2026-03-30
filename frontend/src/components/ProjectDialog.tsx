import { useCallback, useEffect, useRef, useState } from 'react'
import { Dialog, DialogContent, DialogHeader, DialogTitle } from './ui/dialog'
import { Textarea } from './ui/textarea'
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from './ui/select'
import { type Effort, type EffortUnit, type Link, type Priority, type ProjectStatus, type TaskStatus } from '../lib/api'
import { type TaskWithSubtasks } from '../lib/protograph'
import { useAreasQuery, useProjectQuery, useProjectSaveMutation, useProjectLinksMutation } from '../state/areas'
import { useCreateTaskMutation, useTaskStatusMutation, useTaskPinnedMutation } from '../state/tasks'
import { FolderKanban, Circle, CheckCircle2, Timer, Plus, ChevronRight, ChevronDown, X, ExternalLink, Pin } from 'lucide-react'

// ─── collapsed state ──────────────────────────────────────────────────────────

const STORAGE_KEY = 'ng:task-collapsed'

function loadCollapsed(): Set<string> {
  try {
    const raw = localStorage.getItem(STORAGE_KEY)
    return new Set(raw ? JSON.parse(raw) as string[] : [])
  } catch {
    return new Set()
  }
}

function saveCollapsed(set: Set<string>) {
  localStorage.setItem(STORAGE_KEY, JSON.stringify([...set]))
}

function useCollapsedTasks() {
  const [collapsed, setCollapsed] = useState<Set<string>>(loadCollapsed)

  const toggle = useCallback((id: string) => {
    setCollapsed(prev => {
      const next = new Set(prev)
      if (next.has(id)) next.delete(id); else next.add(id)
      saveCollapsed(next)
      return next
    })
  }, [])

  return { collapsed, toggle }
}

const PROJECT_PRIORITIES: { value: Priority; label: string }[] = [
  { value: 'PRIORITY_1', label: 'P1' },
  { value: 'PRIORITY_2', label: 'P2' },
  { value: 'PRIORITY_3', label: 'P3' },
  { value: 'PRIORITY_4', label: 'P4' },
  { value: 'PRIORITY_5', label: 'P5' },
]

const EFFORT_UNITS: { value: EffortUnit; label: string }[] = [
  { value: 'EFFORT_UNIT_DAYS',   label: 'Days' },
  { value: 'EFFORT_UNIT_WEEKS',  label: 'Weeks' },
  { value: 'EFFORT_UNIT_MONTHS', label: 'Months' },
]

const PROJECT_STATUSES: { value: ProjectStatus; label: string }[] = [
  { value: 'PROJECT_STATUS_UNSPECIFIED', label: 'No Status' },
  { value: 'PROJECT_STATUS_BACKLOG',     label: 'Backlog' },
  { value: 'PROJECT_STATUS_ACTIVE',      label: 'Active' },
  { value: 'PROJECT_STATUS_BLOCKED',     label: 'Blocked' },
  { value: 'PROJECT_STATUS_COMPLETED',   label: 'Completed' },
  { value: 'PROJECT_STATUS_ABANDONED',   label: 'Abandoned' },
]

// ─── task tree helpers ────────────────────────────────────────────────────────

const STATUS_CYCLE: TaskStatus[] = ['TASK_STATUS_TODO', 'TASK_STATUS_IN_PROGRESS', 'TASK_STATUS_DONE']

function nextStatus(s: TaskStatus): TaskStatus {
  const idx = STATUS_CYCLE.indexOf(s)
  return STATUS_CYCLE[(idx + 1) % STATUS_CYCLE.length]
}

function updateStatusInTree(tasks: TaskWithSubtasks[], id: string, status: TaskStatus): TaskWithSubtasks[] {
  return tasks.map(t => ({
    ...t,
    status: t.id === id ? status : t.status,
    tasks: updateStatusInTree(t.tasks, id, status),
  }))
}

function updatePinnedInTree(tasks: TaskWithSubtasks[], id: string, pinned: boolean): TaskWithSubtasks[] {
  return tasks.map(t => ({
    ...t,
    pinned: t.id === id ? pinned : t.pinned,
    tasks: updatePinnedInTree(t.tasks, id, pinned),
  }))
}

function addTaskToTree(tasks: TaskWithSubtasks[], task: TaskWithSubtasks, parentId?: string): TaskWithSubtasks[] {
  if (!parentId) return [...tasks, task]
  return tasks.map(t => ({
    ...t,
    tasks: t.id === parentId
      ? [...t.tasks, task]
      : addTaskToTree(t.tasks, task, parentId),
  }))
}

// ─── sub-components ───────────────────────────────────────────────────────────

function TaskStatusIcon({ status }: { status: TaskStatus }) {
  if (status === 'TASK_STATUS_DONE')
    return <CheckCircle2 size={14} className="text-emerald-500 shrink-0" />
  if (status === 'TASK_STATUS_IN_PROGRESS')
    return <Timer size={14} className="text-amber-500 shrink-0" />
  return <Circle size={14} className="text-muted-foreground shrink-0" />
}

function AddTaskInput({
  depth,
  projectId,
  parentTaskId,
  onCreated,
  onCancel,
}: {
  depth: number
  projectId: string
  parentTaskId?: string
  onCreated: (task: TaskWithSubtasks) => void
  onCancel: () => void
}) {
  const [value, setValue] = useState('')
  const inputRef = useRef<HTMLInputElement>(null)
  const createTaskMutation = useCreateTaskMutation()

  useEffect(() => { inputRef.current?.focus() }, [])

  async function submit() {
    const title = value.trim()
    if (!title) { onCancel(); return }
    const created = await createTaskMutation.mutateAsync({ title, projectId, parentTaskId, status: 'TASK_STATUS_TODO' })
    onCreated({ ...created, tasks: [] })
  }

  return (
    <div className="flex items-center gap-2 py-1 pr-2" style={{ paddingLeft: `${8 + depth * 16}px` }}>
      <Circle size={14} className="text-muted-foreground/40 shrink-0" />
      <input
        ref={inputRef}
        value={value}
        onChange={e => setValue(e.target.value)}
        onKeyDown={e => {
          if (e.key === 'Enter') submit()
          if (e.key === 'Escape') onCancel()
        }}
        onBlur={submit}
        placeholder="Task title…"
        className="flex-1 bg-transparent text-sm outline-none placeholder:text-muted-foreground/50"
      />
    </div>
  )
}

function TaskTreeItem({
  task,
  depth,
  addingSubtask,
  setAddingSubtask,
  onToggle,
  onTogglePinned,
  onTaskCreated,
  projectId,
  collapsed,
  onCollapseToggle,
}: {
  task: TaskWithSubtasks
  depth: number
  addingSubtask: string | null
  setAddingSubtask: (id: string | null) => void
  onToggle: (task: TaskWithSubtasks) => void
  onTogglePinned: (task: TaskWithSubtasks) => void
  onTaskCreated: (task: TaskWithSubtasks, parentId: string) => void
  projectId: string
  collapsed: Set<string>
  onCollapseToggle: (id: string) => void
}) {
  const hasChildren = task.tasks.length > 0
  const isCollapsed = collapsed.has(task.id)

  return (
    <div>
      <div
        className="flex items-center gap-2 py-1 pr-2 rounded-md hover:bg-muted/30 group"
        style={{ paddingLeft: `${8 + depth * 16}px` }}
      >
        <button
          onClick={() => hasChildren ? onCollapseToggle(task.id) : undefined}
          className={`shrink-0 w-3.5 flex items-center justify-center ${hasChildren ? 'cursor-pointer text-muted-foreground hover:text-foreground' : 'cursor-default'}`}
          title={hasChildren ? (isCollapsed ? 'Expand' : 'Collapse') : undefined}
        >
          {hasChildren
            ? (isCollapsed ? <ChevronRight size={12} /> : <ChevronDown size={12} />)
            : <span className="w-3.5" />
          }
        </button>
        <button
          onClick={() => onToggle(task)}
          className="shrink-0 hover:scale-110 transition-transform"
          title="Toggle status"
        >
          <TaskStatusIcon status={task.status} />
        </button>
        <span className={`flex-1 text-sm leading-snug ${task.status === 'TASK_STATUS_DONE' ? 'line-through text-muted-foreground' : ''}`}>
          {task.title}
        </span>
        {hasChildren && (
          <span className="text-xs text-muted-foreground/40 group-hover:opacity-0 transition-opacity">
            {task.tasks.length}
          </span>
        )}
        <button
          onClick={() => onTogglePinned(task)}
          className={`transition-opacity text-muted-foreground hover:text-foreground ${task.pinned ? 'opacity-100 text-foreground' : 'opacity-0 group-hover:opacity-100'}`}
          title={task.pinned ? 'Unpin task' : 'Pin task'}
        >
          <Pin size={12} className={task.pinned ? 'fill-current' : ''} />
        </button>
        <button
          onClick={() => setAddingSubtask(task.id)}
          className="opacity-0 group-hover:opacity-100 transition-opacity text-muted-foreground hover:text-foreground"
          title="Add subtask"
        >
          <Plus size={12} />
        </button>
      </div>

      {!isCollapsed && (
        <>
          {task.tasks.map(child => (
            <TaskTreeItem
              key={child.id}
              task={child}
              depth={depth + 1}
              addingSubtask={addingSubtask}
              setAddingSubtask={setAddingSubtask}
              onToggle={onToggle}
              onTogglePinned={onTogglePinned}
              onTaskCreated={onTaskCreated}
              projectId={projectId}
              collapsed={collapsed}
              onCollapseToggle={onCollapseToggle}
            />
          ))}
          {addingSubtask === task.id && (
            <AddTaskInput
              depth={depth + 1}
              projectId={projectId}
              parentTaskId={task.id}
              onCreated={t => { onTaskCreated(t, task.id); setAddingSubtask(null) }}
              onCancel={() => setAddingSubtask(null)}
            />
          )}
        </>
      )}
    </div>
  )
}

// ─── dialog ───────────────────────────────────────────────────────────────────

interface Props {
  projectId: string
  onClose: () => void
}

export default function ProjectDialog({ projectId, onClose }: Props) {
  const [title, setTitle] = useState('')
  const [content, setContent] = useState('')
  const [status, setStatus] = useState<ProjectStatus>('PROJECT_STATUS_UNSPECIFIED')
  const [areaId, setAreaId] = useState<string | undefined>(undefined)
  const [completed, setCompleted] = useState<string | undefined>(undefined)
  const [priority, setPriority] = useState<Priority>('PRIORITY_4')
  const [effortValue, setEffortValue] = useState<string>('')
  const [effortUnit, setEffortUnit] = useState<EffortUnit>('EFFORT_UNIT_WEEKS')
  const [links, setLinks] = useState<Link[]>([])
  const [newLinkUrl, setNewLinkUrl] = useState('')
  const [newLinkTitle, setNewLinkTitle] = useState('')
  const [subprojects, setSubprojects] = useState<{ id: string; title: string; status?: ProjectStatus }[]>([])
  const [rootTasks, setRootTasks] = useState<TaskWithSubtasks[]>([])
  const [addingSubtask, setAddingSubtask] = useState<string | null>(null)
  const [addingRootTask, setAddingRootTask] = useState(false)
  const { collapsed, toggle: toggleCollapsed } = useCollapsedTasks()

  const debounceRef = useRef<ReturnType<typeof setTimeout> | null>(null)
  const skipSaveRef = useRef(true)
  const formRef = useRef({ title: '', content: '', status: 'PROJECT_STATUS_UNSPECIFIED' as ProjectStatus, areaId: undefined as string | undefined, priority: 'PRIORITY_4' as Priority, effortValue: '', effortUnit: 'EFFORT_UNIT_WEEKS' as EffortUnit })

  formRef.current = { title, content, status, areaId, priority, effortValue, effortUnit }

  // ─── queries ────────────────────────────────────────────────────────────────

  const { data: areasData } = useAreasQuery()
  const areas = areasData ?? []

  const { data: projectData, isLoading } = useProjectQuery(projectId)

  useEffect(() => {
    if (!projectData) return
    skipSaveRef.current = true
    setLinks(projectData.links ?? [])
    setTitle(projectData.title)
    setContent(projectData.content ?? '')
    setStatus(projectData.status ?? 'PROJECT_STATUS_UNSPECIFIED')
    setAreaId(projectData.areaId)
    setCompleted(projectData.completed)
    setPriority(projectData.priority ?? 'PRIORITY_4')
    setEffortValue(projectData.estimatedEffort?.value != null ? String(projectData.estimatedEffort.value) : '')
    setEffortUnit(projectData.estimatedEffort?.unit ?? 'EFFORT_UNIT_WEEKS')
    setSubprojects(projectData.projects)
    setRootTasks(projectData.tasks.filter(t => !t.parentTaskId))
  }, [projectData])

  // ─── mutations ──────────────────────────────────────────────────────────────

  const saveMutation = useProjectSaveMutation(projectId)
  const linksMutation = useProjectLinksMutation(projectId)
  const taskStatusMutation = useTaskStatusMutation()
  const taskPinnedMutation = useTaskPinnedMutation()

  const saveProject = useCallback(() => {
    const { title, content, status, areaId, priority, effortValue, effortUnit } = formRef.current
    const parsedEffort = parseInt(effortValue, 10)
    const estimatedEffort: Effort | null = parsedEffort > 0
      ? { value: parsedEffort, unit: effortUnit }
      : null
    saveMutation.mutate({
      title, content, status, areaId,
      parentId: projectData?.parentId,
      estimatedEffort,
      priority,
      updateMask: 'title,content,status,areaId,parentId,estimated_effort,priority',
    })
  }, [saveMutation, projectData?.parentId])

  const scheduleSave = useCallback(() => {
    if (debounceRef.current) clearTimeout(debounceRef.current)
    debounceRef.current = setTimeout(saveProject, 600)
  }, [saveProject])

  useEffect(() => {
    if (skipSaveRef.current) { skipSaveRef.current = false; return }
    scheduleSave()
  }, [title, content, status, areaId, priority, effortValue, effortUnit, scheduleSave])

  useEffect(() => {
    return () => { if (debounceRef.current) clearTimeout(debounceRef.current) }
  }, [])

  async function saveLinks(updated: Link[]) {
    await linksMutation.mutateAsync(updated)
  }

  async function addLink() {
    const url = newLinkUrl.trim()
    if (!url) return
    const updated = [...links, { url, title: newLinkTitle.trim() || undefined }]
    setLinks(updated)
    setNewLinkUrl('')
    setNewLinkTitle('')
    await saveLinks(updated)
  }

  async function removeLink(index: number) {
    const updated = links.filter((_, i) => i !== index)
    setLinks(updated)
    await saveLinks(updated)
  }

  function handleClose() {
    if (debounceRef.current) {
      clearTimeout(debounceRef.current)
      debounceRef.current = null
      saveProject()
    }
    onClose()
  }

  async function toggleTaskStatus(task: TaskWithSubtasks) {
    const next = nextStatus(task.status)
    setRootTasks(prev => updateStatusInTree(prev, task.id, next))
    try {
      await taskStatusMutation.mutateAsync({ id: task.id, status: next, projectId })
    } catch {
      setRootTasks(prev => updateStatusInTree(prev, task.id, task.status))
    }
  }

  async function toggleTaskPinned(task: TaskWithSubtasks) {
    const next = !task.pinned
    setRootTasks(prev => updatePinnedInTree(prev, task.id, next))
    try {
      await taskPinnedMutation.mutateAsync({ id: task.id, pinned: next, projectId })
    } catch {
      setRootTasks(prev => updatePinnedInTree(prev, task.id, !!task.pinned))
    }
  }

  return (
    <Dialog open onOpenChange={open => { if (!open) handleClose() }}>
      <DialogContent className="w-[90vw] max-w-none max-h-[95vh] flex flex-col overflow-y-auto">
        <DialogHeader>
          <DialogTitle>
            <div className="flex items-baseline gap-3">
              <input
                className="flex-1 bg-transparent text-lg font-semibold outline-none placeholder:text-muted-foreground"
                value={title}
                onChange={e => setTitle(e.target.value)}
                placeholder="Project title"
              />
              {completed && (
                <span className="text-xs text-muted-foreground/50 font-normal shrink-0">
                  completed {new Date(completed).toLocaleDateString(undefined, { month: 'short', day: 'numeric', year: 'numeric' })}
                </span>
              )}
            </div>
          </DialogTitle>
        </DialogHeader>

        {isLoading ? (
          <div className="flex-1 flex items-center justify-center text-muted-foreground text-sm">Loading…</div>
        ) : (
          <div className="space-y-5 pr-1">
            <div className="flex items-center gap-6">
              <div className="flex items-center gap-3">
                <span className="text-xs text-muted-foreground shrink-0">Status</span>
                <Select value={status} onValueChange={v => setStatus(v as ProjectStatus)}>
                  <SelectTrigger className="w-36 h-7 text-xs">
                    <SelectValue />
                  </SelectTrigger>
                  <SelectContent>
                    {PROJECT_STATUSES.map(s => (
                      <SelectItem key={s.value} value={s.value} className="text-xs">{s.label}</SelectItem>
                    ))}
                  </SelectContent>
                </Select>
              </div>
              <div className="flex items-center gap-3">
                <span className="text-xs text-muted-foreground shrink-0">Area</span>
                <Select value={areaId ?? '__none__'} onValueChange={v => setAreaId(v === '__none__' ? undefined : v)}>
                  <SelectTrigger className="w-36 h-7 text-xs">
                    <SelectValue />
                  </SelectTrigger>
                  <SelectContent>
                    <SelectItem value="__none__" className="text-xs">No Area</SelectItem>
                    {areas.map(a => (
                      <SelectItem key={a.id} value={a.id} className="text-xs">{a.title}</SelectItem>
                    ))}
                  </SelectContent>
                </Select>
              </div>
              <div className="flex items-center gap-3">
                <span className="text-xs text-muted-foreground shrink-0">Priority</span>
                <Select value={priority} onValueChange={v => setPriority(v as Priority)}>
                  <SelectTrigger className="w-24 h-7 text-xs">
                    <SelectValue />
                  </SelectTrigger>
                  <SelectContent>
                    {PROJECT_PRIORITIES.map(p => (
                      <SelectItem key={p.value} value={p.value} className="text-xs">{p.label}</SelectItem>
                    ))}
                  </SelectContent>
                </Select>
              </div>
              <div className="flex items-center gap-3">
                <span className="text-xs text-muted-foreground shrink-0">Effort</span>
                <input
                  type="number"
                  min={1}
                  value={effortValue}
                  onChange={e => setEffortValue(e.target.value)}
                  placeholder="—"
                  className="w-14 h-7 rounded-md border border-input bg-transparent px-2 text-xs outline-none [appearance:textfield] [&::-webkit-inner-spin-button]:appearance-none [&::-webkit-outer-spin-button]:appearance-none"
                />
                <Select value={effortUnit} onValueChange={v => setEffortUnit(v as EffortUnit)}>
                  <SelectTrigger className="w-24 h-7 text-xs">
                    <SelectValue />
                  </SelectTrigger>
                  <SelectContent>
                    {EFFORT_UNITS.map(u => (
                      <SelectItem key={u.value} value={u.value} className="text-xs">{u.label}</SelectItem>
                    ))}
                  </SelectContent>
                </Select>
              </div>
            </div>

            <div>
              <p className="text-xs text-muted-foreground mb-1.5">Links</p>
              <div className="space-y-1">
                {links.map((link, i) => (
                  <div key={i} className="flex items-center gap-2 px-2 py-1.5 rounded-md bg-muted/50 text-sm group">
                    <ExternalLink size={12} className="text-muted-foreground shrink-0" />
                    <a
                      href={link.url}
                      target="_blank"
                      rel="noopener noreferrer"
                      className="flex-1 truncate text-xs hover:underline"
                    >
                      {link.title || link.url}
                    </a>
                    <button
                      onClick={() => removeLink(i)}
                      className="opacity-0 group-hover:opacity-100 transition-opacity text-muted-foreground hover:text-foreground"
                    >
                      <X size={12} />
                    </button>
                  </div>
                ))}
                <div className="flex items-center gap-2 pt-0.5">
                  <input
                    value={newLinkUrl}
                    onChange={e => setNewLinkUrl(e.target.value)}
                    onKeyDown={e => { if (e.key === 'Enter') { void addLink() } }}
                    placeholder="URL"
                    className="flex-1 h-7 rounded-md border border-input bg-transparent px-2 text-xs outline-none placeholder:text-muted-foreground/50"
                  />
                  <input
                    value={newLinkTitle}
                    onChange={e => setNewLinkTitle(e.target.value)}
                    onKeyDown={e => { if (e.key === 'Enter') { void addLink() } }}
                    placeholder="Title (optional)"
                    className="w-36 h-7 rounded-md border border-input bg-transparent px-2 text-xs outline-none placeholder:text-muted-foreground/50"
                  />
                  <button
                    onClick={() => { void addLink() }}
                    className="flex items-center gap-1 h-7 px-2 rounded-md border border-input text-xs text-muted-foreground hover:text-foreground transition-colors"
                  >
                    <Plus size={11} />
                    Add
                  </button>
                </div>
              </div>
            </div>

            {subprojects.length > 0 && (
              <div>
                <p className="text-xs text-muted-foreground mb-1.5">Subprojects</p>
                <div className="space-y-1">
                  {subprojects.map(p => (
                    <div key={p.id} className="flex items-center gap-2 px-2 py-1.5 rounded-md bg-muted/50 text-sm">
                      <FolderKanban size={13} className="text-muted-foreground shrink-0" />
                      <span className="flex-1">{p.title}</span>
                      <span className="text-xs text-muted-foreground">
                        {PROJECT_STATUSES.find(s => s.value === p.status)?.label ?? ''}
                      </span>
                    </div>
                  ))}
                </div>
              </div>
            )}

            <div>
              <p className="text-xs text-muted-foreground mb-1.5">Tasks</p>
              <div className="rounded-md border border-border/50">
                {rootTasks.length === 0 && !addingRootTask ? (
                  <p className="px-3 py-2 text-xs text-muted-foreground/60">No tasks yet</p>
                ) : (
                  <div className="py-1">
                    {rootTasks.map(task => (
                      <TaskTreeItem
                        key={task.id}
                        task={task}
                        depth={0}
                        addingSubtask={addingSubtask}
                        setAddingSubtask={setAddingSubtask}
                        onToggle={toggleTaskStatus}
                        onTogglePinned={toggleTaskPinned}
                        onTaskCreated={(task, parentId) => {
                          setRootTasks(prev => addTaskToTree(prev, task, parentId))
                        }}
                        projectId={projectId}
                        collapsed={collapsed}
                        onCollapseToggle={toggleCollapsed}
                      />
                    ))}
                    {addingRootTask && (
                      <AddTaskInput
                        depth={0}
                        projectId={projectId}
                        onCreated={t => {
                          setRootTasks(prev => [...prev, t])
                          setAddingRootTask(false)
                        }}
                        onCancel={() => setAddingRootTask(false)}
                      />
                    )}
                  </div>
                )}
                <button
                  onClick={() => { setAddingRootTask(true); setAddingSubtask(null) }}
                  className="flex items-center gap-1.5 w-full px-2 py-1.5 text-xs text-muted-foreground/60 hover:text-muted-foreground transition-colors border-t border-border/50"
                >
                  <Plus size={11} />
                  Add task
                </button>
              </div>
            </div>

            <div>
              <p className="text-xs text-muted-foreground mb-1.5">Notes</p>
              <Textarea
                value={content}
                onChange={e => setContent(e.target.value)}
                placeholder="Add notes…"
                className="min-h-[360px] text-sm resize-none"
              />
            </div>
          </div>
        )}

      </DialogContent>
    </Dialog>
  )
}
