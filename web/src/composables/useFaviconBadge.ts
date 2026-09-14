import { watch } from 'vue'
import { useNotificationsStore } from '../stores/notifications'
import { useAuthStore } from '../stores/auth'

const FAVICON_HREF = '/favicon.svg'
const CANVAS_SIZE = 64
const MAX_DISPLAY_COUNT = 99

let baseImagePromise: Promise<HTMLImageElement> | null = null

function loadBaseImage(): Promise<HTMLImageElement> {
  if (!baseImagePromise) {
    baseImagePromise = new Promise((resolve, reject) => {
      const img = new Image()
      img.onload = () => resolve(img)
      img.onerror = reject
      img.src = FAVICON_HREF
    })
  }
  return baseImagePromise
}

function getFaviconLink(): HTMLLinkElement {
  let link = document.querySelector<HTMLLinkElement>('link[rel="icon"]')
  if (!link) {
    link = document.createElement('link')
    link.rel = 'icon'
    document.head.appendChild(link)
  }
  return link
}

async function renderFavicon(count: number, grayscale: boolean) {
  const link = getFaviconLink()
  if (!grayscale && count <= 0) {
    link.href = FAVICON_HREF
    return
  }

  const img = await loadBaseImage()
  const canvas = document.createElement('canvas')
  canvas.width = CANVAS_SIZE
  canvas.height = CANVAS_SIZE
  const ctx = canvas.getContext('2d')
  if (!ctx) return

  ctx.filter = grayscale ? 'grayscale(1) opacity(0.55)' : 'none'
  ctx.drawImage(img, 0, 0, CANVAS_SIZE, CANVAS_SIZE)
  ctx.filter = 'none'

  if (!grayscale && count > 0) {
    const text = count > MAX_DISPLAY_COUNT ? '99+' : String(count)
    const radius = CANVAS_SIZE * (text.length > 2 ? 0.36 : 0.32)
    const cx = CANVAS_SIZE - radius * 0.85
    const cy = CANVAS_SIZE - radius * 0.85

    ctx.beginPath()
    ctx.arc(cx, cy, radius, 0, Math.PI * 2)
    ctx.fillStyle = '#ef4444'
    ctx.fill()

    ctx.fillStyle = '#ffffff'
    ctx.font = `700 ${text.length > 2 ? radius * 0.95 : radius * 1.2}px sans-serif`
    ctx.textAlign = 'center'
    ctx.textBaseline = 'middle'
    ctx.fillText(text, cx, cy + 1)
  }

  link.href = canvas.toDataURL('image/png')
}

export function useFaviconBadge() {
  const store = useNotificationsStore()
  const auth = useAuthStore()

  watch(
    [() => store.totalUnread, () => auth.isAuthenticated],
    ([count, isAuthenticated]) => {
      void renderFavicon(count, !isAuthenticated)
    },
    { immediate: true }
  )
}
