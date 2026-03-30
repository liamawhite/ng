import { type Area, type Effort, type Link, type Project, type Priority, type ProjectStatus, type TaskStatus } from './api'

export interface TaskWithSubtasks {
  id: string
  title: string
  content?: string
  projectId?: string
  parentTaskId?: string
  status: TaskStatus
  pinned?: boolean
  tasks: TaskWithSubtasks[]
}

export interface ProjectWithRelations {
  id: string
  title: string
  content?: string
  status?: ProjectStatus
  areaId?: string
  parentId?: string
  completed?: string
  estimatedEffort?: Effort
  links?: Link[]
  priority?: Priority
  projects: { id: string; title: string; status?: ProjectStatus }[]
  tasks: TaskWithSubtasks[]
}

export interface AreaWithProjects extends Area {
  projects: Project[]
}

export interface PinnedTaskWithContext {
  id: string
  title: string
  status: TaskStatus
  projectId?: string
  parentTaskId?: string
  pinned?: boolean
  priority?: Priority
  breadcrumb: string[]  // [projectTitle, parentTaskTitle?] — excludes the task itself
  areaId?: string
}

export async function query(base: string, body: unknown): Promise<unknown> {
  const res = await fetch(`${base.replace(/\/$/, '')}/protograph/v1alpha1/query`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(body),
  })
  if (!res.ok) {
    const text = await res.text().catch(() => String(res.status))
    throw new Error(`protograph: ${res.status} ${text}`)
  }
  return res.json()
}
