import { api } from '../stores/auth'
import { callService } from './http'

export type OpenCellIDSettings = {
  key: string
}

export type OpenCellIDLocateRequest = {
  mcc: string
  mnc: string
  lac: string // 十六进制，如 "01F9"
  cid: string // 十六进制，如 "D08E01"
  network_mode: string // GSM|WCDMA|LTE|NR
}

export type OpenCellIDLocateResponse = {
  lat: number
  lon: number
  range: number
  samples: number
  radio: string
}

export const opencellidService = {
  getSettings() {
    return callService(async () => {
      const res = await api.get('/settings/opencellid')
      return (res.data || {}) as OpenCellIDSettings
    })
  },
  saveSettings(payload: OpenCellIDSettings) {
    return callService(async () => {
      await api.put('/settings/opencellid', payload)
      return true
    })
  },
  locate(payload: OpenCellIDLocateRequest) {
    return callService(async () => {
      const res = await api.post<OpenCellIDLocateResponse>('/settings/opencellid/locate', payload)
      return res.data
    })
  }
}
