import { defineStore } from 'pinia'
import { computed, ref } from 'vue'
import type { AppError } from '../types/domain'
import type { NotificationFeedItem } from '../types/api'
import { notificationCenterService } from '../services/notificationCenter'

export const useNotificationsStore = defineStore('notifications', () => {
  const smsUnread = ref(0)
  const callsUnread = ref(0)
  const feed = ref<NotificationFeedItem[]>([])

  const totalUnread = computed(() => smsUnread.value + callsUnread.value)

  const loading = ref(false)
  const lastOkAt = ref<number | null>(null)
  const error = ref<AppError | null>(null)

  async function fetchSummary() {
    const result = await notificationCenterService.getSummary()
    if (result.ok) {
      smsUnread.value = result.data.sms_unread
      callsUnread.value = result.data.calls_unread
      error.value = null
    } else {
      error.value = result.error
    }
    return result
  }

  async function fetchFeed(limit = 20) {
    const result = await notificationCenterService.getFeed(limit)
    if (result.ok) feed.value = result.data
    return result
  }

  async function refreshAll() {
    loading.value = true
    try {
      await fetchSummary()
    } finally {
      loading.value = false
    }
    lastOkAt.value = Date.now()
  }

  function removeFromFeed(predicate: (item: NotificationFeedItem) => boolean) {
    feed.value = feed.value.filter(item => !predicate(item))
  }

  async function markSmsRead(imsi: string | undefined, peer: string) {
    smsUnread.value = Math.max(0, smsUnread.value - (feed.value.find(i => i.kind === 'sms' && i.peer === peer)?.unread_count || 0))
    removeFromFeed(item => item.kind === 'sms' && item.peer === peer)
    const result = await notificationCenterService.markSmsRead({ imsi, peer })
    await fetchSummary()
    return result
  }

  async function markCallRead(id: number) {
    removeFromFeed(item => item.kind === 'call' && item.call_id === id)
    if (callsUnread.value > 0) callsUnread.value -= 1
    const result = await notificationCenterService.markCallsRead({ id })
    await fetchSummary()
    return result
  }

  async function markDeviceCallsRead(deviceId: string) {
    removeFromFeed(item => item.kind === 'call' && item.device_id === deviceId)
    const result = await notificationCenterService.markCallsRead({ device_id: deviceId })
    await fetchSummary()
    return result
  }

  async function clearAll() {
    smsUnread.value = 0
    callsUnread.value = 0
    feed.value = []
    const result = await notificationCenterService.clearAll()
    await fetchSummary()
    return result
  }

  return {
    smsUnread,
    callsUnread,
    totalUnread,
    feed,
    loading,
    lastOkAt,
    error,
    fetchSummary,
    fetchFeed,
    refreshAll,
    markSmsRead,
    markCallRead,
    markDeviceCallsRead,
    clearAll
  }
})
