import { createContext, useCallback, useContext, useEffect, useRef, useState } from 'react'

export interface VimBinding {
  key: string        // 'j', 'k', 'G', 'gg', 'Enter', 'Escape'
  description: string
  handler: () => void
}

interface BindingsLayer {
  id: symbol
  global: boolean
  getBindings: () => VimBinding[]
}

interface VimContextType {
  addLayer: (layer: BindingsLayer) => void
  removeLayer: (id: symbol) => void
  getActiveBindings: () => VimBinding[]
  getSequence: () => string
  subscribe: (cb: () => void) => () => void
}

function mergeBindings(layers: BindingsLayer[]): VimBinding[] {
  const topLayer = [...layers].reverse().find(l => !l.global)
  const globalBindings = layers.filter(l => l.global).flatMap(l => l.getBindings())
  return [...(topLayer ? topLayer.getBindings() : []), ...globalBindings]
}

const VimContext = createContext<VimContextType | null>(null)

export function VimProvider({ children }: { children: React.ReactNode }) {
  const layersRef = useRef<BindingsLayer[]>([])
  const sequenceRef = useRef('')
  const timeoutRef = useRef<ReturnType<typeof setTimeout> | null>(null)
  const subscribersRef = useRef<Set<() => void>>(new Set())

  const notify = useCallback(() => {
    subscribersRef.current.forEach(cb => cb())
  }, [])

  const subscribe = useCallback((cb: () => void) => {
    subscribersRef.current.add(cb)
    return () => subscribersRef.current.delete(cb)
  }, [])

  const getActiveBindings = useCallback((): VimBinding[] => {
    return mergeBindings(layersRef.current)
  }, [])

  const clearSequence = useCallback(() => {
    sequenceRef.current = ''
    if (timeoutRef.current) {
      clearTimeout(timeoutRef.current)
      timeoutRef.current = null
    }
    notify()
  }, [notify])

  const addLayer = useCallback((layer: BindingsLayer) => {
    layersRef.current = [...layersRef.current, layer]
    notify()
  }, [notify])

  const removeLayer = useCallback((id: symbol) => {
    layersRef.current = layersRef.current.filter(l => l.id !== id)
    clearSequence()
    notify()
  }, [clearSequence, notify])

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

      if (!layersRef.current.length) return

      const bindings = mergeBindings(layersRef.current)
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
        notify()
        if (timeoutRef.current) clearTimeout(timeoutRef.current)
        timeoutRef.current = setTimeout(clearSequence, 10000)
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

  const getSequence = useCallback(() => sequenceRef.current, [])

  return (
    <VimContext.Provider value={{ addLayer, removeLayer, getActiveBindings, getSequence, subscribe }}>
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
export function useVimBindings(bindings: VimBinding[], options?: { global?: boolean }) {
  const { addLayer, removeLayer } = useVimContext()
  const bindingsRef = useRef(bindings)
  bindingsRef.current = bindings

  useEffect(() => {
    const id = Symbol()
    addLayer({ id, global: options?.global ?? false, getBindings: () => bindingsRef.current })
    return () => removeLayer(id)
    // intentionally only run on mount/unmount
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [])
}

/** Returns the current top-layer vim bindings for display purposes. */
export function useActiveVimBindings(): VimBinding[] {
  const { getActiveBindings, subscribe } = useVimContext()
  const [bindings, setBindings] = useState(() => getActiveBindings())

  useEffect(() => {
    return subscribe(() => setBindings(getActiveBindings()))
  }, [subscribe, getActiveBindings])

  return bindings
}

/** Returns the current vim mode and pending key chord for status bar display. */
export function useVimStatus(): { mode: string; sequence: string } {
  const { getSequence, subscribe } = useVimContext()
  const [sequence, setSequence] = useState(() => getSequence())

  useEffect(() => {
    return subscribe(() => setSequence(getSequence()))
  }, [subscribe, getSequence])

  return { mode: 'NORMAL', sequence }
}
