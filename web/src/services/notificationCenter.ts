import { api } from '../stores/auth'
import { callService } from './http'
import type { NotificationFeedItem, NotificationSummary } from '../types/api'

export type MarkSmsReadPayload = {
  imsi?: string
  peer: string
}

export type MarkCallsReadPayload = {
  id?: number
  device_id?: string
}

export const notificationCenterService = {
  async getSummary() {
    return callService(async () => {
      const res = await api.get<NotificationSummary>('/notification-center/summary')
      return res.data
    })
  },

  async getFeed(limit = 20) {
    return callService(async () => {
      const res = await api.get<{ items: NotificationFeedItem[] }>('/notification-center/feed', { params: { limit } })
      return res.data.items
    })
  },

  async markSmsRead(payload: MarkSmsReadPayload) {
    return callService(async () => {
      await api.post('/notification-center/sms/read', payload)
    })
  },

  async markCallsRead(payload: MarkCallsReadPayload) {
    return callService(async () => {
      await api.post('/notification-center/calls/read', payload)
    })
  },

  async clearAll() {
    return callService(async () => {
      await api.post('/notification-center/clear-all')
    })
  }
}
