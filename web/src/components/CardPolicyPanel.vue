<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { Sim24Regular } from '@vicons/fluent'
import { Loading } from '@element-plus/icons-vue'
import { ElMessage } from 'element-plus'
import type { CardPolicy } from '../types/api'
import { devicesService } from '../services/devices'
import { cardsService } from '../services/cards'
import { errorMessage } from '../services/http'

const props = defineProps<{
  deviceId: string | undefined
  iccid: string | undefined
  policy: CardPolicy | null
  deviceOnline: boolean
}>()

const emit = defineEmits<{
  policyChanged: []
}>()

// 本地镜像（跟上游 policy 同步）。airplane 直接镜像存储的“用户飞行意图”，
// 与 vowifi 解耦：开 VoWiFi 不再把飞行开关显示成关（VoWiFi 接管时开关仍点亮但禁用），
// 关掉 VoWiFi 后按该意图回退（之前飞行回飞行，否则回在线）。
const local = ref<{
  network_enabled: boolean
  vowifi_enabled: boolean
  airplane_enabled: boolean
  ip_version: 'v4' | 'v6' | 'v4v6'
  apn: string
}>({ network_enabled: false, vowifi_enabled: false, airplane_enabled: false, ip_version: 'v4', apn: '' })

// 各开关的热切换中间态（pending/failed）
const networkPending = ref(false)
const networkFailed = ref(false)
const vowifiPending = ref(false)
const vowifiFailed = ref(false)
const airplanePending = ref(false)
const airplaneFailed = ref(false)

// ===== 流量限制编辑状态 =====
// 编辑用「数值 + 单位」组合，保存时换算成字节；展示用从 policy.quota_usage 读。
type SizeUnit = 'MB' | 'GB' | 'TB'
const UNIT_BYTES: Record<SizeUnit, number> = { MB: 1024 * 1024, GB: 1024 * 1024 * 1024, TB: 1024 * 1024 * 1024 * 1024 }
const COMMON_TZ = ['Asia/Shanghai', 'Asia/Tokyo', 'Asia/Hong_Kong', 'Asia/Singapore', 'Asia/Bangkok', 'Europe/London', 'America/New_York', 'UTC', '']

const quota = ref({
  enabled: false,
  sizeValue: 1,          // 套餐流量数值
  sizeUnit: 'GB' as SizeUnit,
  billingDay: 1,         // 计费日 1-31
  billingTimezone: '',   // 空串=跟随系统时区
  autoStopEnabled: false,
  autoStopValue: 1,       // 使用量阈值数值（与套餐流量独立）
  autoStopUnit: 'GB' as SizeUnit,
})
const quotaSaving = ref(false)

// 从字节反解出 {数值, 单位} 供编辑回填（取最大适配单位）
function splitBytes(bytes: number): { value: number; unit: SizeUnit } {
  const v = Number(bytes) || 0
  if (v >= UNIT_BYTES.TB) return { value: +(v / UNIT_BYTES.TB).toFixed(2), unit: 'TB' }
  if (v >= UNIT_BYTES.GB) return { value: +(v / UNIT_BYTES.GB).toFixed(2), unit: 'GB' }
  if (v >= UNIT_BYTES.MB) return { value: +(v / UNIT_BYTES.MB).toFixed(2), unit: 'MB' }
  return { value: 0, unit: 'MB' }
}

function formatBytesLocal(bytes: number): string {
  const v = Number(bytes) || 0
  const units = ['B', 'KB', 'MB', 'GB', 'TB']
  let val = v
  let i = 0
  while (val >= 1024 && i < units.length - 1) {
    val /= 1024
    i++
  }
  return `${val.toFixed(i === 0 ? 0 : 2)} ${units[i]}`
}

function formatPeriod(ts?: string): string {
  if (!ts) return '--'
  const d = new Date(ts)
  return Number.isFinite(d.getTime()) ? d.toLocaleDateString() : '--'
}

// 上游 policy 变化时原地同步各字段（不整体替换对象，避免 el-switch 崩溃）
watch(
  () => props.policy,
  (p) => {
    if (!p) return
    local.value.network_enabled = p.network_enabled
    local.value.vowifi_enabled = p.vowifi_enabled
    // 直接镜像存储的飞行意图（VoWiFi 开启时也如实点亮，开关由 vowifi 禁用）
    local.value.airplane_enabled = p.airplane_enabled
    local.value.ip_version = p.ip_version || 'v4'
    local.value.apn = p.apn || ''
    networkFailed.value = false
    vowifiFailed.value = false
    airplaneFailed.value = false
    // 同步流量限制编辑状态
    quota.value.enabled = !!p.quota_enabled
    const qs = splitBytes(p.quota_bytes || 0)
    quota.value.sizeValue = qs.value > 0 ? qs.value : 1
    quota.value.sizeUnit = qs.unit
    quota.value.billingDay = p.billing_day || 1
    quota.value.billingTimezone = p.billing_timezone || ''
    quota.value.autoStopEnabled = !!p.auto_stop_enabled
    const as = splitBytes(p.auto_stop_threshold_bytes || 0)
    quota.value.autoStopValue = as.value > 0 ? as.value : qs.value > 0 ? qs.value : 1
    quota.value.autoStopUnit = as.value > 0 ? as.unit : qs.unit
  },
  { immediate: true }
)

const sourceLabel = computed(() => {
  if (!props.policy) return ''
  return props.policy.source === 'user' ? '手动设置' : '自动默认'
})

const canToggle = computed(() => props.deviceOnline && !!props.iccid)

// 流量限制用量展示
const quotaUsage = computed(() => props.policy?.quota_usage)
const quotaBytes = computed(() => props.policy?.quota_bytes || 0)
const thresholdBytes = computed(() => quotaUsage.value?.threshold_bytes || quotaBytes.value || 0)
const usedBytes = computed(() => quotaUsage.value?.used_bytes || 0)
const usedPercent = computed(() => {
  const total = thresholdBytes.value
  if (total <= 0) return 0
  return Math.min(100, Math.round((usedBytes.value / total) * 100))
})
const quotaExceeded = computed(() => !!quotaUsage.value?.exceeded)

async function saveQuota() {
  if (!props.iccid) return
  quotaSaving.value = true
  const result = await cardsService.putPolicy(props.iccid, {
    quota_enabled: quota.value.enabled,
    quota_bytes: quota.value.enabled ? Math.round(quota.value.sizeValue * UNIT_BYTES[quota.value.sizeUnit]) : 0,
    billing_day: quota.value.billingDay,
    billing_timezone: quota.value.billingTimezone,
    auto_stop_enabled: quota.value.autoStopEnabled,
    auto_stop_threshold_bytes: quota.value.autoStopEnabled
      ? Math.round(quota.value.autoStopValue * UNIT_BYTES[quota.value.autoStopUnit])
      : 0,
  })
  quotaSaving.value = false
  if (!result.ok) {
    ElMessage.error(errorMessage(result.error, '保存流量限制失败'))
    return
  }
  ElMessage.success('流量限制已保存')
  emit('policyChanged')
}

async function onNetworkToggle(rawVal: string | number | boolean) {
  const val = rawVal as boolean
  if (!props.deviceId || !canToggle.value) return
  // 开网前：若流量已超限，前端先拦截并回滚
  if (val && quotaExceeded.value) {
    ElMessage.warning('本计费周期流量已达上限，无法开启网络')
    local.value.network_enabled = false
    networkFailed.value = true
    return
  }
  networkPending.value = true
  networkFailed.value = false
  const prev = !val
  let result
  if (val) {
    result = await devicesService.startNetwork(props.deviceId, {
      ip_version: local.value.ip_version,
      apn: local.value.apn
    })
  } else {
    result = await devicesService.stopNetwork(props.deviceId)
  }
  networkPending.value = false
  if (!result.ok) {
    local.value.network_enabled = prev
    networkFailed.value = true
    // 流量超限的后端 403 提示
    if (result.error?.status === 403) {
      ElMessage.warning(result.error.message || '本计费周期流量已达上限')
    }
  } else {
    networkFailed.value = false
    // 开网络与 vowifi/飞行互斥（后端已互斥落库，这里同步 UI）
    if (val) {
      local.value.vowifi_enabled = false
      local.value.airplane_enabled = false
    }
    emit('policyChanged')
  }
}

async function onVoWiFiToggle(rawVal: string | number | boolean) {
  const val = rawVal as boolean
  if (!props.deviceId || !canToggle.value) return
  vowifiPending.value = true
  vowifiFailed.value = false
  const prev = !val
  let result
  if (val) {
    result = await devicesService.enableVoWiFi(props.deviceId)
  } else {
    result = await devicesService.disableVoWiFi(props.deviceId)
  }
  vowifiPending.value = false
  if (!result.ok) {
    local.value.vowifi_enabled = prev
    vowifiFailed.value = true
  } else {
    vowifiFailed.value = false
    // 开 VoWiFi：仅互斥关网络；不动飞行意图（保留用户飞行态，关 VoWiFi 后据此回退）
    if (val) {
      local.value.network_enabled = false
    }
    emit('policyChanged')
  }
}

async function onAirplaneToggle(rawVal: string | number | boolean) {
  const val = rawVal as boolean
  if (!props.deviceId || !canToggle.value) return
  airplanePending.value = true
  airplaneFailed.value = false
  const prev = !val
  const result = await devicesService.setFlightMode(props.deviceId, val)
  airplanePending.value = false
  if (!result.ok) {
    local.value.airplane_enabled = prev
    airplaneFailed.value = true
  } else {
    airplaneFailed.value = false
    // 开飞行与网络/vowifi 互斥（后端已互斥落库，这里同步 UI）
    if (val) {
      local.value.network_enabled = false
      local.value.vowifi_enabled = false
    }
    emit('policyChanged')
  }
}
</script>

<template>
  <div>
    <!-- 标题行 -->
    <div class="flex items-center gap-3 mb-4">
      <div class="w-10 h-10 rounded-xl bg-violet-50 dark:bg-violet-500/10 flex items-center justify-center text-violet-600 dark:text-violet-400">
        <el-icon size="22"><Sim24Regular /></el-icon>
      </div>
      <div>
        <div class="text-lg font-bold text-gray-900 dark:text-white">卡策略</div>
        <div class="text-xs text-gray-500 dark:text-gray-400">网络/VoWiFi 开关跟着 SIM 卡走，切换即时生效</div>
      </div>
    </div>

    <!-- 无 ICCID 提示 -->
    <div v-show="!iccid" class="ui-panel-muted p-4 text-center text-sm text-gray-500 dark:text-gray-400">
      设备尚未识别到 SIM 卡 ICCID，策略不可操作
    </div>

    <!-- 离线提示（有 ICCID 但设备离线） -->
    <div v-show="iccid && !deviceOnline" class="mb-3 px-3 py-2 rounded-lg bg-yellow-50 dark:bg-yellow-900/20 text-xs text-yellow-700 dark:text-yellow-300">
      设备离线，策略仅展示，切换操作已禁用
    </div>

    <!-- 用 v-show 让 el-switch 始终挂载，避免 element-plus 2.13 在挂载前访问未就绪 input 而崩溃 -->
    <div v-show="iccid" class="space-y-3">
      <!-- ICCID + 来源 -->
      <div class="ui-panel-muted p-3 flex items-center justify-between">
        <div>
          <div class="text-xs font-bold text-gray-500 uppercase tracking-wider mb-0.5">当前卡 ICCID</div>
          <div class="text-sm font-mono text-gray-800 dark:text-gray-100">{{ iccid }}</div>
        </div>
        <el-tag v-if="sourceLabel" :type="policy?.source === 'user' ? 'primary' : 'info'" size="small">{{ sourceLabel }}</el-tag>
      </div>

      <div class="grid grid-cols-1 lg:grid-cols-2 gap-3">
                <!-- IP 版本 -->
        <div class="space-y-1">
          <label class="text-xs font-bold text-gray-500 uppercase tracking-wider">IP 版本</label>
          <el-select v-model="local.ip_version" class="w-full" :disabled="!canToggle">
            <el-option label="IPv4" value="v4" />
            <el-option label="IPv6" value="v6" />
            <el-option label="IPv4 + IPv6（双栈）" value="v4v6" />
          </el-select>
          <div class="text-xs text-gray-400">下次开启网络时生效</div>
        </div>

        <!-- APN -->
        <div class="space-y-1">
          <label class="text-xs font-bold text-gray-500 uppercase tracking-wider">APN（可选）</label>
          <el-input v-model="local.apn" placeholder="留空自动识别" :disabled="!canToggle" />
          <div class="text-xs text-gray-400">下次开启网络时生效</div>
        </div>
        <!-- 开启网络 -->
        <div
          class="ui-panel-muted p-3 space-y-1"
          :class="local.network_enabled ? 'border border-emerald-300 bg-emerald-50/50 dark:bg-emerald-900/20' : ''"
        >
          <div class="flex items-center justify-between">
            <div>
              <div class="text-sm font-bold text-gray-800 dark:text-gray-100">开启网络</div>
              <div class="text-xs text-gray-500 dark:text-gray-400">VoWiFi/飞行开启时不可用</div>
            </div>
            <div class="flex items-center gap-2">
              <span v-if="networkFailed" class="text-xs text-orange-500 dark:text-orange-400">未生效</span>
              <el-icon v-if="networkPending" class="animate-spin text-gray-400"><Loading /></el-icon>
              <el-switch
                v-model="local.network_enabled"
                :disabled="!canToggle || local.vowifi_enabled || local.airplane_enabled || networkPending"
                @change="onNetworkToggle"
              />
            </div>
          </div>
        </div>

        <!-- VoWiFi -->
        <div
          class="ui-panel-muted p-3 space-y-1"
          :class="local.vowifi_enabled ? 'border border-orange-300 bg-orange-50/50 dark:bg-orange-900/20' : ''"
        >
          <div class="flex items-center justify-between">
            <div>
              <div class="text-sm font-bold text-gray-800 dark:text-gray-100">VoWiFi</div>
              <div class="text-xs text-gray-500 dark:text-gray-400">启用后进飞行模式，不支持国内运营商</div>
            </div>
            <div class="flex items-center gap-2">
              <span v-if="vowifiFailed" class="text-xs text-orange-500 dark:text-orange-400">未生效</span>
              <el-icon v-if="vowifiPending" class="animate-spin text-gray-400"><Loading /></el-icon>
              <el-switch
                v-model="local.vowifi_enabled"
                :disabled="!canToggle || vowifiPending"
                @change="onVoWiFiToggle"
              />
            </div>
          </div>
        </div>

        <!-- 飞行模式 -->
        <div
          class="ui-panel-muted p-3 space-y-1"
          :class="local.airplane_enabled ? 'border border-sky-300 bg-sky-50/50 dark:bg-sky-900/20' : ''"
        >
          <div class="flex items-center justify-between">
            <div class="flex items-center gap-2">
              <div>
                <div class="text-sm font-bold text-gray-800 dark:text-gray-100">飞行模式</div>
                <div class="text-xs text-gray-500 dark:text-gray-400">射频关闭，断网；VoWiFi 开启时由其接管</div>
              </div>
            </div>
            <div class="flex items-center gap-2">
              <span v-if="airplaneFailed" class="text-xs text-orange-500 dark:text-orange-400">未生效</span>
              <el-icon v-if="airplanePending" class="animate-spin text-gray-400"><Loading /></el-icon>
              <el-switch
                v-model="local.airplane_enabled"
                :disabled="!canToggle || local.vowifi_enabled || airplanePending"
                @change="onAirplaneToggle"
              />
            </div>
          </div>
        </div>


      </div>
      <!-- 流量限制 -->
      <div class="ui-panel-muted p-4 space-y-3">
        <div class="flex items-center justify-between">
          <div>
            <div class="text-sm font-bold text-gray-800 dark:text-gray-100">流量限制</div>
            <div class="text-xs text-gray-500 dark:text-gray-400">按卡(ICCID)统计，跨计费周期自动重置</div>
          </div>
          <el-switch v-model="quota.enabled" :disabled="!iccid" />
        </div>

        <div v-show="quota.enabled" class="space-y-3">
          <!-- 套餐流量 + 阈值（并排各占一半） -->
          <div class="grid grid-cols-2 gap-2">
            <div class="space-y-1">
              <label class="text-xs font-bold text-gray-500 uppercase tracking-wider">套餐流量</label>
              <div class="flex gap-2">
                <el-input-number v-model="quota.sizeValue" :min="0" :step="1" :controls="false" class="w-full" />
                <el-select v-model="quota.sizeUnit" class="w-20 shrink-0">
                  <el-option label="MB" value="MB" />
                  <el-option label="GB" value="GB" />
                  <el-option label="TB" value="TB" />
                </el-select>
              </div>
            </div>
            <div v-show="quota.autoStopEnabled" class="space-y-1">
              <label class="text-xs font-bold text-gray-500 uppercase tracking-wider">达到使用量阈值</label>
              <div class="flex gap-2">
                <el-input-number v-model="quota.autoStopValue" :min="0" :step="1" :controls="false" class="w-full" />
                <el-select v-model="quota.autoStopUnit" class="w-20 shrink-0">
                  <el-option label="MB" value="MB" />
                  <el-option label="GB" value="GB" />
                  <el-option label="TB" value="TB" />
                </el-select>
              </div>
            </div>
          </div>

          <!-- 计费日 + 时区 -->
          <div class="grid grid-cols-2 gap-2">
            <div class="space-y-1">
              <label class="text-xs font-bold text-gray-500 uppercase tracking-wider">计费日</label>
              <el-input-number v-model="quota.billingDay" :min="1" :max="31" :controls="false" class="w-full" />
              <div class="text-xs text-gray-400">每月 1-31 日，无该日取月底</div>
            </div>
            <div class="space-y-1">
              <label class="text-xs font-bold text-gray-500 uppercase tracking-wider">计费时区</label>
              <el-select v-model="quota.billingTimezone" filterable allow-create clearable placeholder="跟随系统" class="w-full">
                <el-option label="跟随系统" value="" />
                <el-option v-for="tz in COMMON_TZ" :key="tz" :label="tz" :value="tz" />
              </el-select>
              <div class="text-xs text-gray-400">可输入 IANA 名，如 Asia/Shanghai</div>
            </div>
          </div>

          <!-- 达到使用量后关闭网络 -->
          <div class="space-y-1 rounded-lg border border-gray-200 dark:border-gray-700 p-3">
            <div class="flex items-center justify-between">
              <div>
                <div class="text-sm font-bold text-gray-800 dark:text-gray-100">达到使用量后关闭网络</div>
                <div class="text-xs text-gray-500 dark:text-gray-400">阈值独立于套餐流量，可留出漏记余量</div>
              </div>
              <el-switch v-model="quota.autoStopEnabled" />
            </div>
          </div>

          <!-- 当前用量 -->
          <div v-if="quotaUsage" class="space-y-1">
            <div class="flex items-center justify-between text-xs">
              <span class="text-gray-500 dark:text-gray-400">本计费周期已用</span>
              <span class="font-mono" :class="quotaExceeded ? 'text-red-500 dark:text-red-400' : 'text-gray-700 dark:text-gray-200'">
                {{ formatBytesLocal(usedBytes) }} / {{ formatBytesLocal(thresholdBytes) }}
              </span>
            </div>
            <el-progress :percentage="usedPercent" :status="quotaExceeded ? 'exception' : 'success'" :stroke-width="10" />
            <div class="text-xs text-gray-400">
              计费周期：{{ formatPeriod(quotaUsage.period_start) }} ~ {{ formatPeriod(quotaUsage.period_end) }}
            </div>
          </div>

          <!-- 保存按钮 -->
          <div class="flex justify-end">
            <el-button type="primary" size="small" :loading="quotaSaving" :disabled="!iccid" @click="saveQuota">
              保存流量限制
            </el-button>
          </div>
        </div>
      </div>

    </div>
  </div>
</template>
