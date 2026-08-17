export type MccMncRow = {
  mcc: string
  mnc: string
  iso: string
  country: string
  country_code: string
  network: string
}

const TABLE_URL = 'https://raw.githubusercontent.com/musalbas/mcc-mnc-table/refs/heads/master/mcc-mnc-table.json'
const STORAGE_KEY = 'go-4gproxy:mcc-mnc-table:v1'
const CACHE_TTL_MS = 7 * 24 * 60 * 60 * 1000

type CachePayload = {
  fetched_at: number
  rows: MccMncRow[]
}

let indexPromise: Promise<Map<string, MccMncRow>> | null = null

function normalizeCode(s: string): string {
  return String(s || '').trim()
}

function buildIndex(rows: MccMncRow[]): Map<string, MccMncRow> {
  const idx = new Map<string, MccMncRow>()
  for (const r of rows) {
    const mcc = normalizeCode(r?.mcc)
    const mnc = normalizeCode(r?.mnc)
    if (!mcc || !mnc) continue
    const key = `${mcc}${mnc}`
    if (!idx.has(key)) {
      idx.set(key, {
        mcc,
        mnc,
        iso: normalizeCode(r?.iso).toLowerCase(),
        country: normalizeCode(r?.country),
        country_code: normalizeCode(r?.country_code),
        network: normalizeCode(r?.network)
      })
      continue
    }
    const cur = idx.get(key)!
    if (!cur.network && r?.network) {
      cur.network = normalizeCode(r.network)
    }
  }
  return idx
}

function readCache(): CachePayload | null {
  try {
    const raw = localStorage.getItem(STORAGE_KEY)
    if (!raw) return null
    const data = JSON.parse(raw) as CachePayload
    if (!data || !Array.isArray(data.rows) || typeof data.fetched_at !== 'number') return null
    return data
  } catch {
    return null
  }
}

function writeCache(rows: MccMncRow[]) {
  try {
    const payload: CachePayload = { fetched_at: Date.now(), rows }
    localStorage.setItem(STORAGE_KEY, JSON.stringify(payload))
  } catch {
    // Ignore cache write failures (private mode/quota/security policy).
  }
}

async function fetchRows(): Promise<MccMncRow[]> {
  const res = await fetch(TABLE_URL, { method: 'GET' })
  if (!res.ok) throw new Error(`mcc-mnc-table fetch failed: ${res.status}`)
  const data = await res.json()
  if (!Array.isArray(data)) return []
  const out: MccMncRow[] = []
  for (const it of data) {
    if (!it || typeof it !== 'object') continue
    const r = it as Record<string, unknown>
    const mcc = typeof r.mcc === 'string' ? r.mcc : ''
    const mnc = typeof r.mnc === 'string' ? r.mnc : ''
    if (!mcc || !mnc) continue
    out.push({
      mcc,
      mnc,
      iso: typeof r.iso === 'string' ? r.iso : '',
      country: typeof r.country === 'string' ? r.country : '',
      country_code: typeof r.country_code === 'string' ? r.country_code : '',
      network: typeof r.network === 'string' ? r.network : ''
    })
  }
  return out
}

export async function getMccMncIndex(): Promise<Map<string, MccMncRow>> {
  if (indexPromise) return indexPromise
  indexPromise = (async () => {
    const cache = readCache()
    const now = Date.now()
    if (cache && cache.rows.length > 0 && now - cache.fetched_at < CACHE_TTL_MS) {
      return buildIndex(cache.rows)
    }
    try {
      const rows = await fetchRows()
      if (rows.length > 0) writeCache(rows)
      return buildIndex(rows)
    } catch {
      if (cache && cache.rows.length > 0) {
        return buildIndex(cache.rows)
      }
      return new Map()
    }
  })()
  return indexPromise
}

export function isNumericCode(s: string): boolean {
  return /^[0-9]+$/.test(normalizeCode(s))
}

// resolveOperatorName 处理后端只解析出中国大陆/香港/台湾运营商名称、其余地区原样
// 回落成数字 PLMN 的情况：此时改用全球 MCC/MNC 表按 code 查名，而不是把数字当作
// 运营商名称直接显示。code 是调用方已知的 serving MCC+MNC（可为空）；如果调用方没有
// 单独的 code 而 operator 本身就是 5~6 位数字 PLMN（列表/仪表盘等精简响应常见），
// 也会直接把 operator 当作 code 去查表。
export function resolveOperatorName(operator: string | undefined, code: string, index: Map<string, MccMncRow> | null): string {
  const op = normalizeCode(operator || '')
  if (op && !isNumericCode(op)) return op
  if (index) {
    for (const candidate of [code, op]) {
      const c = normalizeCode(candidate)
      if (!c || (c.length !== 5 && c.length !== 6) || !isNumericCode(c)) continue
      const row = index.get(c)
      const name = row ? (normalizeCode(row.network) || normalizeCode(row.country)) : ''
      if (name) return name
    }
  }
  return op
}

export function isoToFlagEmoji(iso: string): string {
  const s = normalizeCode(iso).toUpperCase()
  if (s.length !== 2) return ''
  const a = s.charCodeAt(0)
  const b = s.charCodeAt(1)
  if (a < 65 || a > 90 || b < 65 || b > 90) return ''
  return String.fromCodePoint(0x1f1e6 + (a - 65)) + String.fromCodePoint(0x1f1e6 + (b - 65))
}
