import { describe, it, expect, vi, afterEach } from 'vitest'
import { render, renderHook, act, cleanup } from '@testing-library/react'
import React from 'react'
import { VimProvider, useVimBindings, type VimBinding } from './vim'

afterEach(cleanup)

// Dispatch a keydown event on document (where the vim listener lives)
function pressKey(key: string) {
  document.dispatchEvent(new KeyboardEvent('keydown', { key, bubbles: true }))
}

// Dispatch a keydown event on a specific element (tests form-bypass logic)
function pressKeyOn(element: EventTarget, key: string) {
  element.dispatchEvent(new KeyboardEvent('keydown', { key, bubbles: true }))
}

const wrapper = ({ children }: { children: React.ReactNode }) => (
  <VimProvider>{children}</VimProvider>
)

// ─── Single-key bindings ──────────────────────────────────────────────────────

describe('single-key bindings', () => {
  it('calls handler when matching key is pressed', () => {
    const handler = vi.fn()
    renderHook(() => useVimBindings([{ key: 'j', description: 'down', handler }]), { wrapper })
    act(() => pressKey('j'))
    expect(handler).toHaveBeenCalledOnce()
  })

  it('does not call handler for a non-matching key', () => {
    const handler = vi.fn()
    renderHook(() => useVimBindings([{ key: 'j', description: 'down', handler }]), { wrapper })
    act(() => pressKey('k'))
    expect(handler).not.toHaveBeenCalled()
  })

  it('calls the correct handler among multiple bindings', () => {
    const jHandler = vi.fn()
    const kHandler = vi.fn()
    renderHook(
      () => useVimBindings([
        { key: 'j', description: 'down', handler: jHandler },
        { key: 'k', description: 'up', handler: kHandler },
      ]),
      { wrapper },
    )
    act(() => pressKey('k'))
    expect(kHandler).toHaveBeenCalledOnce()
    expect(jHandler).not.toHaveBeenCalled()
  })
})

// ─── Key sequences ────────────────────────────────────────────────────────────

describe('key sequences', () => {
  it('fires handler for a multi-char sequence', () => {
    const handler = vi.fn()
    renderHook(() => useVimBindings([{ key: 'gg', description: 'top', handler }]), { wrapper })
    act(() => { pressKey('g'); pressKey('g') })
    expect(handler).toHaveBeenCalledOnce()
  })

  it('does not fire on the first key of a partial sequence', () => {
    const handler = vi.fn()
    renderHook(() => useVimBindings([{ key: 'gg', description: 'top', handler }]), { wrapper })
    act(() => pressKey('g'))
    expect(handler).not.toHaveBeenCalled()
  })

  it('clears sequence buffer after a match so the sequence can fire again', () => {
    const handler = vi.fn()
    renderHook(() => useVimBindings([{ key: 'gg', description: 'top', handler }]), { wrapper })
    act(() => { pressKey('g'); pressKey('g') })
    act(() => { pressKey('g'); pressKey('g') })
    expect(handler).toHaveBeenCalledTimes(2)
  })

  it('falls back to a single-key binding after a non-completing prefix', () => {
    const ggHandler = vi.fn()
    const jHandler = vi.fn()
    renderHook(
      () => useVimBindings([
        { key: 'gg', description: 'top', handler: ggHandler },
        { key: 'j', description: 'down', handler: jHandler },
      ]),
      { wrapper },
    )
    // 'g' is a prefix of 'gg'; 'j' doesn't complete it — should retry 'j' alone
    act(() => { pressKey('g'); pressKey('j') })
    expect(ggHandler).not.toHaveBeenCalled()
    expect(jHandler).toHaveBeenCalledOnce()
  })

  it('clears the sequence buffer after a 1 s timeout', () => {
    vi.useFakeTimers()
    const handler = vi.fn()
    renderHook(() => useVimBindings([{ key: 'gg', description: 'top', handler }]), { wrapper })

    act(() => pressKey('g'))
    expect(handler).not.toHaveBeenCalled()

    // After timeout the buffer resets; a second 'g' starts a fresh sequence
    act(() => vi.advanceTimersByTime(1100))
    act(() => pressKey('g'))
    expect(handler).not.toHaveBeenCalled() // still no complete 'gg'

    vi.useRealTimers()
  })
})

// ─── Layer stack ──────────────────────────────────────────────────────────────

describe('layer stack', () => {
  function Layer({ bindings }: { bindings: VimBinding[] }) {
    useVimBindings(bindings)
    return null
  }

  it('only the topmost layer handles keys', () => {
    const h1 = vi.fn()
    const h2 = vi.fn()
    render(
      <VimProvider>
        <Layer bindings={[{ key: 'j', description: 'l1', handler: h1 }]} />
        <Layer bindings={[{ key: 'j', description: 'l2', handler: h2 }]} />
      </VimProvider>,
    )
    act(() => pressKey('j'))
    expect(h2).toHaveBeenCalledOnce()
    expect(h1).not.toHaveBeenCalled()
  })

  it('resumes the lower layer after the upper layer unmounts', () => {
    const h1 = vi.fn()
    const h2 = vi.fn()
    const { rerender } = render(
      <VimProvider>
        <Layer bindings={[{ key: 'j', description: 'l1', handler: h1 }]} />
        <Layer bindings={[{ key: 'j', description: 'l2', handler: h2 }]} />
      </VimProvider>,
    )
    act(() => pressKey('j'))
    expect(h2).toHaveBeenCalledOnce()

    rerender(
      <VimProvider>
        <Layer bindings={[{ key: 'j', description: 'l1', handler: h1 }]} />
      </VimProvider>,
    )

    act(() => pressKey('j'))
    expect(h1).toHaveBeenCalledOnce()
  })
})

// ─── Form element bypass ──────────────────────────────────────────────────────

describe('form element bypass', () => {
  it('ignores keydown originating from INPUT', () => {
    const handler = vi.fn()
    renderHook(() => useVimBindings([{ key: 'j', description: 'down', handler }]), { wrapper })
    const input = document.createElement('input')
    document.body.appendChild(input)
    act(() => pressKeyOn(input, 'j'))
    document.body.removeChild(input)
    expect(handler).not.toHaveBeenCalled()
  })

  it('ignores keydown originating from TEXTAREA', () => {
    const handler = vi.fn()
    renderHook(() => useVimBindings([{ key: 'j', description: 'down', handler }]), { wrapper })
    const ta = document.createElement('textarea')
    document.body.appendChild(ta)
    act(() => pressKeyOn(ta, 'j'))
    document.body.removeChild(ta)
    expect(handler).not.toHaveBeenCalled()
  })

  it('ignores keydown originating from a contentEditable element', () => {
    const handler = vi.fn()
    renderHook(() => useVimBindings([{ key: 'j', description: 'down', handler }]), { wrapper })
    const div = document.createElement('div')
    div.contentEditable = 'true'
    document.body.appendChild(div)
    act(() => pressKeyOn(div, 'j'))
    document.body.removeChild(div)
    expect(handler).not.toHaveBeenCalled()
  })

  it('handles keydown from a regular element', () => {
    const handler = vi.fn()
    renderHook(() => useVimBindings([{ key: 'j', description: 'down', handler }]), { wrapper })
    const div = document.createElement('div')
    document.body.appendChild(div)
    act(() => pressKeyOn(div, 'j'))
    document.body.removeChild(div)
    expect(handler).toHaveBeenCalledOnce()
  })
})

// ─── Handler freshness ────────────────────────────────────────────────────────

describe('handler freshness', () => {
  it('calls the latest handler after a re-render without re-registering the layer', () => {
    const h1 = vi.fn()
    const h2 = vi.fn()
    const { rerender } = renderHook(
      ({ handler }: { handler: () => void }) =>
        useVimBindings([{ key: 'j', description: 'down', handler }]),
      { wrapper, initialProps: { handler: h1 } },
    )

    act(() => pressKey('j'))
    expect(h1).toHaveBeenCalledOnce()

    rerender({ handler: h2 })

    act(() => pressKey('j'))
    expect(h2).toHaveBeenCalledOnce()
    expect(h1).toHaveBeenCalledOnce() // still just once — not called again
  })
})

// ─── Error handling ───────────────────────────────────────────────────────────

describe('error handling', () => {
  it('throws when useVimBindings is used outside VimProvider', () => {
    const spy = vi.spyOn(console, 'error').mockImplementation(() => {})
    expect(() => renderHook(() => useVimBindings([]))).toThrow(
      'useVimBindings must be used within VimProvider',
    )
    spy.mockRestore()
  })
})
