import { useMemo } from 'react'
import { useVimBindings } from '../lib/vim'

interface ConfirmDialogProps {
  title: string
  description?: React.ReactNode
  confirmLabel?: string
  cancelLabel?: string
  onConfirm: () => void
  onCancel: () => void
}

export function ConfirmDialog({
  title,
  description,
  confirmLabel = 'Confirm',
  cancelLabel = 'Cancel',
  onConfirm,
  onCancel,
}: ConfirmDialogProps) {
  useVimBindings(useMemo(() => [
    { key: 'y',      description: 'Confirm', handler: onConfirm },
    { key: 'n',      description: 'Cancel',  handler: onCancel },
    { key: 'Escape', description: 'Cancel',  handler: onCancel },
  ], [onConfirm, onCancel]))

  return (
    <div
      className="fixed inset-0 z-50 flex items-center justify-center bg-black/50"
      onClick={onCancel}
    >
      <div
        className="bg-background border border-border rounded-xl shadow-lg p-6 max-w-sm w-full mx-4"
        onClick={e => e.stopPropagation()}
      >
        <h2 className="text-base font-semibold mb-1">{title}</h2>
        {description && (
          <p className="text-sm text-muted-foreground mb-5 leading-snug">{description}</p>
        )}
        <div className="flex gap-2 justify-end">
          <button
            onClick={onCancel}
            className="px-3 py-1.5 text-sm rounded-md border border-border hover:bg-accent transition-colors"
          >
            {cancelLabel} <span className="text-muted-foreground ml-1 text-xs">[n]</span>
          </button>
          <button
            onClick={onConfirm}
            className="px-3 py-1.5 text-sm rounded-md bg-destructive text-destructive-foreground hover:bg-destructive/90 transition-colors"
          >
            {confirmLabel} <span className="opacity-70 ml-1 text-xs">[y]</span>
          </button>
        </div>
      </div>
    </div>
  )
}
