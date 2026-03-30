import { useCallback, useEffect, useRef, useState } from 'react'
import { Dialog, DialogContent, DialogHeader, DialogTitle } from './ui/dialog'
import { Textarea } from './ui/textarea'
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from './ui/select'
import { type Priority, type TaskStatus } from '../lib/api'
import { useTaskQuery, useTaskSaveMutation } from '../state/tasks'
import { Pin } from 'lucide-react'

const TASK_STATUSES: { value: TaskStatus; label: string }[] = [
  { value: 'TASK_STATUS_TODO',        label: 'Todo' },
  { value: 'TASK_STATUS_IN_PROGRESS', label: 'In Progress' },
  { value: 'TASK_STATUS_DONE',        label: 'Done' },
]

const TASK_PRIORITIES: { value: Priority; label: string }[] = [
  { value: 'PRIORITY_1', label: 'P1' },
  { value: 'PRIORITY_2', label: 'P2' },
  { value: 'PRIORITY_3', label: 'P3' },
  { value: 'PRIORITY_4', label: 'P4' },
  { value: 'PRIORITY_5', label: 'P5' },
]

interface Props {
  taskId: string
  breadcrumb?: string[]
  onClose: () => void
}

export default function TaskDialog({ taskId, breadcrumb, onClose }: Props) {
  const [title, setTitle] = useState('')
  const [content, setContent] = useState('')
  const [status, setStatus] = useState<TaskStatus>('TASK_STATUS_TODO')
  const [priority, setPriority] = useState<Priority>('PRIORITY_4')
  const [pinned, setPinned] = useState(false)

  const debounceRef = useRef<ReturnType<typeof setTimeout> | null>(null)
  const skipSaveRef = useRef(true)
  const formRef = useRef({ title: '', content: '', status: 'TASK_STATUS_TODO' as TaskStatus, priority: 'PRIORITY_4' as Priority, pinned: false })

  formRef.current = { title, content, status, priority, pinned }

  const { data: taskData, isLoading } = useTaskQuery(taskId)

  useEffect(() => {
    if (!taskData) return
    skipSaveRef.current = true
    setTitle(taskData.title)
    setContent(taskData.content ?? '')
    setStatus(taskData.status ?? 'TASK_STATUS_TODO')
    setPriority(taskData.priority ?? 'PRIORITY_4')
    setPinned(taskData.pinned ?? false)
  }, [taskData])

  const saveMutation = useTaskSaveMutation(taskId, taskData?.projectId)

  const saveTask = useCallback(() => {
    const { title, content, status, priority, pinned } = formRef.current
    saveMutation.mutate({
      title, content, status, priority, pinned,
      updateMask: 'title,content,status,priority,pinned',
    })
  }, [saveMutation])

  const scheduleSave = useCallback(() => {
    if (debounceRef.current) clearTimeout(debounceRef.current)
    debounceRef.current = setTimeout(saveTask, 600)
  }, [saveTask])

  useEffect(() => {
    if (skipSaveRef.current) { skipSaveRef.current = false; return }
    scheduleSave()
  }, [title, content, status, priority, pinned, scheduleSave])

  useEffect(() => {
    return () => { if (debounceRef.current) clearTimeout(debounceRef.current) }
  }, [])

  function handleClose() {
    if (debounceRef.current) {
      clearTimeout(debounceRef.current)
      debounceRef.current = null
      saveTask()
    }
    onClose()
  }

  return (
    <Dialog open onOpenChange={open => { if (!open) handleClose() }}>
      <DialogContent className="w-[680px] max-w-[95vw] max-h-[90vh] flex flex-col overflow-y-auto">
        <DialogHeader>
          <DialogTitle>
            <input
              className="w-full bg-transparent text-lg font-semibold outline-none placeholder:text-muted-foreground"
              value={title}
              onChange={e => setTitle(e.target.value)}
              placeholder="Task title"
            />
          </DialogTitle>
        </DialogHeader>

        {isLoading ? (
          <div className="flex-1 flex items-center justify-center text-muted-foreground text-sm">Loading…</div>
        ) : (
          <div className="space-y-5 pr-1">
            {breadcrumb && breadcrumb.length > 0 && (
              <p className="text-xs text-muted-foreground">{breadcrumb.join(' › ')}</p>
            )}

            <div className="flex items-center gap-6 flex-wrap">
              <div className="flex items-center gap-3">
                <span className="text-xs text-muted-foreground shrink-0">Status</span>
                <Select value={status} onValueChange={v => setStatus(v as TaskStatus)}>
                  <SelectTrigger className="w-36 h-7 text-xs">
                    <SelectValue />
                  </SelectTrigger>
                  <SelectContent>
                    {TASK_STATUSES.map(s => (
                      <SelectItem key={s.value} value={s.value} className="text-xs">{s.label}</SelectItem>
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
                    {TASK_PRIORITIES.map(p => (
                      <SelectItem key={p.value} value={p.value} className="text-xs">{p.label}</SelectItem>
                    ))}
                  </SelectContent>
                </Select>
              </div>

              <button
                onClick={() => setPinned(p => !p)}
                className={`flex items-center gap-1.5 h-7 px-2 rounded-md border text-xs transition-colors ${
                  pinned
                    ? 'border-foreground text-foreground bg-accent'
                    : 'border-input text-muted-foreground hover:text-foreground'
                }`}
                title={pinned ? 'Unpin task' : 'Pin task'}
              >
                <Pin size={11} className={pinned ? 'fill-current' : ''} />
                {pinned ? 'Pinned' : 'Pin'}
              </button>
            </div>

            <div>
              <p className="text-xs text-muted-foreground mb-1.5">Notes</p>
              <Textarea
                value={content}
                onChange={e => setContent(e.target.value)}
                placeholder="Add notes…"
                className="min-h-[200px] text-sm resize-none"
              />
            </div>
          </div>
        )}
      </DialogContent>
    </Dialog>
  )
}
