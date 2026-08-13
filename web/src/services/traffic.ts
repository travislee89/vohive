import { api } from '../stores/auth'
import { callService } from './http'

export type TrafficRange = 'day' | 'week' | 'month'
export type TrafficBucket = {
  bucket: string
  period_start?: string
  rx_bytes: number
  tx_bytes: number
  total_bytes: number
}
export type TrafficChart = {
  timestamps: string[]
  period_starts?: string[]
  devices: string[]
  series: Record<string, number[]>
}
export type TrafficTotal = {
  rx_bytes: number
  tx_bytes: number
  total_bytes: number
}
export type TrafficRangeTotals = {
  day: TrafficTotal
  week: TrafficTotal
  month: TrafficTotal
}
export const EMPTY_TRAFFIC_TOTAL: TrafficTotal = { rx_bytes: 0, tx_bytes: 0, total_bytes: 0 }
export const EMPTY_TRAFFIC_RANGE_TOTALS: TrafficRangeTotals = {
  day: EMPTY_TRAFFIC_TOTAL,
  week: EMPTY_TRAFFIC_TOTAL,
  month: EMPTY_TRAFFIC_TOTAL
}
export type TrafficAnalysis = { buckets: TrafficBucket[]; chart: TrafficChart | null; summary: TrafficRangeTotals }

export function createEmptyTrafficAnalysis(): TrafficAnalysis {
  return { buckets: [], chart: null, summary: EMPTY_TRAFFIC_RANGE_TOTALS }
}

export const trafficService = {
  getAnalysis(range: TrafficRange, deviceId?: string, signal?: AbortSignal) {
    return callService(async () => {
      const params: Record<string, string> = { range }
      if (typeof deviceId === 'string' && deviceId.trim()) {
        params.device_id = deviceId.trim()
      }
      const res = await api.get('/traffic/analysis', { params, signal })
      const data = res?.data
      const summary = data?.summary || EMPTY_TRAFFIC_RANGE_TOTALS
      return {
        buckets: (data?.buckets || []) as TrafficBucket[],
        chart: (data?.chart || null) as TrafficChart | null,
        summary: {
          day: summary.day || EMPTY_TRAFFIC_TOTAL,
          week: summary.week || EMPTY_TRAFFIC_TOTAL,
          month: summary.month || EMPTY_TRAFFIC_TOTAL
        }
      } as TrafficAnalysis
    })
  },

  rollup(horizonDays?: number) {
    return callService(async () => {
      const params = horizonDays ? { horizon: horizonDays } : undefined
      const res = await api.post('/traffic/rollup', undefined, { params })
      return res?.data?.rollup as { days: number; weeks: number; months: number; horizon_days: number } | undefined
    })
  }
}
