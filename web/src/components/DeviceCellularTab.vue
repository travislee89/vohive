<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { Copy20Regular, Dismiss20Regular, Location20Regular } from '@vicons/fluent'
import { ElMessage } from 'element-plus'
import type { DeviceOverviewItem } from '../types/api'
import { copyToClipboard } from '../utils/clipboard'
import { resolveOperatorName, type MccMncRow } from '../utils/mcc-mnc'
import { opencellidService } from '../services/opencellid'
import StatusLight from './StatusLight.vue'
import type { StatusLightTone } from './statusLight'
import FieldRow from './FieldRow.vue'

const props = defineProps<{
  device: DeviceOverviewItem | null
  operatorNameDisplay: string
  simOperatorDisplay: string
  mccMncIndex: Map<string, MccMncRow> | null
}>()

const modem = computed(() => props.device?.modem)

// ---- 通用：空值判定（后端约定 0 代表"未取到读数"）----
function num(v: number | undefined | null): number | null {
  return typeof v === 'number' && Number.isFinite(v) && v !== 0 ? v : null
}

function numText(v: number | undefined | null, suffix = '') {
  const n = num(v)
  return n === null ? '--' : `${n}${suffix}`
}

// ---- 注册状态 ----
const regStatus = computed(() => modem.value?.reg_status ?? 0)
const isRegistered = computed(() => regStatus.value === 1 || regStatus.value === 5)
const isRoaming = computed(() => regStatus.value === 5)

const regTone = computed<StatusLightTone>(() => {
  switch (regStatus.value) {
    case 1:
    case 5:
      return 'success'
    case 2:
      return 'warning'
    case 3:
      return 'danger'
    default:
      return 'neutral'
  }
})

const ratDisplay = computed(() =>
  [modem.value?.network_duplex, modem.value?.network_mode].filter(Boolean).join(' ') || '--'
)

// ---- 信号质量分级 ----
type Level = { text: string; barClass: string; textClass: string; bars: number }

function levelFor(value: number | null, thresholds: number[]): Level {
  const labels = ['极差', '差', '中', '良', '优']
  const barClasses = [
    'bg-red-500',
    'bg-red-500',
    'bg-amber-500',
    'bg-emerald-500',
    'bg-emerald-500',
  ]
  const textClasses = [
    'text-red-500 dark:text-red-400',
    'text-red-500 dark:text-red-400',
    'text-amber-500 dark:text-amber-400',
    'text-emerald-500 dark:text-emerald-400',
    'text-emerald-500 dark:text-emerald-400',
  ]
  if (value === null) {
    return { text: '--', barClass: 'bg-gray-300 dark:bg-gray-600', textClass: 'text-gray-400 dark:text-gray-500', bars: 0 }
  }
  let idx = 0
  for (let i = 0; i < thresholds.length; i++) {
    if (value >= thresholds[i]) idx = i + 1
  }
  return { text: labels[idx], barClass: barClasses[idx], textClass: textClasses[idx], bars: idx + 1 }
}

// 阈值以业界常用 LTE/NR 参考区间为准，从低到高
// RSSI 阈值与概览页「信号强度」卡片保持一致，因为综合信号强度对外统一以 RSSI 为准
const rssiLevel = computed(() => levelFor(num(modem.value?.signal_dbm), [-105, -95, -85, -75]))
const rsrpLevel = computed(() => levelFor(num(modem.value?.signal_rsrp), [-115, -105, -95, -80]))
const rsrqLevel = computed(() => levelFor(num(modem.value?.signal_rsrq), [-20, -15, -11, -8]))
const sinrLevel = computed(() => levelFor(num(modem.value?.signal_sinr), [-3, 0, 10, 20]))
const nr5gSinrLevel = computed(() => levelFor(num(modem.value?.nr5g_signal_sinr), [-3, 0, 10, 20]))

const hasNR5G = computed(() => num(modem.value?.nr5g_signal_sinr) !== null)

const metricRows = computed(() => [
  { label: 'RSSI', value: numText(modem.value?.signal_dbm, ' dBm'), level: rssiLevel.value },
  { label: 'RSRP', value: numText(modem.value?.signal_rsrp, ' dBm'), level: rsrpLevel.value },
  { label: 'RSRQ', value: numText(modem.value?.signal_rsrq, ' dB'), level: rsrqLevel.value },
  { label: 'SINR', value: numText(modem.value?.signal_sinr, ' dB'), level: sinrLevel.value },
])

// ---- 归属 / 漫游对比 ----
const nativePlmn = computed(() => {
  const mcc = modem.value?.native_mcc
  const mnc = modem.value?.native_mnc
  return mcc || mnc ? `${mcc || '--'}-${mnc || '--'}` : '--'
})

const servingPlmn = computed(() => {
  const mcc = modem.value?.serving_mcc
  const mnc = modem.value?.serving_mnc
  return mcc || mnc ? `${mcc || '--'}-${mnc || '--'}` : '--'
})

const nativePlmnDisplay = computed(() => {
  if (nativePlmn.value === '--') return undefined
  const code = `${modem.value?.native_mcc || ''}${modem.value?.native_mnc || ''}`
  const name = resolveOperatorName('', code, props.mccMncIndex)
  return name ? `${nativePlmn.value} · ${name}` : nativePlmn.value
})

const servingPlmnDisplay = computed(() => {
  if (servingPlmn.value === '--') return undefined
  const code = `${modem.value?.serving_mcc || ''}${modem.value?.serving_mnc || ''}`
  const name = resolveOperatorName(modem.value?.operator, code, props.mccMncIndex)
  return name ? `${servingPlmn.value} · ${name}` : servingPlmn.value
})

// ---- 小区定位参数（供第三方基站定位 API 使用）----
const locationParams = computed(() => ({
  mcc: modem.value?.serving_mcc || modem.value?.native_mcc || '',
  mnc: modem.value?.serving_mnc || modem.value?.native_mnc || '',
  lac: modem.value?.lac || '',
  cid: modem.value?.cell_id || '',
  signal_dbm: num(modem.value?.signal_dbm),
}))

const hasLocationParams = computed(() => {
  const p = locationParams.value
  return !!(p.mcc || p.mnc || p.lac || p.cid)
})

async function copyLocationJSON() {
  await copyToClipboard(JSON.stringify(locationParams.value, null, 2), '已复制定位参数 JSON')
}

// ---- OpenCellID 基站定位查询 ----
const hasOpenCellIdKey = ref(false)
const locating = ref(false)
const locateResult = ref<{ lat: number; lon: number; radio: string; range: number } | null>(null)

onMounted(async () => {
  const result = await opencellidService.getSettings()
  hasOpenCellIdKey.value = result.ok && !!result.data.key?.trim()
})

const canLocate = computed(() => hasOpenCellIdKey.value && hasLocationParams.value)

const mapsURL = computed(() =>
  locateResult.value ? `https://www.google.com/maps?q=${locateResult.value.lat},${locateResult.value.lon}` : ''
)

async function locateWithOpenCellID() {
  if (!canLocate.value) return
  locating.value = true
  try {
    const result = await opencellidService.locate({
      mcc: locationParams.value.mcc,
      mnc: locationParams.value.mnc,
      lac: locationParams.value.lac,
      cid: locationParams.value.cid,
      network_mode: modem.value?.network_mode || ''
    })
    if (!result.ok) {
      ElMessage.error(result.error.message || '查询 OpenCellID 定位失败')
      return
    }
    const { lat, lon, range, radio } = result.data
    locateResult.value = { lat, lon, radio, range }
  } catch (e: any) {
    ElMessage.error(e?.message || '查询 OpenCellID 定位失败')
  } finally {
    locating.value = false
  }
}
</script>

<template>
  <div class="space-y-4">
    <div v-if="!device" class="ui-panel-muted p-8 text-center text-sm text-gray-400">
      暂无设备
    </div>
    <template v-else>
      <div class="grid grid-cols-1 xl:grid-cols-2 gap-4 items-start">

        <!-- ===== 服务小区 ===== -->
        <div class="ui-panel-muted p-4">
          <div class="flex items-center justify-between mb-3">
            <div class="text-xs font-bold text-gray-500 uppercase tracking-wider">服务小区</div>
          </div>

          <div class="flex items-center gap-2.5 rounded-xl px-3.5 py-2.5 mb-3 border"
            :class="isRegistered
              ? 'bg-emerald-50 border-emerald-200 dark:bg-emerald-500/10 dark:border-emerald-500/25'
              : 'bg-gray-100 border-gray-200 dark:bg-white/5 dark:border-white/10'"
          >
            <StatusLight :tone="regTone" size="sm" :animated="isRegistered" />
            <div class="flex-1 min-w-0">
              <div class="text-sm font-bold leading-tight"
                :class="isRegistered ? 'text-emerald-700 dark:text-emerald-300' : 'text-gray-500 dark:text-gray-400'">
                {{ operatorNameDisplay || modem?.operator || '--' }}
                <span v-if="ratDisplay !== '--'" class="opacity-70">· {{ ratDisplay }}{{ isRoaming ? ' ·' : '' }}</span>
                <el-tag v-if="isRoaming" type="warning" size="small" effect="light" class="ml-1">漫游</el-tag>
              </div>
              <div class="text-xs text-gray-400 mt-0.5">{{ modem?.reg_status_text || '--' }}</div>
            </div>
          </div>

          <div class="space-y-1.5 text-sm text-gray-700 dark:text-gray-200">
            <FieldRow label="频段" :value="modem?.radio_band" monospace copyable />
            <FieldRow label="信道 (ARFCN)" :value="modem?.radio_channel ? String(modem.radio_channel) : undefined" monospace copyable />
            <FieldRow label="Cell ID" :value="modem?.cell_id" monospace copyable />
            <FieldRow label="LAC / TAC" :value="modem?.lac" monospace copyable />
            <div class="flex w-full items-center justify-between gap-3">
              <span class="text-gray-500">PS 附着</span>
              <span>{{ modem?.ps_attached ? '是' : '否' }}</span>
            </div>
            <FieldRow label="APN" :value="modem?.apn" monospace copyable />
          </div>
        </div>

        <!-- ===== 信号质量 ===== -->
        <div class="ui-panel-muted p-4">
          <div class="text-xs font-bold text-gray-500 uppercase tracking-wider mb-3">信号质量</div>

          <div class="flex items-center gap-3 mb-4">
            <div class="flex items-end gap-1" style="height: 32px">
              <div v-for="i in 5" :key="i"
                class="w-2 rounded-sm"
                :style="{ height: (i * 16 + 20) + '%' }"
                :class="i <= rssiLevel.bars ? (rssiLevel.barClass) : 'bg-gray-200 dark:bg-white/10'"
              />
            </div>
            <div>
              <div class="flex items-baseline gap-1.5">
                <span class="text-2xl font-extrabold tabular-nums leading-none" :class="rssiLevel.textClass">
                  {{ numText(modem?.signal_dbm) }}
                </span>
                <span class="text-xs text-gray-400">dBm RSSI</span>
              </div>
              <div class="text-xs mt-0.5" :class="rssiLevel.textClass">综合信号：{{ rssiLevel.text }}</div>
            </div>
          </div>

          <div class="space-y-2">
            <div v-for="row in metricRows" :key="row.label" class="flex items-center gap-3">
              <span class="w-14 shrink-0 text-xs font-bold text-gray-400">{{ row.label }}</span>
              <div class="flex-1 flex gap-0.5" v-if="row.level">
                <div v-for="i in 4" :key="i" class="h-1.5 flex-1 rounded-full"
                  :class="i <= row.level.bars ? row.level.barClass : 'bg-gray-200 dark:bg-white/10'" />
              </div>
              <div class="flex-1" v-else />
              <span class="w-24 shrink-0 text-right text-sm font-mono tabular-nums text-gray-700 dark:text-gray-200">{{ row.value }}</span>
            </div>
            <div v-if="hasNR5G" class="flex items-center gap-3">
              <span class="w-14 shrink-0 text-xs font-bold text-gray-400">NR SINR</span>
              <div class="flex-1 flex gap-0.5">
                <div v-for="i in 4" :key="i" class="h-1.5 flex-1 rounded-full"
                  :class="i <= nr5gSinrLevel.bars ? nr5gSinrLevel.barClass : 'bg-gray-200 dark:bg-white/10'" />
              </div>
              <span class="w-24 shrink-0 text-right text-sm font-mono tabular-nums text-gray-700 dark:text-gray-200">{{ numText(modem?.nr5g_signal_sinr, ' dB') }}</span>
            </div>
          </div>
        </div>
      </div>

      <div class="grid grid-cols-1 xl:grid-cols-2 gap-4 items-start">

        <!-- ===== 归属 / 漫游 ===== -->
        <div class="ui-panel-muted p-4">
          <div class="text-xs font-bold text-gray-500 uppercase tracking-wider mb-3">归属与漫游</div>
          <div class="space-y-1.5 text-sm text-gray-700 dark:text-gray-200">
            <FieldRow label="归属运营商" :value="simOperatorDisplay" copyable />
            <FieldRow label="归属 PLMN (MCC-MNC)" :value="nativePlmnDisplay" copyable />
            <FieldRow label="当前驻留 PLMN (MCC-MNC)" :value="servingPlmnDisplay" copyable />
            <div class="flex w-full items-center justify-between gap-3">
              <span class="text-gray-500">漫游状态</span>
              <el-tag :type="isRoaming ? 'warning' : 'info'" size="small" effect="light">
                {{ isRegistered ? (isRoaming ? '漫游中' : '归属网络') : '未注册' }}
              </el-tag>
            </div>
            <FieldRow label="GID1" :value="modem?.gid1" monospace copyable />
            <FieldRow label="GID2" :value="modem?.gid2" monospace copyable />
          </div>
        </div>

        <!-- ===== 小区定位参数 ===== -->
        <div class="ui-panel-muted p-4">
          <div class="flex items-center justify-between mb-3">
            <div class="text-xs font-bold text-gray-500 uppercase tracking-wider">小区定位参数</div>
            <div class="flex items-center gap-2">
              <el-button size="small" plain :disabled="!hasLocationParams" @click="copyLocationJSON">
                <el-icon class="mr-1"><Copy20Regular /></el-icon>
                复制 JSON
              </el-button>
              <el-button
                size="small"
                plain
                :disabled="!canLocate"
                :loading="locating"
                :title="!hasOpenCellIdKey ? '请先在系统设置中配置 OpenCellID Key' : ''"
                @click="locateWithOpenCellID"
              >
                <el-icon class="mr-1"><Location20Regular /></el-icon>
                OpenCellID 定位
              </el-button>
            </div>
          </div>
          <div class="text-xs text-gray-400 mb-3">可用于第三方基站定位 API（高德、百度、Google、OpenCellID 等）</div>
          <div v-if="!hasLocationParams" class="flex items-center justify-center py-6 text-sm text-gray-400">
            暂无定位数据
          </div>
          <template v-else>
            <div class="grid grid-cols-2 gap-x-4 gap-y-1.5 text-sm text-gray-700 dark:text-gray-200 mb-3">
              <FieldRow label="MCC" :value="locationParams.mcc" monospace copyable />
              <FieldRow label="MNC" :value="locationParams.mnc" monospace copyable />
              <FieldRow label="LAC/TAC" :value="locationParams.lac" monospace copyable />
              <FieldRow label="CID" :value="locationParams.cid" monospace copyable />
            </div>
            <div v-if="locateResult" class="relative rounded-lg border border-blue-200 dark:border-blue-500/20 bg-blue-50 dark:bg-blue-500/10 p-3 pr-8 text-sm text-gray-700 dark:text-gray-200">
              <button
                type="button"
                class="absolute top-2 right-2 text-gray-400 hover:text-gray-600 dark:hover:text-gray-300"
                aria-label="关闭"
                @click="locateResult = null"
              >
                <el-icon size="16"><Dismiss20Regular /></el-icon>
              </button>
              <div class="grid grid-cols-2 gap-x-4 gap-y-1.5">
                <div><span class="text-gray-500">纬度：</span>{{ locateResult.lat }}</div>
                <div><span class="text-gray-500">经度：</span>{{ locateResult.lon }}</div>
                <div><span class="text-gray-500">网络制式：</span>{{ locateResult.radio || '--' }}</div>
                <div><span class="text-gray-500">估计误差半径：</span>{{ locateResult.range ? locateResult.range + ' 米' : '--' }}</div>
              </div>
              <a :href="mapsURL" target="_blank" rel="noopener noreferrer" class="inline-block mt-2 text-blue-600 dark:text-blue-400 hover:underline">
                在 Google Maps 中查看
              </a>
            </div>
          </template>
        </div>
      </div>
    </template>
  </div>
</template>
