import { useEffect, useMemo, useRef, useState } from 'react'
import { useSearchParams } from 'react-router-dom'
import { Circle, CircleDot, CircleAlert, CheckCircle2, CircleMinus, type LucideIcon } from 'lucide-react'
import { type Effort, type Project, type Priority } from '../lib/api'
import { type AreaWithProjects, type PinnedTaskWithContext } from '../lib/protograph'
import { useAreasQuery, useProjectPriorityMutation } from '../state/areas'
import { usePinnedTasksQuery, useTaskPriorityMutation } from '../state/tasks'
import { useVimBindings } from '../lib/vim'
import ProjectDialog from '../components/ProjectDialog'
import TaskDialog from '../components/TaskDialog'

// ─── types ───────────────────────────────────────────────────────────────────

interface ProjectWithArea extends Project {
  areaTitle?: string
  areaColor?: string
}

// ─── priority ─────────────────────────────────────────────────────────────────

const PRIORITY_NUM: Record<string, number> = {
  PRIORITY_UNSPECIFIED: 4,
  PRIORITY_1: 1,
  PRIORITY_2: 2,
  PRIORITY_3: 3,
  PRIORITY_4: 4,
  PRIORITY_5: 5,
}

const PRIORITY_LABEL: Record<string, string> = {
  PRIORITY_UNSPECIFIED: 'P4',
  PRIORITY_1: 'P1',
  PRIORITY_2: 'P2',
  PRIORITY_3: 'P3',
  PRIORITY_4: 'P4',
  PRIORITY_5: 'P5',
}

const PRIORITY_CLASS: Record<string, string> = {
  PRIORITY_1: 'bg-destructive/15 text-destructive',
  PRIORITY_2: 'bg-orange-500/15 text-orange-500',
  PRIORITY_3: 'bg-yellow-500/15 text-yellow-500',
  PRIORITY_4: 'bg-muted text-muted-foreground',
  PRIORITY_5: 'bg-muted text-muted-foreground/60',
}

function priorityNum(p?: Priority): number {
  return PRIORITY_NUM[p ?? 'PRIORITY_UNSPECIFIED'] ?? 4
}

function sortByPriority(projects: ProjectWithArea[]): ProjectWithArea[] {
  return [...projects].sort((a, b) => priorityNum(a.priority) - priorityNum(b.priority))
}

const TASK_STATUS_GROUP_ORDER = [
  'TASK_STATUS_IN_PROGRESS',
  'TASK_STATUS_TODO',
  'TASK_STATUS_BLOCKED',
  'TASK_STATUS_DONE',
  'TASK_STATUS_UNSPECIFIED',
]

const TASK_STATUS_LABEL: Record<string, string> = {
  TASK_STATUS_IN_PROGRESS: 'In Progress',
  TASK_STATUS_TODO:        'Todo',
  TASK_STATUS_BLOCKED:     'Blocked',
  TASK_STATUS_DONE:        'Done',
}

function groupOrder(status?: string): number {
  return TASK_STATUS_GROUP_ORDER.indexOf(status ?? 'TASK_STATUS_UNSPECIFIED')
}

function sortAndGroupTasks(tasks: PinnedTaskWithContext[]): PinnedTaskWithContext[] {
  return [...tasks].sort((a, b) => {
    const gs = groupOrder(a.status) - groupOrder(b.status)
    if (gs !== 0) return gs
    return priorityNum(a.priority) - priorityNum(b.priority)
  })
}

// ─── status icons ─────────────────────────────────────────────────────────────

const PROJECT_STATUS_ICON: Record<string, { icon: LucideIcon; className: string }> = {
  PROJECT_STATUS_UNSPECIFIED: { icon: Circle,        className: 'text-muted-foreground' },
  PROJECT_STATUS_BACKLOG:     { icon: Circle,        className: 'text-muted-foreground' },
  PROJECT_STATUS_ACTIVE:      { icon: CircleDot,     className: 'text-blue-500' },
  PROJECT_STATUS_BLOCKED:     { icon: CircleAlert,   className: 'text-destructive' },
  PROJECT_STATUS_COMPLETED:   { icon: CheckCircle2,  className: 'text-green-500' },
  PROJECT_STATUS_ABANDONED:   { icon: CircleMinus,   className: 'text-muted-foreground' },
}

const TASK_STATUS_ICON: Record<string, { icon: LucideIcon; className: string }> = {
  TASK_STATUS_UNSPECIFIED: { icon: Circle,       className: 'text-muted-foreground' },
  TASK_STATUS_TODO:        { icon: Circle,       className: 'text-muted-foreground' },
  TASK_STATUS_IN_PROGRESS: { icon: CircleDot,    className: 'text-blue-500' },
  TASK_STATUS_DONE:        { icon: CheckCircle2, className: 'text-green-500' },
}

// ─── components ──────────────────────────────────────────────────────────────

function formatEffort(effort: Effort): string {
  const unit = effort.unit === 'EFFORT_UNIT_DAYS' ? 'day' : effort.unit === 'EFFORT_UNIT_WEEKS' ? 'week' : 'month'
  return `~${effort.value} ${unit}${effort.value !== 1 ? 's' : ''}`
}

function ProjectCard({
  project,
  selected,
  onClick,
  refProp,
}: {
  project: ProjectWithArea
  selected: boolean
  onClick: () => void
  refProp?: (el: HTMLDivElement | null) => void
}) {
  return (
    <div
      ref={refProp}
      onClick={onClick}
      className={`flex items-start gap-2.5 px-3 py-2.5 rounded-lg border transition-colors cursor-pointer ${
        selected ? 'border-primary bg-accent' : 'border-border hover:bg-accent'
      }`}
    >
      {(() => { const s = PROJECT_STATUS_ICON[project.status ?? 'PROJECT_STATUS_UNSPECIFIED'] ?? PROJECT_STATUS_ICON.PROJECT_STATUS_UNSPECIFIED; const Icon = s.icon; return <Icon size={13} className={`${s.className} shrink-0 mt-0.5`} /> })()}
      <div className="flex flex-col gap-1.5 min-w-0">
        <span className="text-sm leading-snug">{project.title}</span>
        <div className="flex items-center gap-1.5 flex-wrap">
          {project.areaTitle && (
            <span
              className="text-[10px] font-medium px-1.5 py-0.5 rounded-full"
              style={project.areaColor ? {
                backgroundColor: project.areaColor + '22',
                color: project.areaColor,
              } : undefined}
            >
              {project.areaTitle}
            </span>
          )}
          <span className={`text-[10px] font-medium px-1.5 py-0.5 rounded-full ${PRIORITY_CLASS[project.priority ?? 'PRIORITY_UNSPECIFIED'] ?? PRIORITY_CLASS.PRIORITY_4}`}>
            {PRIORITY_LABEL[project.priority ?? 'PRIORITY_UNSPECIFIED'] ?? 'P4'}
          </span>
          {project.estimatedEffort && project.estimatedEffort.value > 0 && (
            <span className="text-[10px] font-medium px-1.5 py-0.5 rounded-full bg-muted text-muted-foreground">
              {formatEffort(project.estimatedEffort)}
            </span>
          )}
        </div>
      </div>
    </div>
  )
}

function PinnedTaskCard({
  task,
  selected,
  onClick,
  refProp,
}: {
  task: PinnedTaskWithContext
  selected: boolean
  onClick: () => void
  refProp?: (el: HTMLDivElement | null) => void
}) {
  const s = TASK_STATUS_ICON[task.status ?? 'TASK_STATUS_UNSPECIFIED'] ?? TASK_STATUS_ICON.TASK_STATUS_TODO
  const Icon = s.icon
  return (
    <div
      ref={refProp}
      onClick={onClick}
      className={`flex items-start gap-2.5 px-3 py-2.5 rounded-lg border transition-colors cursor-pointer ${
        selected ? 'border-primary bg-accent' : 'border-border hover:bg-accent'
      }`}
    >
      <Icon size={13} className={`${s.className} shrink-0 mt-0.5`} />
      <div className="flex flex-col gap-1 min-w-0">
        <span className={`text-sm leading-snug${task.status === 'TASK_STATUS_DONE' ? ' line-through text-muted-foreground' : ''}`}>
          {task.title}
        </span>
        <div className="flex items-center gap-1.5 flex-wrap">
          <span className={`text-[10px] font-medium px-1.5 py-0.5 rounded-full ${PRIORITY_CLASS[task.priority ?? 'PRIORITY_UNSPECIFIED'] ?? PRIORITY_CLASS.PRIORITY_4}`}>
            {PRIORITY_LABEL[task.priority ?? 'PRIORITY_UNSPECIFIED'] ?? 'P4'}
          </span>
          {task.breadcrumb.length > 0 && (
            <span className="text-[10px] text-muted-foreground truncate">
              {task.breadcrumb.join(' › ')}
            </span>
          )}
        </div>
      </div>
    </div>
  )
}

// ─── page ─────────────────────────────────────────────────────────────────────

export default function HomePage() {
  const [selectedAreaIds, setSelectedAreaIds] = useState<Set<string>>(new Set())
  const [activeSection, setActiveSection] = useState<'tasks' | 'projects'>('tasks')
  const [selectedIdx, setSelectedIdx] = useState(0)
  const [searchParams, setSearchParams] = useSearchParams()
  const openProjectId = searchParams.get('project')
  const openTaskId = searchParams.get('task')
  const itemRefs = useRef<Map<string, HTMLDivElement | null>>(new Map())

  function setOpenProjectId(id: string | null) {
    setSearchParams(id ? { project: id } : {}, { replace: true })
  }

  function setOpenTaskId(id: string | null) {
    setSearchParams(id ? { task: id } : {}, { replace: true })
  }

  const areasQuery = useAreasQuery()
  const pinnedQuery = usePinnedTasksQuery()

  const areas: AreaWithProjects[] = areasQuery.data ?? []
  const pinnedTasks: PinnedTaskWithContext[] = pinnedQuery.data ?? []
  const loading = areasQuery.isLoading || pinnedQuery.isLoading
  const error = areasQuery.error ?? pinnedQuery.error

  const taskPriorityMutation = useTaskPriorityMutation()
  const projectPriorityMutation = useProjectPriorityMutation()

  const activeProjects = useMemo<ProjectWithArea[]>(() => {
    const all = areas.flatMap(area =>
      (area.projects ?? [])
        .filter(p => p.status === 'PROJECT_STATUS_ACTIVE')
        .map(p => ({ ...p, areaTitle: area.title, areaColor: area.color }))
    )
    const filtered = selectedAreaIds.size > 0
      ? all.filter(p => selectedAreaIds.has(p.areaId ?? ''))
      : all
    return sortByPriority(filtered)
  }, [areas, selectedAreaIds])

  const filteredPinnedTasks = useMemo(() => {
    const filtered = selectedAreaIds.size > 0
      ? pinnedTasks.filter(t => selectedAreaIds.has(t.areaId ?? ''))
      : pinnedTasks
    return sortAndGroupTasks(filtered)
  }, [pinnedTasks, selectedAreaIds])

  const sectionItems = activeSection === 'tasks' ? filteredPinnedTasks : activeProjects

  // Clamp cursor when list shrinks
  useEffect(() => {
    setSelectedIdx(i => Math.min(i, Math.max(0, sectionItems.length - 1)))
  }, [sectionItems.length])

  useEffect(() => {
    const id = activeSection === 'tasks'
      ? filteredPinnedTasks[selectedIdx]?.id
      : activeProjects[selectedIdx]?.id
    if (id) itemRefs.current.get(id)?.scrollIntoView({ block: 'nearest' })
  }, [activeSection, filteredPinnedTasks, activeProjects, selectedIdx])

  // ─── mutations ──────────────────────────────────────────────────────────────

  function setSelectedPriority(p: Priority) {
    if (activeSection === 'tasks') {
      const task = filteredPinnedTasks[selectedIdx]
      if (task) taskPriorityMutation.mutate({ id: task.id, priority: p })
    } else {
      const project = activeProjects[selectedIdx]
      if (project) projectPriorityMutation.mutate({ id: project.id, priority: p })
    }
  }

  // ─── vim bindings ─────────────────────────────────────────────────────────

  const bindings = useMemo(() => [
    {
      key: 'h',
      description: 'Switch to pinned tasks',
      handler: () => {
        setActiveSection('tasks')
        setSelectedIdx(i => Math.min(i, Math.max(0, filteredPinnedTasks.length - 1)))
      },
    },
    {
      key: 'l',
      description: 'Switch to active projects',
      handler: () => {
        setActiveSection('projects')
        setSelectedIdx(i => Math.min(i, Math.max(0, activeProjects.length - 1)))
      },
    },
    {
      key: 'j',
      description: 'Select next item',
      handler: () => setSelectedIdx(i => Math.min(i + 1, sectionItems.length - 1)),
    },
    {
      key: 'k',
      description: 'Select previous item',
      handler: () => setSelectedIdx(i => Math.max(i - 1, 0)),
    },
    {
      key: 'gg',
      description: 'Go to first item',
      handler: () => setSelectedIdx(0),
    },
    {
      key: 'G',
      description: 'Go to last item',
      handler: () => setSelectedIdx(Math.max(0, sectionItems.length - 1)),
    },
    {
      key: 'Enter',
      description: 'Open selected item',
      handler: () => {
        if (activeSection === 'projects') {
          const project = activeProjects[selectedIdx]
          if (project) setOpenProjectId(project.id)
        } else {
          const task = filteredPinnedTasks[selectedIdx]
          if (task) setOpenTaskId(task.id)
        }
      },
    },
    { key: 'p1', description: 'Set priority P1', handler: () => setSelectedPriority('PRIORITY_1') },
    { key: 'p2', description: 'Set priority P2', handler: () => setSelectedPriority('PRIORITY_2') },
    { key: 'p3', description: 'Set priority P3', handler: () => setSelectedPriority('PRIORITY_3') },
    { key: 'p4', description: 'Set priority P4', handler: () => setSelectedPriority('PRIORITY_4') },
    { key: 'p5', description: 'Set priority P5', handler: () => setSelectedPriority('PRIORITY_5') },
  ], [activeSection, filteredPinnedTasks, activeProjects, sectionItems, selectedIdx])

  useVimBindings(bindings)

  if (loading) return <div className="p-6 text-muted-foreground">Loading…</div>
  if (error)   return <div className="p-6 text-destructive">Error: {String(error)}</div>

  return (
    <>
      {openProjectId && (
        <ProjectDialog
          projectId={openProjectId}
          onClose={() => setOpenProjectId(null)}
        />
      )}
      {openTaskId && (() => {
        const ctx = filteredPinnedTasks.find(t => t.id === openTaskId)
        return (
          <TaskDialog
            taskId={openTaskId}
            breadcrumb={ctx?.breadcrumb}
            onClose={() => setOpenTaskId(null)}
          />
        )
      })()}
      <div className="p-6 h-full flex flex-col gap-6">
        {/* Header with area filters */}
        <div className="flex items-center gap-3">
          <h1 className="text-2xl font-semibold">Home</h1>
          <div className="flex items-center gap-1.5">
            <button
              onClick={() => { setSelectedAreaIds(new Set()); setSelectedIdx(0) }}
              className={`px-2.5 py-0.5 rounded-full text-xs font-medium transition-colors cursor-pointer ${
                selectedAreaIds.size === 0
                  ? 'bg-foreground text-background'
                  : 'bg-muted text-muted-foreground hover:bg-accent hover:text-accent-foreground'
              }`}
            >
              All
            </button>
            {areas.map(area => {
              const active = selectedAreaIds.has(area.id)
              function toggle() {
                setSelectedAreaIds(prev => {
                  const next = new Set(prev)
                  if (next.has(area.id)) next.delete(area.id)
                  else next.add(area.id)
                  return next
                })
                setSelectedIdx(0)
              }
              return (
                <button
                  key={area.id}
                  onClick={toggle}
                  className="px-2.5 py-0.5 rounded-full text-xs font-medium transition-colors cursor-pointer"
                  style={area.color
                    ? active
                      ? { backgroundColor: area.color, color: '#fff' }
                      : { backgroundColor: area.color + '22', color: area.color }
                    : undefined
                  }
                >
                  {area.title}
                </button>
              )
            })}
          </div>
        </div>

        {/* Two-column layout: pinned tasks (primary) + active projects (sidebar) */}
        <div className="flex gap-8 flex-1 items-start overflow-y-auto">
          {/* Pinned tasks — primary column */}
          <section className="flex-1 min-w-0">
            <div className="flex items-center gap-2 mb-3">
              <h2 className="text-xs font-semibold uppercase tracking-wider text-muted-foreground">
                Pinned Tasks
              </h2>
              {filteredPinnedTasks.length > 0 && (
                <span className="text-xs text-muted-foreground bg-muted rounded px-1.5 py-0.5">
                  {filteredPinnedTasks.length}
                </span>
              )}
            </div>
            {filteredPinnedTasks.length === 0 ? (
              <p className="text-sm text-muted-foreground/60 px-1">No pinned tasks.</p>
            ) : (
              <div className="space-y-4">
                {TASK_STATUS_GROUP_ORDER.map(status => {
                  const group = filteredPinnedTasks.filter(t => (t.status ?? 'TASK_STATUS_UNSPECIFIED') === status)
                  if (group.length === 0) return null
                  const label = TASK_STATUS_LABEL[status]
                  const { icon: GroupIcon, className: iconClass } = TASK_STATUS_ICON[status] ?? TASK_STATUS_ICON.TASK_STATUS_TODO
                  return (
                    <div key={status}>
                      <div className="flex items-center gap-1.5 mb-1.5">
                        <GroupIcon size={11} className={iconClass} />
                        <span className="text-[10px] font-semibold uppercase tracking-wider text-muted-foreground">{label}</span>
                      </div>
                      <div className="space-y-1.5">
                        {group.map(task => {
                          const i = filteredPinnedTasks.indexOf(task)
                          return (
                            <PinnedTaskCard
                              key={task.id}
                              task={task}
                              selected={activeSection === 'tasks' && selectedIdx === i}
                              onClick={() => { setActiveSection('tasks'); setSelectedIdx(i) }}
                              refProp={el => { itemRefs.current.set(task.id, el) }}
                            />
                          )
                        })}
                      </div>
                    </div>
                  )
                })}
              </div>
            )}
          </section>

          {/* Active projects — sidebar */}
          <section className="w-72 shrink-0">
            <div className="flex items-center gap-2 mb-3">
              <h2 className="text-xs font-semibold uppercase tracking-wider text-muted-foreground">
                Active Projects
              </h2>
              {activeProjects.length > 0 && (
                <span className="text-xs text-muted-foreground bg-muted rounded px-1.5 py-0.5">
                  {activeProjects.length}
                </span>
              )}
            </div>
            {activeProjects.length === 0 ? (
              <p className="text-sm text-muted-foreground/60 px-1">
                No active projects — press <kbd className="px-1 py-0.5 rounded bg-muted text-foreground font-mono text-xs">+p</kbd> to create one.
              </p>
            ) : (
              <div className="space-y-1.5">
                {activeProjects.map((project, i) => (
                  <ProjectCard
                    key={project.id}
                    project={project}
                    selected={activeSection === 'projects' && selectedIdx === i}
                    onClick={() => { setActiveSection('projects'); setSelectedIdx(i) }}
                    refProp={el => { itemRefs.current.set(project.id, el) }}
                  />
                ))}
              </div>
            )}
          </section>
        </div>
      </div>
    </>
  )
}
