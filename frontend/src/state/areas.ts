import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import { toast } from 'sonner'
import { projects as projectsApi, graph as graphApi, type Link, type Priority, type ProjectStatus } from '../lib/api'
import { query, type AreaWithProjects, type ProjectWithRelations, type TaskWithSubtasks } from '../lib/protograph'
import { getBaseURL } from '../lib/transport'
import { queryKeys } from './queryKeys'

function nestedTaskFields(depth: number): Record<string, unknown> {
  const base: Record<string, unknown> = { id: {}, title: {}, content: {}, status: {}, projectId: {}, parentTaskId: {}, pinned: {} }
  if (depth > 0) base['tasks'] = nestedTaskFields(depth - 1)
  return base
}

export async function listAreasWithProjects(): Promise<AreaWithProjects[]> {
  const base = await getBaseURL()
  const result = await query(base, {
    'ng.v1.AreaService': {
      list: {
        $: {},
        areas: {
          id: {},
          title: {},
          color: {},
          projects: { id: {}, title: {}, status: {}, areaId: {}, completed: {}, estimatedEffort: { value: {}, unit: {} }, priority: {} },
        },
      },
    },
  })
  const data = result as { 'ng.v1.AreaService'?: { list?: { areas?: AreaWithProjects[] } } }
  return data['ng.v1.AreaService']?.list?.areas ?? []
}

export async function getProjectWithRelations(projectId: string): Promise<ProjectWithRelations> {
  const base = await getBaseURL()

  // NOTE: ProjectService.Get returns a single object — protograph only fans out
  // relations on []any list items, so tasks would be silently absent if we used Get.
  // Instead, drive the task tree via TaskService.List, where fan-out works correctly.
  const [proj, related, tasksResult] = await Promise.all([
    projectsApi.get(projectId),
    graphApi.listRelated(projectId, 'PREDICATE_SUBPROJECT', 'DIRECTION_OUTGOING'),
    query(base, {
      'ng.v1.TaskService': {
        list: {
          $: { projectId },
          tasks: nestedTaskFields(4),
        },
      },
    }),
  ])

  const subprojects = related.entities
    .filter(r => r.entity.project)
    .map(r => r.entity.project as { id: string; title: string; status?: ProjectStatus })

  const tasksData = tasksResult as { 'ng.v1.TaskService'?: { list?: { tasks?: TaskWithSubtasks[] } } }
  const allTasks = tasksData['ng.v1.TaskService']?.list?.tasks ?? []

  return {
    id: proj.id,
    title: proj.title,
    content: proj.content,
    status: proj.status,
    areaId: proj.areaId,
    parentId: proj.parentId,
    completed: proj.completed,
    estimatedEffort: proj.estimatedEffort,
    links: proj.links,
    priority: proj.priority,
    projects: subprojects,
    tasks: allTasks,
  }
}

export function useAreasQuery() {
  return useQuery({ queryKey: queryKeys.areas(), queryFn: listAreasWithProjects })
}

export function useProjectQuery(projectId: string) {
  return useQuery({
    queryKey: queryKeys.project(projectId),
    queryFn: () => getProjectWithRelations(projectId),
    staleTime: Infinity,
  })
}

export function useCreateProjectMutation() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: (data: { title: string; areaId?: string }) => projectsApi.create(data),
    onSuccess: () => queryClient.invalidateQueries({ queryKey: queryKeys.areas() }),
  })
}

export function useProjectSaveMutation(projectId: string) {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: (data: Parameters<typeof projectsApi.update>[1]) => projectsApi.update(projectId, data),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: queryKeys.areas() })
      queryClient.invalidateQueries({ queryKey: queryKeys.project(projectId) })
    },
    onError: () => toast.error('Failed to save project'),
  })
}

export function useProjectLinksMutation(projectId: string) {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: (links: Link[]) => projectsApi.update(projectId, { links, updateMask: 'links' }),
    onSuccess: () => queryClient.invalidateQueries({ queryKey: queryKeys.project(projectId) }),
  })
}

export function useProjectStatusMutation() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: ({ id, status }: { id: string; status: ProjectStatus }) =>
      projectsApi.update(id, { status, updateMask: 'status' }),
    onMutate: async ({ id, status }) => {
      await queryClient.cancelQueries({ queryKey: queryKeys.areas() })
      const prev = queryClient.getQueryData(queryKeys.areas())
      queryClient.setQueryData(queryKeys.areas(), (old: AreaWithProjects[] | undefined) =>
        (old ?? []).map(a => ({ ...a, projects: a.projects.map(p => p.id === id ? { ...p, status } : p) })))
      return { prev }
    },
    onError: (_err, _vars, ctx) => queryClient.setQueryData(queryKeys.areas(), ctx?.prev),
    onSettled: () => queryClient.invalidateQueries({ queryKey: queryKeys.areas() }),
  })
}

export function useProjectPriorityMutation() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: ({ id, priority }: { id: string; priority: Priority }) =>
      projectsApi.update(id, { priority, updateMask: 'priority' }),
    onMutate: async ({ id, priority }) => {
      await queryClient.cancelQueries({ queryKey: queryKeys.areas() })
      const prev = queryClient.getQueryData(queryKeys.areas())
      queryClient.setQueryData(queryKeys.areas(), (old: AreaWithProjects[] | undefined) =>
        (old ?? []).map(a => ({ ...a, projects: a.projects.map(p => p.id === id ? { ...p, priority } : p) })))
      return { prev }
    },
    onError: (_err, _vars, ctx) => queryClient.setQueryData(queryKeys.areas(), ctx?.prev),
    onSettled: () => queryClient.invalidateQueries({ queryKey: queryKeys.areas() }),
  })
}

export function useDeleteProjectMutation() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: (id: string) => projectsApi.delete(id),
    onMutate: async (id) => {
      await queryClient.cancelQueries({ queryKey: queryKeys.areas() })
      const prev = queryClient.getQueryData(queryKeys.areas())
      queryClient.setQueryData(queryKeys.areas(), (old: AreaWithProjects[] | undefined) =>
        (old ?? []).map(a => ({ ...a, projects: a.projects.filter(p => p.id !== id) })))
      return { prev }
    },
    onError: (_err, _vars, ctx) => queryClient.setQueryData(queryKeys.areas(), ctx?.prev),
    onSettled: () => queryClient.invalidateQueries({ queryKey: queryKeys.areas() }),
  })
}
