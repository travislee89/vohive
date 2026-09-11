import { watch } from 'vue'
import { useNotificationsStore } from '../stores/notifications'

const BASE_TITLE = 'VoHive'
const MAX_DISPLAY_COUNT = 99

export function useDocumentTitleBadge() {
  const store = useNotificationsStore()

  watch(
    () => store.totalUnread,
    (count) => {
      document.title = count > 0 ? `(${count > MAX_DISPLAY_COUNT ? '99+' : count}) ${BASE_TITLE}` : BASE_TITLE
    },
    { immediate: true }
  )
}
