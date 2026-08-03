import { api } from '../stores/auth'
import { callService } from './http'
import type { CSCallListResponse } from '../types/api'

export const callsService = {
  /**
   * 查询指定设备的当前活跃呼叫列表
   */
  async list(deviceId: string) {
    return callService(async () => {
      const res = await api.get<CSCallListResponse>(`/devices/${deviceId}/calls`)
      return res.data
    })
  }
}