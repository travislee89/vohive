// 统一的日期/时间格式化工具。
// 项目前端统一使用 yyyy-mm-dd（及 yyyy-mm-dd HH:MM:SS），避免系统 locale
// 把日期渲染成 mm/dd/yyyy（如 8/11/2026）。

function pad2(v: number): string {
  return String(v).padStart(2, '0')
}

// 解析任意可转 Date 的输入（字符串/数字/Date），无效返回 null。
export function parseDate(value: unknown): Date | null {
  if (value == null || value === '') return null
  const d = new Date(value as string | number | Date)
  return Number.isFinite(d.getTime()) ? d : null
}

// 仅日期：yyyy-mm-dd
export function formatISODate(value: unknown): string {
  const d = parseDate(value)
  if (!d) return '--'
  return `${d.getFullYear()}-${pad2(d.getMonth() + 1)}-${pad2(d.getDate())}`
}

// 日期 + 时间：yyyy-mm-dd HH:MM:SS
export function formatISODateTime(value: unknown): string {
  const d = parseDate(value)
  if (!d) return '--'
  return `${formatISODate(d)} ${pad2(d.getHours())}:${pad2(d.getMinutes())}:${pad2(d.getSeconds())}`
}

// 仅时间：HH:MM:SS（24 小时制，替代 toLocaleTimeString 的 AM/PM）
export function formatISOTime(value: unknown): string {
  const d = parseDate(value)
  if (!d) return '--'
  return `${pad2(d.getHours())}:${pad2(d.getMinutes())}:${pad2(d.getSeconds())}`
}

// 整点小时：HH:00（小时桶刻度）
export function formatHourStart(value: unknown): string {
  const d = parseDate(value)
  if (!d) return '--'
  return `${pad2(d.getHours())}:00`
}

// 月-日：MM-dd（图表短刻度）
export function formatMonthDay(value: unknown): string {
  const d = parseDate(value)
  if (!d) return '--'
  return `${pad2(d.getMonth() + 1)}-${pad2(d.getDate())}`
}

// 日期 + 整点小时：yyyy-mm-dd HH:00（小时桶）
export function formatISODateHour(value: unknown): string {
  const d = parseDate(value)
  if (!d) return '--'
  return `${formatISODate(d)} ${pad2(d.getHours())}:00`
}

// 紧凑文件戳：yyyyMMdd-HHmmss（无分隔符，用于导出文件名）
export function formatCompactStamp(value: unknown): string {
  const d = parseDate(value)
  if (!d) return 'unknown'
  return `${d.getFullYear()}${pad2(d.getMonth() + 1)}${pad2(d.getDate())}-${pad2(d.getHours())}${pad2(d.getMinutes())}${pad2(d.getSeconds())}`
}

