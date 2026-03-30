// Types matching the grpc-gateway JSON output (proto field names → camelCase).

export interface Area {
  id: string
  title: string
  color?: string
}

export type EffortUnit =
  | 'EFFORT_UNIT_UNSPECIFIED'
  | 'EFFORT_UNIT_DAYS'
  | 'EFFORT_UNIT_WEEKS'
  | 'EFFORT_UNIT_MONTHS'

export interface Effort {
  value: number
  unit: EffortUnit
}

export interface Link {
  url: string
  title?: string
}

export interface Project {
  id: string
  title: string
  content?: string
  parentId?: string
  status?: ProjectStatus
  areaId?: string
  completed?: string // RFC3339 timestamp
  estimatedEffort?: Effort
  links?: Link[]
  priority?: Priority
}

export interface Task {
  id: string
  title: string
  content?: string
  projectId?: string
  parentTaskId?: string
  status: TaskStatus
  pinned?: boolean
  priority?: Priority
}

export type ProjectStatus =
  | 'PROJECT_STATUS_UNSPECIFIED'
  | 'PROJECT_STATUS_ACTIVE'
  | 'PROJECT_STATUS_BACKLOG'
  | 'PROJECT_STATUS_BLOCKED'
  | 'PROJECT_STATUS_COMPLETED'
  | 'PROJECT_STATUS_ABANDONED'

export type Priority =
  | 'PRIORITY_UNSPECIFIED'
  | 'PRIORITY_1'
  | 'PRIORITY_2'
  | 'PRIORITY_3'
  | 'PRIORITY_4'
  | 'PRIORITY_5'

export type TaskStatus =
  | 'TASK_STATUS_UNSPECIFIED'
  | 'TASK_STATUS_TODO'
  | 'TASK_STATUS_IN_PROGRESS'
  | 'TASK_STATUS_DONE'

export type Predicate = 'PREDICATE_UNSPECIFIED' | 'PREDICATE_PART_OF' | 'PREDICATE_IN_AREA' | 'PREDICATE_TASK' | 'PREDICATE_SUBTASK' | 'PREDICATE_SUBPROJECT'
export type Direction = 'DIRECTION_UNSPECIFIED' | 'DIRECTION_OUTGOING' | 'DIRECTION_INCOMING'

export interface RelatedEntity {
  predicate: Predicate
  direction: Direction
  entity: { project?: Project; task?: Task; area?: Area }
}

// --- client factory ---

import { getBaseURL } from './transport'

async function request<T>(
  method: string,
  path: string,
  body?: unknown,
): Promise<T> {
  const base = await getBaseURL()
  const res = await fetch(`${base}${path}`, {
    method,
    headers: body ? { 'Content-Type': 'application/json' } : {},
    body: body ? JSON.stringify(body) : undefined,
  })
  if (!res.ok) {
    const text = await res.text()
    throw new Error(`${method} ${path} → ${res.status}: ${text}`)
  }
  // DELETE returns empty body
  if (res.status === 204 || res.headers.get('content-length') === '0') return {} as T
  return res.json() as Promise<T>
}

// --- Areas ---

export const areas = {
  create: (data: { title: string }) => request<Area>('POST', '/api/v1/areas', data),
  get: (id: string) => request<Area>('GET', `/api/v1/areas/${id}`),
  list: () => request<{ areas: Area[] }>('GET', '/api/v1/areas'),
  update: (id: string, data: { title?: string; color?: string; updateMask: string }) => request<Area>('PUT', `/api/v1/areas/${id}`, data),
  delete: (id: string) => request<void>('DELETE', `/api/v1/areas/${id}`),
}

// --- Projects ---

export const projects = {
  create: (data: { title: string; content?: string; parentId?: string; status?: ProjectStatus; areaId?: string; estimatedEffort?: Effort; links?: Link[] }) =>
    request<Project>('POST', '/api/v1/projects', data),

  get: (id: string) => request<Project>('GET', `/api/v1/projects/${id}`),

  list: (filters?: { parentId?: string; status?: ProjectStatus; areaId?: string }) => {
    const params = new URLSearchParams()
    if (filters?.parentId) params.set('parent_id', filters.parentId)
    if (filters?.status) params.set('status', filters.status)
    if (filters?.areaId) params.set('area_id', filters.areaId)
    const qs = params.size ? `?${params}` : ''
    return request<{ projects: Project[] }>('GET', `/api/v1/projects${qs}`)
  },

  update: (id: string, data: { title?: string; content?: string; parentId?: string; status?: ProjectStatus; areaId?: string; estimatedEffort?: Effort | null; links?: Link[]; priority?: Priority; updateMask: string }) =>
    request<Project>('PUT', `/api/v1/projects/${id}`, data),

  delete: (id: string) => request<void>('DELETE', `/api/v1/projects/${id}`),
}

// --- Tasks ---

export const tasks = {
  create: (data: {
    title: string
    content?: string
    projectId?: string
    parentTaskId?: string
    status?: TaskStatus
    pinned?: boolean
  }) => request<Task>('POST', '/api/v1/tasks', data),

  get: (id: string) => request<Task>('GET', `/api/v1/tasks/${id}`),

  list: (filters?: { projectId?: string; pinned?: boolean }) => {
    const params = new URLSearchParams()
    if (filters?.projectId) params.set('project_id', filters.projectId)
    if (filters?.pinned !== undefined) params.set('pinned', String(filters.pinned))
    const qs = params.size ? `?${params}` : ''
    return request<{ tasks: Task[] }>('GET', `/api/v1/tasks${qs}`)
  },

  update: (id: string, data: { title?: string; content?: string; projectId?: string; parentTaskId?: string; status?: TaskStatus; pinned?: boolean; priority?: Priority; updateMask: string }) =>
    request<Task>('PUT', `/api/v1/tasks/${id}`, data),

  delete: (id: string) => request<void>('DELETE', `/api/v1/tasks/${id}`),
}

// --- Graph ---

export const graph = {
  listRelated: (id: string, predicate?: Predicate, direction?: Direction) => {
    const params = new URLSearchParams()
    if (predicate) params.set('predicate', predicate)
    if (direction) params.set('direction', direction)
    const qs = params.size ? `?${params}` : ''
    return request<{ entities: RelatedEntity[] }>('GET', `/api/v1/graph/${id}/related${qs}`)
  },
}
