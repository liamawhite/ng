import { useEffect, useMemo, useRef, useState } from 'react'
import { useSearchParams } from 'react-router-dom'
import { Circle, CircleDot, CircleAlert, CheckCircle2, CircleMinus, type LucideIcon } from 'lucide-react'
import { type Effort, type Project, type Priority, type ProjectStatus } from '../lib/api'
import { type AreaWithProjects } from '../lib/protograph'
import { useAreasQuery, useProjectStatusMutation, useProjectPriorityMutation, useDeleteProjectMutation } from '../state/areas'
import { useVimBindings } from '../lib/vim'
import ProjectDialog from '../components/ProjectDialog'
import { ConfirmDialog } from '../components/ConfirmDialog'

// ─── types ───────────────────────────────────────────────────────────────────

interface ProjectWithArea extends Project {
  areaTitle?: string
  areaColor?: string
}

type ColumnId = 'backlog' | 'active' | 'blocked' | 'archived'

const MAIN_COLUMNS: { id: ColumnId; label: string }[] = [
  { id: 'backlog',  label: 'Backlog' },
  { id: 'active',   label: 'Active' },
  { id: 'blocked',  label: 'Blocked' },
  { id: 'archived', label: 'Archived' },
]

const STATUS_TO_COLUMN: Record<string, ColumnId> = {
  PROJECT_STATUS_UNSPECIFIED: 'backlog',
  PROJECT_STATUS_BACKLOG:     'backlog',
  PROJECT_STATUS_ACTIVE:      'active',
  PROJECT_STATUS_BLOCKED:     'blocked',
  PROJECT_STATUS_COMPLETED:   'archived',
  PROJECT_STATUS_ABANDONED:   'archived',
}

// Linear order used by > / < to step a project through statuses.
const STATUS_ORDER: ProjectStatus[] = [
  'PROJECT_STATUS_UNSPECIFIED',
  'PROJECT_STATUS_BACKLOG',
  'PROJECT_STATUS_ACTIVE',
  'PROJECT_STATUS_BLOCKED',
  'PROJECT_STATUS_COMPLETED',
  'PROJECT_STATUS_ABANDONED',
]

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

// ─── status icons ─────────────────────────────────────────────────────────────

const STATUS_ICON: Record<string, { icon: LucideIcon; className: string }> = {
  PROJECT_STATUS_UNSPECIFIED: { icon: Circle,        className: 'text-muted-foreground' },
  PROJECT_STATUS_BACKLOG:     { icon: Circle,        className: 'text-muted-foreground' },
  PROJECT_STATUS_ACTIVE:      { icon: CircleDot,     className: 'text-blue-500' },
  PROJECT_STATUS_BLOCKED:     { icon: CircleAlert,   className: 'text-destructive' },
  PROJECT_STATUS_COMPLETED:   { icon: CheckCircle2,  className: 'text-green-500' },
  PROJECT_STATUS_ABANDONED:   { icon: CircleMinus,   className: 'text-muted-foreground' },
}

// ─── components ──────────────────────────────────────────────────────────────

function formatEffort(effort: Effort): string {
  const unit = effort.unit === 'EFFORT_UNIT_DAYS' ? 'day' : effort.unit === 'EFFORT_UNIT_WEEKS' ? 'week' : 'month'
  return `~${effort.value} ${unit}${effort.value !== 1 ? 's' : ''}`
}

function ProjectCard({
  project,
  selected,
  showEffort,
  onClick,
  refProp,
}: {
  project: ProjectWithArea
  selected: boolean
  showEffort: boolean
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
      {(() => { const s = STATUS_ICON[project.status ?? 'PROJECT_STATUS_UNSPECIFIED'] ?? STATUS_ICON.PROJECT_STATUS_UNSPECIFIED; const Icon = s.icon; return <Icon size={13} className={`${s.className} shrink-0 mt-0.5`} /> })()}
      <div className="flex flex-col gap-1.5 min-w-0">
        <div className="flex items-baseline gap-1.5 min-w-0">
          <span className={`text-sm leading-snug${project.status === 'PROJECT_STATUS_COMPLETED' ? ' line-through text-muted-foreground' : ''}`}>{project.title}</span>
          {project.completed && (
            <span className="text-[10px] text-muted-foreground shrink-0">
              {new Date(project.completed).toLocaleDateString('en-US', { month: 'short', year: 'numeric', timeZone: 'UTC' })}
            </span>
          )}
        </div>
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
          {showEffort && project.estimatedEffort && project.estimatedEffort.value > 0 && (
            <span className="text-[10px] font-medium px-1.5 py-0.5 rounded-full bg-muted text-muted-foreground">
              {formatEffort(project.estimatedEffort)}
            </span>
          )}
        </div>
      </div>
    </div>
  )
}

function Section({
  label,
  projects,
  colId,
  indexOffset,
  selectedColId,
  selectedRowIdx,
  showEffort,
  onSelect,
  itemRefs,
}: {
  label?: string
  projects: ProjectWithArea[]
  colId: ColumnId
  indexOffset: number
  selectedColId: ColumnId
  selectedRowIdx: number
  showEffort: boolean
  onSelect: (colId: ColumnId, rowIdx: number) => void
  itemRefs: React.MutableRefObject<Map<string, HTMLDivElement | null>>
}) {
  return (
    <div className="min-h-10">
      {label && (
        <p className="text-[10px] font-semibold uppercase tracking-wider text-muted-foreground/60 mb-1.5 px-1">
          {label}
        </p>
      )}
      <div className="space-y-1.5">
        {projects.map((project, i) => (
          <ProjectCard
            key={project.id}
            project={project}
            selected={selectedColId === colId && selectedRowIdx === indexOffset + i}
            showEffort={showEffort}
            onClick={() => onSelect(colId, indexOffset + i)}
            refProp={el => { itemRefs.current.set(project.id, el) }}
          />
        ))}
      </div>
    </div>
  )
}

// ─── page ─────────────────────────────────────────────────────────────────────

export default function ProjectsPage() {
  const [selectedAreaIds, setSelectedAreaIds] = useState<Set<string>>(new Set())
  const [selectedColId, setSelectedColId] = useState<ColumnId>('backlog')
  const [selectedRowIdx, setSelectedRowIdx] = useState(0)
  const [searchParams, setSearchParams] = useSearchParams()
  const openProjectId = searchParams.get('project')
  const [confirmDelete, setConfirmDelete] = useState<ProjectWithArea | null>(null)
  const itemRefs = useRef<Map<string, HTMLDivElement | null>>(new Map())

  function setOpenProjectId(id: string | null) {
    setSearchParams(id ? { project: id } : {}, { replace: true })
  }

  // ─── query ────────────────────────────────────────────────────────────────

  const { data: areasData, isLoading, error } = useAreasQuery()
  const areas: AreaWithProjects[] = areasData ?? []

  // ─── derived state ────────────────────────────────────────────────────────

  const allProjects = useMemo<ProjectWithArea[]>(() =>
    areas.flatMap(area =>
      (area.projects ?? []).map(p => ({
        ...p,
        areaTitle: area.title,
        areaColor: area.color,
      }))
    ), [areas])

  const filteredProjects = useMemo(
    () => selectedAreaIds.size > 0 ? allProjects.filter(p => selectedAreaIds.has(p.areaId ?? '')) : allProjects,
    [allProjects, selectedAreaIds],
  )

  const columns = useMemo(() => {
    const byCol = new Map<ColumnId, ProjectWithArea[]>()
    for (const col of MAIN_COLUMNS) byCol.set(col.id, [])
    for (const p of filteredProjects) {
      const col = STATUS_TO_COLUMN[p.status ?? 'PROJECT_STATUS_UNSPECIFIED'] ?? 'backlog'
      byCol.get(col)!.push(p)
    }
    for (const [id, list] of byCol) byCol.set(id, sortByPriority(list))
    return byCol
  }, [filteredProjects])

  const archivedProjects = columns.get('archived') ?? []
  const completedProjects = archivedProjects
    .filter(p => p.status === 'PROJECT_STATUS_COMPLETED')
    .sort((a, b) => {
      if (!a.completed && !b.completed) return 0
      if (!a.completed) return 1
      if (!b.completed) return -1
      return b.completed.localeCompare(a.completed)
    })
  const abandonedProjects = archivedProjects.filter(p => p.status === 'PROJECT_STATUS_ABANDONED')

  const selectedColProjects = useMemo(
    () => selectedColId === 'archived'
      ? [...completedProjects, ...abandonedProjects]
      : (columns.get(selectedColId) ?? []),
    [columns, selectedColId, completedProjects, abandonedProjects],
  )

  // Clamp cursor when list shrinks
  useEffect(() => {
    setSelectedRowIdx(r => Math.min(r, Math.max(0, selectedColProjects.length - 1)))
  }, [selectedColProjects.length])

  useEffect(() => {
    const id = selectedColProjects[selectedRowIdx]?.id
    if (id) itemRefs.current.get(id)?.scrollIntoView({ block: 'nearest' })
  }, [selectedColProjects, selectedRowIdx])

  // ─── mutations ────────────────────────────────────────────────────────────

  const statusMutation = useProjectStatusMutation()
  const priorityMutation = useProjectPriorityMutation()
  const deleteMutation = useDeleteProjectMutation()

  // ─── actions ──────────────────────────────────────────────────────────────

  function moveProject(project: ProjectWithArea, delta: 1 | -1) {
    const idx = STATUS_ORDER.indexOf(project.status ?? 'PROJECT_STATUS_UNSPECIFIED')
    const next = STATUS_ORDER[idx + delta]
    if (!next) return
    statusMutation.mutate({ id: project.id, status: next })
  }

  function setSelectedPriority(p: Priority) {
    const project = selectedColProjects[selectedRowIdx]
    if (!project) return
    priorityMutation.mutate({ id: project.id, priority: p })
  }

  function deleteProject(project: ProjectWithArea) {
    setConfirmDelete(null)
    deleteMutation.mutate(project.id)
  }

  const colIdxOf = (id: ColumnId) => MAIN_COLUMNS.findIndex(c => c.id === id)

  function navigateCursor(delta: 1 | -1) {
    const next = MAIN_COLUMNS[Math.min(Math.max(colIdxOf(selectedColId) + delta, 0), MAIN_COLUMNS.length - 1)]
    if (next && next.id !== selectedColId) {
      setSelectedColId(next.id)
      setSelectedRowIdx(0)
    }
  }

  function moveSelectedProject(delta: 1 | -1) {
    const project = selectedColProjects[selectedRowIdx]
    if (!project) return
    const newStatusIdx = STATUS_ORDER.indexOf(project.status ?? 'PROJECT_STATUS_UNSPECIFIED') + delta
    const newStatus = STATUS_ORDER[newStatusIdx]
    if (!newStatus) return
    const newColId = STATUS_TO_COLUMN[newStatus] ?? selectedColId

    if (newColId !== selectedColId) {
      // Compute destination index from current (pre-mutation) filteredProjects, simulating the move
      const newIdx = filteredProjects
        .filter(p => {
          const eff = p.id === project.id ? newStatus : (p.status ?? 'PROJECT_STATUS_UNSPECIFIED')
          return STATUS_TO_COLUMN[eff] === newColId
        })
        .findIndex(p => p.id === project.id)
      setSelectedColId(newColId)
      setSelectedRowIdx(Math.max(0, newIdx))
    }

    moveProject(project, delta)
  }

  // ─── vim bindings ─────────────────────────────────────────────────────────

  const bindings = useMemo(() => [
    {
      key: 'h',
      description: 'Select column left',
      handler: () => navigateCursor(-1),
    },
    {
      key: 'l',
      description: 'Select column right',
      handler: () => navigateCursor(1),
    },
    {
      key: 'j',
      description: 'Select next project',
      handler: () => setSelectedRowIdx(r => Math.min(r + 1, selectedColProjects.length - 1)),
    },
    {
      key: 'k',
      description: 'Select previous project',
      handler: () => setSelectedRowIdx(r => Math.max(r - 1, 0)),
    },
    {
      key: 'H',
      description: 'Move project left',
      handler: () => moveSelectedProject(-1),
    },
    {
      key: 'L',
      description: 'Move project right',
      handler: () => moveSelectedProject(1),
    },
    {
      key: 'gg',
      description: 'Go to first project',
      handler: () => setSelectedRowIdx(0),
    },
    {
      key: 'G',
      description: 'Go to last project',
      handler: () => setSelectedRowIdx(Math.max(0, selectedColProjects.length - 1)),
    },
    {
      key: 'Enter',
      description: 'Open selected project',
      handler: () => {
        const project = selectedColProjects[selectedRowIdx]
        if (project) setOpenProjectId(project.id)
      },
    },
    {
      key: 'd',
      description: 'Delete selected project',
      handler: () => {
        const project = selectedColProjects[selectedRowIdx]
        if (project) setConfirmDelete(project)
      },
    },
    { key: 'p1', description: 'Set priority P1', handler: () => setSelectedPriority('PRIORITY_1') },
    { key: 'p2', description: 'Set priority P2', handler: () => setSelectedPriority('PRIORITY_2') },
    { key: 'p3', description: 'Set priority P3', handler: () => setSelectedPriority('PRIORITY_3') },
    { key: 'p4', description: 'Set priority P4', handler: () => setSelectedPriority('PRIORITY_4') },
    { key: 'p5', description: 'Set priority P5', handler: () => setSelectedPriority('PRIORITY_5') },
  ], [columns, selectedColId, selectedRowIdx, selectedColProjects, archivedProjects])

  useVimBindings(bindings)

  if (isLoading) return <div className="p-6 text-muted-foreground">Loading…</div>
  if (error)     return <div className="p-6 text-destructive">Error: {String(error)}</div>

  return (
    <>
    {openProjectId && (
      <ProjectDialog
        projectId={openProjectId}
        onClose={() => setOpenProjectId(null)}
      />
    )}
    {confirmDelete && (
      <ConfirmDialog
        title="Delete project?"
        description={<><span className="font-medium text-foreground">{confirmDelete.title}</span> will be permanently deleted.</>}
        confirmLabel="Delete"
        onConfirm={() => deleteProject(confirmDelete)}
        onCancel={() => setConfirmDelete(null)}
      />
    )}
    <div className="p-6 h-full flex flex-col">
      <div className="flex items-center gap-3 mb-6">
        <h1 className="text-2xl font-semibold">Projects</h1>
        <div className="flex items-center gap-1.5">
          <button
            onClick={() => { setSelectedAreaIds(new Set()); setSelectedRowIdx(0) }}
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
              setSelectedRowIdx(0)
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
      <div className="flex gap-4 flex-1 items-start overflow-y-auto">
        {MAIN_COLUMNS.map(col => {
          const colProjects = columns.get(col.id) ?? []
          const isArchived = col.id === 'archived'

          return (
            <div key={col.id} className={`flex flex-col flex-1 min-w-0 rounded-lg px-2 py-2 -mx-2 transition-colors ${selectedColId === col.id ? 'bg-accent/30' : ''} ${col.id === 'active' ? 'ring-1 ring-inset ring-primary/20' : ''}`}>
              <div className="flex items-center justify-between mb-3">
                <h2 className="text-xs font-semibold uppercase tracking-wider text-muted-foreground">
                  {col.label}
                </h2>
                {colProjects.length > 0 && (
                  <span className="text-xs text-muted-foreground bg-muted rounded px-1.5 py-0.5">
                    {colProjects.length}
                  </span>
                )}
              </div>

              {isArchived ? (
                <div className="flex flex-col gap-3 border border-border rounded-lg p-2">
                  <Section
                    label="Completed"
                    projects={completedProjects}
                    colId="archived"
                    indexOffset={0}
                    selectedColId={selectedColId}
                    selectedRowIdx={selectedRowIdx}
                    showEffort={false}
                    onSelect={(c, r) => { setSelectedColId(c); setSelectedRowIdx(r) }}
                    itemRefs={itemRefs}
                  />
                  <div className="border-t border-border/50" />
                  <Section
                    label="Abandoned"
                    projects={abandonedProjects}
                    colId="archived"
                    indexOffset={completedProjects.length}
                    selectedColId={selectedColId}
                    selectedRowIdx={selectedRowIdx}
                    showEffort={false}
                    onSelect={(c, r) => { setSelectedColId(c); setSelectedRowIdx(r) }}
                    itemRefs={itemRefs}
                  />
                </div>
              ) : (
                <Section
                  projects={colProjects}
                  colId={col.id}
                  indexOffset={0}
                  selectedColId={selectedColId}
                  selectedRowIdx={selectedRowIdx}
                  showEffort={true}
                  onSelect={(c, r) => { setSelectedColId(c); setSelectedRowIdx(r) }}
                  itemRefs={itemRefs}
                />
              )}
            </div>
          )
        })}
      </div>
    </div>
    </>
  )
}
