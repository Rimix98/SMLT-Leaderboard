interface OverlayCloseHandlers {
  onMousedown(e: MouseEvent): void
  onMouseup(e: MouseEvent): void
}

export function makeOverlayClose(closeFn: () => void): OverlayCloseHandlers {
  let mousedownOverlay = false
  return {
    onMousedown(e: MouseEvent) {
      mousedownOverlay = e.target === e.currentTarget
    },
    onMouseup(e: MouseEvent) {
      if (mousedownOverlay && e.target === e.currentTarget) {
        closeFn()
      }
      mousedownOverlay = false
    }
  }
}
