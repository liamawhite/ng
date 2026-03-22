import { createContext, useCallback, useContext, useEffect, useRef } from 'react'

export interface VimBinding {
  key: string        // 'j', 'k', 'G', 'gg', 'Enter', 'Escape'
  description: string
  handler: () => void
}

interface BindingsLayer {
  id: symbol
  getBindings: () => VimBinding[]
}

interface VimContextType {
  addLayer: (layer: BindingsLayer) => void
  removeLayer: (id: symbol) => void
}

const VimContext = createContext<VimContextType | null>(null)

export function VimProvider({ children }: { children: React.ReactNode }) {
  const layersRef = useRef<BindingsLayer[]>([])
  const sequenceRef = useRef('')
  const timeoutRef = useRef<ReturnType<typeof setTimeout> | null>(null)

  const clearSequence = useCallback(() => {
    sequenceRef.current = ''
    if (timeoutRef.current) {
      clearTimeout(timeoutRef.current)
      timeoutRef.current = null
    }
  }, [])

  const addLayer = useCallback((layer: BindingsLayer) => {
    layersRef.current = [...layersRef.current, layer]
  }, [])

  const removeLayer = useCallback((id: symbol) => {
    layersRef.current = layersRef.current.filter(l => l.id !== id)
    clearSequence()
  }, [clearSequence])

  useEffect(() => {
    const handleKeyDown = (e: KeyboardEvent) => {
      const target = e.target
      if (
        target instanceof HTMLElement && (
          target.tagName === 'INPUT' ||
          target.tagName === 'TEXTAREA' ||
          target.tagName === 'SELECT' ||
          target.contentEditable === 'true'
        )
      ) return

      if (['Meta', 'Control', 'Alt', 'Shift'].includes(e.key)) return

      const topLayer = layersRef.current[layersRef.current.length - 1]
      if (!topLayer) return

      const bindings = topLayer.getBindings()
      const seq = sequenceRef.current + e.key

      const exact = bindings.find(b => b.key === seq)
      if (exact) {
        e.preventDefault()
        exact.handler()
        clearSequence()
        return
      }

      const isPrefix = bindings.some(b => b.key.startsWith(seq) && b.key !== seq)
      if (isPrefix) {
        e.preventDefault()
        sequenceRef.current = seq
        if (timeoutRef.current) clearTimeout(timeoutRef.current)
        timeoutRef.current = setTimeout(clearSequence, 1000)
        return
      }

      // No match with accumulated sequence — retry with just this key
      const singleExact = bindings.find(b => b.key === e.key)
      if (singleExact) {
        e.preventDefault()
        singleExact.handler()
      }
      clearSequence()
    }

    document.addEventListener('keydown', handleKeyDown)
    return () => document.removeEventListener('keydown', handleKeyDown)
  }, [clearSequence])

  return (
    <VimContext.Provider value={{ addLayer, removeLayer }}>
      {children}
    </VimContext.Provider>
  )
}

function useVimContext() {
  const ctx = useContext(VimContext)
  if (!ctx) throw new Error('useVimBindings must be used within VimProvider')
  return ctx
}

/**
 * Register vim key bindings for the current view. The bindings are active while
 * the component is mounted; the topmost registered layer takes precedence.
 *
 * Pass a stable array (e.g. useMemo) to avoid unnecessary churn, though the
 * handlers are always read fresh via a ref so closures stay current regardless.
 */
export function useVimBindings(bindings: VimBinding[]) {
  const { addLayer, removeLayer } = useVimContext()
  const bindingsRef = useRef(bindings)
  bindingsRef.current = bindings

  useEffect(() => {
    const id = Symbol()
    addLayer({ id, getBindings: () => bindingsRef.current })
    return () => removeLayer(id)
    // intentionally only run on mount/unmount
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [])
}
