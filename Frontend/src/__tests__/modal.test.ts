import { describe, it, expect, vi } from 'vitest'
import { makeOverlayClose } from '../utils/modal'

describe('makeOverlayClose', () => {
  it('returns object with onMousedown and onMouseup', () => {
    const closeFn = vi.fn()
    const overlay = makeOverlayClose(closeFn)

    expect(overlay).toHaveProperty('onMousedown')
    expect(overlay).toHaveProperty('onMouseup')
    expect(typeof overlay.onMousedown).toBe('function')
    expect(typeof overlay.onMouseup).toBe('function')
  })

  it('calls closeFn when clicking overlay directly', () => {
    const closeFn = vi.fn()
    const overlay = makeOverlayClose(closeFn)

    const currentTarget = { id: 'overlay' }
    const mousedownEvent = { target: currentTarget, currentTarget }
    overlay.onMousedown(mousedownEvent as unknown as MouseEvent)

    const mouseupEvent = { target: currentTarget, currentTarget }
    overlay.onMouseup(mouseupEvent as unknown as MouseEvent)

    expect(closeFn).toHaveBeenCalledTimes(1)
  })

  it('does NOT call closeFn when clicking child element', () => {
    const closeFn = vi.fn()
    const overlay = makeOverlayClose(closeFn)

    const currentTarget = { id: 'overlay' }
    const child = { id: 'child', parentElement: currentTarget }

    overlay.onMousedown({ target: child, currentTarget } as unknown as MouseEvent)
    overlay.onMouseup({ target: child, currentTarget } as unknown as MouseEvent)

    expect(closeFn).not.toHaveBeenCalled()
  })

  it('does NOT call closeFn when mousedown on child, mouseup on overlay', () => {
    const closeFn = vi.fn()
    const overlay = makeOverlayClose(closeFn)

    const currentTarget = { id: 'overlay' }
    const child = { id: 'child' }

    overlay.onMousedown({ target: child, currentTarget } as unknown as MouseEvent)
    overlay.onMouseup({ target: currentTarget, currentTarget } as unknown as MouseEvent)

    expect(closeFn).not.toHaveBeenCalled()
  })

  it('resets mousedown state after mouseup', () => {
    const closeFn = vi.fn()
    const overlay = makeOverlayClose(closeFn)

    const currentTarget = { id: 'overlay' }

    overlay.onMousedown({ target: currentTarget, currentTarget } as unknown as MouseEvent)
    overlay.onMouseup({ target: currentTarget, currentTarget } as unknown as MouseEvent)
    expect(closeFn).toHaveBeenCalledTimes(1)

    overlay.onMouseup({ target: currentTarget, currentTarget } as unknown as MouseEvent)
    expect(closeFn).toHaveBeenCalledTimes(1)
  })

  it('multiple overlays are independent', () => {
    const closeFn1 = vi.fn()
    const closeFn2 = vi.fn()
    const overlay1 = makeOverlayClose(closeFn1)
    const overlay2 = makeOverlayClose(closeFn2)

    const target1 = { id: 'o1' }
    const target2 = { id: 'o2' }

    overlay1.onMousedown({ target: target1, currentTarget: target1 } as unknown as MouseEvent)
    overlay2.onMousedown({ target: target2, currentTarget: target2 } as unknown as MouseEvent)

    overlay1.onMouseup({ target: target1, currentTarget: target1 } as unknown as MouseEvent)
    expect(closeFn1).toHaveBeenCalledTimes(1)
    expect(closeFn2).not.toHaveBeenCalled()
  })
})
