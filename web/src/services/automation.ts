import { api } from '../stores/auth'
import { callService } from './http'
import type {
  AutomationLogListParams,
  AutomationLogListResponse,
  AutomationLogRetention,
  AutomationTask,
  AutomationTaskListResponse,
  AutomationTaskPayload
} from '../types/automation'

export const automationService = {
  listTasks() {
    return callService(async () => {
      const res = await api.get('/automation/tasks')
      return res.data as AutomationTaskListResponse
    })
  },
  createTask(payload: AutomationTaskPayload) {
    return callService(async () => {
      const res = await api.post('/automation/tasks', payload)
      return res.data as AutomationTask
    })
  },
  updateTask(id: string, payload: Partial<AutomationTaskPayload>) {
    return callService(async () => {
      const res = await api.put(`/automation/tasks/${id}`, payload)
      return res.data as AutomationTask
    })
  },
  toggleTask(id: string, enabled: boolean) {
    return callService(async () => {
      const res = await api.patch(`/automation/tasks/${id}`, { enabled })
      return res.data as AutomationTask
    })
  },
  deleteTask(id: string) {
    return callService(async () => {
      await api.delete(`/automation/tasks/${id}`)
      return true
    })
  },
  runTask(id: string) {
    return callService(async () => {
      const res = await api.post(`/automation/tasks/${id}/run`)
      return res.data as { status: string; message: string }
    })
  },
  listLogs(params: AutomationLogListParams) {
    return callService(async () => {
      const res = await api.get('/automation/logs', { params })
      return res.data as AutomationLogListResponse
    })
  },
  cleanupLogs(params: { action_type?: string; status?: string; before?: string }) {
    return callService(async () => {
      const res = await api.delete('/automation/logs', { params })
      return res.data as { deleted: number }
    })
  },
  getRetention() {
    return callService(async () => {
      const res = await api.get('/automation/logs/retention')
      return res.data as AutomationLogRetention
    })
  },
  updateRetention(payload: AutomationLogRetention) {
    return callService(async () => {
      const res = await api.put('/automation/logs/retention', payload)
      return res.data as AutomationLogRetention
    })
  }
}
