export function debounce<T extends (...args: never[]) => unknown>(fn: T, ms = 250): T {
  let timer: ReturnType<typeof setTimeout>
  return function (this: unknown, ...args: Parameters<T>) {
    clearTimeout(timer)
    timer = setTimeout(() => fn.apply(this, args), ms)
  } as unknown as T
}
