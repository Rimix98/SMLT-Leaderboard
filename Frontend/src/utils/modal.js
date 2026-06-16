export function makeOverlayClose(closeFn) {
  let mousedownOverlay = false
  return {
    onMousedown(e) {
      mousedownOverlay = e.target === e.currentTarget
    },
    onMouseup(e) {
      if (mousedownOverlay && e.target === e.currentTarget) {
        closeFn()
      }
      mousedownOverlay = false
    }
  }
}
