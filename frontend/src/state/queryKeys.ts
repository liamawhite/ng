export const queryKeys = {
  areas:       () => ['areas']              as const,
  pinnedTasks: () => ['pinnedTasks']        as const,
  project:     (id: string) => ['project', id] as const,
  task:        (id: string) => ['task',    id] as const,
}
