import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import { toast } from 'sonner'
import { tasks as tasksApi, type Priority, type TaskStatus } from '../lib/api'
import { query, type PinnedTaskWithContext } from '../lib/protograph'
import { getBaseURL } from '../lib/transport'
import { queryKeys } from './queryKeys'

export async function listPinnedTasks(): Promise<PinnedTaskWithContext[]> {
  const base = await getBaseURL()
  const result = await query(base, {
    'ng.v1.TaskService': {
      list: {
        $: { pinned: true },
        tasks: {
          id: {}, title: {}, status: {}, projectId: {}, parentTaskId: {}, pinned: {}, priority: {},
          project: { id: {}, title: {}, areaId: {} },
          task: { id: {}, title: {} },
        },
      },
    },
  })

  const data = result as {
    'ng.v1.TaskService'?: { list?: { tasks?: Array<{
      id: string; title: string; status: TaskStatus
      projectId?: string; parentTaskId?: string; pinned?: boolean; priority?: Priority
      project?: { id: string; title: string; areaId?: string }
      task?: { id: string; title: string }
    }> } }
  }

  return (data['ng.v1.TaskService']?.list?.tasks ?? []).map(t => {
    const crumbs: string[] = t.project ? [t.project.title] : []
    if (t.task) crumbs.push(t.task.title)
    return {
      id: t.id,
      title: t.title,
      status: t.status,
      projectId: t.projectId,
      parentTaskId: t.parentTaskId,
      pinned: t.pinned,
      priority: t.priority,
      breadcrumb: crumbs,
      areaId: t.project?.areaId,
    }
  })
}

export function usePinnedTasksQuery() {
  return useQuery({ queryKey: queryKeys.pinnedTasks(), queryFn: listPinnedTasks })
}

export function useTaskQuery(taskId: string) {
  return useQuery({
    queryKey: queryKeys.task(taskId),
    queryFn: () => tasksApi.get(taskId),
    staleTime: Infinity,
  })
}

export function useCreateTaskMutation() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: (data: Parameters<typeof tasksApi.create>[0]) => tasksApi.create(data),
    onSuccess: (_, vars) => {
      queryClient.invalidateQueries({ queryKey: queryKeys.pinnedTasks() })
      if (vars.projectId) queryClient.invalidateQueries({ queryKey: queryKeys.project(vars.projectId) })
    },
  })
}

export function useTaskSaveMutation(taskId: string, projectId?: string) {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: (data: Parameters<typeof tasksApi.update>[1]) => tasksApi.update(taskId, data),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: queryKeys.pinnedTasks() })
      queryClient.invalidateQueries({ queryKey: queryKeys.task(taskId) })
      if (projectId) queryClient.invalidateQueries({ queryKey: queryKeys.project(projectId) })
    },
    onError: () => toast.error('Failed to save task'),
  })
}

export function useTaskStatusMutation() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: ({ id, status }: { id: string; status: TaskStatus; projectId?: string }) =>
      tasksApi.update(id, { status, updateMask: 'status' }),
    onSuccess: (_, { projectId }) => {
      queryClient.invalidateQueries({ queryKey: queryKeys.pinnedTasks() })
      if (projectId) queryClient.invalidateQueries({ queryKey: queryKeys.project(projectId) })
    },
  })
}

export function useTaskPinnedMutation() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: ({ id, pinned }: { id: string; pinned: boolean; projectId?: string }) =>
      tasksApi.update(id, { pinned, updateMask: 'pinned' }),
    onSuccess: (_, { projectId }) => {
      queryClient.invalidateQueries({ queryKey: queryKeys.pinnedTasks() })
      if (projectId) queryClient.invalidateQueries({ queryKey: queryKeys.project(projectId) })
    },
  })
}

export function useTaskPriorityMutation() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: ({ id, priority }: { id: string; priority: Priority }) =>
      tasksApi.update(id, { priority, updateMask: 'priority' }),
    onMutate: async ({ id, priority }) => {
      await queryClient.cancelQueries({ queryKey: queryKeys.pinnedTasks() })
      const prev = queryClient.getQueryData(queryKeys.pinnedTasks())
      queryClient.setQueryData(queryKeys.pinnedTasks(), (old: PinnedTaskWithContext[] | undefined) =>
        (old ?? []).map(t => t.id === id ? { ...t, priority } : t))
      return { prev }
    },
    onError: (_err, _vars, ctx) => queryClient.setQueryData(queryKeys.pinnedTasks(), ctx?.prev),
    onSettled: () => queryClient.invalidateQueries({ queryKey: queryKeys.pinnedTasks() }),
  })
}
