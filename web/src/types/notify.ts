export type NotifyChannelKey = 'telegram' | 'feishu' | 'qq' | 'webhook' | 'bark' | 'email' | 'pushplus'

export type NotifyMatchField = 'any' | 'content' | 'sender'
export type NotifyMatchMethod = 'all' | 'contains' | 'not_contains' | 'equals' | 'regex'
export type NotifyBodyMode = 'plain' | 'custom_json'
export type NotifyLogStatus = 'success' | 'unmatched' | 'failed'

export type NotifyRule = {
  id: string
  message_type: string
  name: string
  enabled: boolean
  priority: number
  match_field: NotifyMatchField
  match_method: NotifyMatchMethod
  match_content: string
  target_channels: NotifyChannelKey[]
  title_template: string
  body_mode: NotifyBodyMode
  body_template: string
  is_default: boolean
  created_at: string
  updated_at: string
}

export type NotifyRulePayload = {
  message_type?: string
  name?: string
  enabled?: boolean
  priority?: number
  match_field?: NotifyMatchField
  match_method?: NotifyMatchMethod
  match_content?: string
  target_channels?: NotifyChannelKey[]
  title_template?: string
  body_mode?: NotifyBodyMode
  body_template?: string
}

export type NotifyRuleListResponse = {
  rules: NotifyRule[]
  enabled: number
  total: number
}

export type NotifyLog = {
  id: number
  message_type: string
  status: NotifyLogStatus
  content_summary: string
  matched_rule_id?: string
  matched_rule_name?: string
  channel?: string
  error_detail?: string
  device_id?: string
  sms_id?: number
  timestamp: string
}

export type NotifyLogListParams = {
  page?: number
  page_size?: number
  type?: string
  status?: string
  start?: string
  end?: string
  q?: string
}

export type NotifyLogListResponse = {
  logs: NotifyLog[]
  total: number
  page: number
  page_size: number
}

export type NotifyLogRetention = {
  auto_cleanup_enabled: boolean
  retention_days: number
}
