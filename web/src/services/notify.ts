import { api } from '../stores/auth'
import { callService } from './http'
import type {
  NotifyLogListParams,
  NotifyLogListResponse,
  NotifyLogRetention,
  NotifyRule,
  NotifyRuleListResponse,
  NotifyRulePayload
} from '../types/notify'

export const notifyService = {
  listRules(messageType?: string) {
    return callService(async () => {
      const res = await api.get('/notify/rules', { params: messageType ? { type: messageType } : undefined })
      return res.data as NotifyRuleListResponse
    })
  },
  createRule(payload: NotifyRulePayload) {
    return callService(async () => {
      const res = await api.post('/notify/rules', payload)
      return res.data as NotifyRule
    })
  },
  updateRule(id: string, payload: Partial<NotifyRulePayload>) {
    return callService(async () => {
      const res = await api.put(`/notify/rules/${id}`, payload)
      return res.data as NotifyRule
    })
  },
  deleteRule(id: string) {
    return callService(async () => {
      await api.delete(`/notify/rules/${id}`)
      return true
    })
  },
  listLogs(params: NotifyLogListParams) {
    return callService(async () => {
      const res = await api.get('/notify/logs', { params })
      return res.data as NotifyLogListResponse
    })
  },
  cleanupLogs(params: { type?: string; status?: string; before?: string }) {
    return callService(async () => {
      const res = await api.delete('/notify/logs', { params })
      return res.data as { deleted: number }
    })
  },
  getRetention() {
    return callService(async () => {
      const res = await api.get('/notify/logs/retention')
      return res.data as NotifyLogRetention
    })
  },
  updateRetention(payload: NotifyLogRetention) {
    return callService(async () => {
      const res = await api.put('/notify/logs/retention', payload)
      return res.data as NotifyLogRetention
    })
  }
}
