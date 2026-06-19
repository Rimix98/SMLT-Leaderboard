import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { debounce } from '../utils/debounce'

describe('debounce', () => {
  beforeEach(() => {
    vi.useFakeTimers()
  })

  afterEach(() => {
    vi.useRealTimers()
  })

  it('delays function execution', () => {
    const fn = vi.fn()
    const debounced = debounce(fn, 250)

    debounced()
    expect(fn).not.toHaveBeenCalled()

    vi.advanceTimersByTime(250)
    expect(fn).toHaveBeenCalledTimes(1)
  })

  it('resets timer on subsequent calls', () => {
    const fn = vi.fn()
    const debounced = debounce(fn, 250)

    debounced()
    vi.advanceTimersByTime(100)
    debounced()
    vi.advanceTimersByTime(100)
    expect(fn).not.toHaveBeenCalled()

    vi.advanceTimersByTime(150)
    expect(fn).toHaveBeenCalledTimes(1)
  })

  it('passes arguments to function', () => {
    const fn = vi.fn()
    const debounced = debounce(fn, 100)

    debounced('arg1', 'arg2')
    vi.advanceTimersByTime(100)

    expect(fn).toHaveBeenCalledWith('arg1', 'arg2')
  })

  it('uses default delay of 250ms', () => {
    const fn = vi.fn()
    const debounced = debounce(fn)

    debounced()
    vi.advanceTimersByTime(200)
    expect(fn).not.toHaveBeenCalled()

    vi.advanceTimersByTime(50)
    expect(fn).toHaveBeenCalledTimes(1)
  })

  it('preserves this context', () => {
    const obj = { value: 42 }
    const fn = vi.fn(function (this: typeof obj) { return this.value })
    const debounced = debounce(fn, 100)

    debounced.call(obj)
    vi.advanceTimersByTime(100)

    expect(fn).toHaveReturnedWith(42)
  })
})
