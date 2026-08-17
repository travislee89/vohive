export type AutomationActionType = 'reboot' | 'sms'
export type AutomationTriggerType = 'fixed_schedule' | 'interval'
export type AutomationIntervalUnit = 'hours' | 'days'
export type AutomationRunStatus = 'success' | 'failed'
export type AutomationTriggerSource = 'schedule' | 'manual'

export type AutomationTask = {
  id: string
  name: string
  enabled: boolean
  action_type: AutomationActionType
  device_id: string
  sms_phone?: string
  sms_content?: string
  sms_delay_min_sec?: number
  sms_delay_max_sec?: number
  sms_retry_count?: number
  trigger_type: AutomationTriggerType
  fixed_times?: string[]
  weekdays?: number[]
  interval_value?: number
  interval_unit?: AutomationIntervalUnit
  last_run_at?: string
  next_run_at?: string
  created_at: string
  updated_at: string
}

export type AutomationTaskPayload = {
  name?: string
  enabled?: boolean
  action_type?: AutomationActionType
  device_id?: string
  sms_phone?: string
  sms_content?: string
  sms_delay_min_sec?: number
  sms_delay_max_sec?: number
  sms_retry_count?: number
  trigger_type?: AutomationTriggerType
  fixed_times?: string[]
  weekdays?: number[]
  interval_value?: number
  interval_unit?: AutomationIntervalUnit
}

export type AutomationTaskListResponse = {
  tasks: AutomationTask[]
  total: number
}

export type AutomationRunLog = {
  id: number
  task_id: string
  task_name: string
  action_type: AutomationActionType
  device_id?: string
  trigger_source: AutomationTriggerSource
  status: AutomationRunStatus
  result_summary: string
  error_detail?: string
  timestamp: string
}

export type AutomationLogListParams = {
  page?: number
  page_size?: number
  action_type?: string
  status?: string
  start?: string
  end?: string
  q?: string
}

export type AutomationLogListResponse = {
  logs: AutomationRunLog[]
  total: number
  page: number
  page_size: number
}

export type AutomationLogRetention = {
  auto_cleanup_enabled: boolean
  retention_days: number
}
