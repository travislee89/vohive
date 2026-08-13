<script setup lang="ts">
import { computed, onMounted, onUnmounted, ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { ElMessage } from 'element-plus'
import { CallInbound24Regular, CallOutbound24Regular } from '@vicons/fluent'
import PageHeader from '../components/PageHeader.vue'
import EmptyState from '../components/EmptyState.vue'
import ErrorState from '../components/ErrorState.vue'
import ListSkeleton from '../components/ListSkeleton.vue'
import RefreshButton from '../components/RefreshButton.vue'
import { useEventStream } from '../composables/useEventStream'
import { callsService } from '../services/calls'
import { toAppError } from '../services/http'
import { api } from '../stores/auth'
import type { CSCallEvent, CSCallInfo, CSCallListResponse, DeviceMgmtListItem } from '../types/api'
import { formatISOTime } from '../utils/datetime'

const route = useRoute()
const router = useRouter()

const devices = ref<DeviceMgmtListItem[]>([])
const devicesError = ref<{ message: string; status?: number; method?: string; url?: string; hint?: string } | null>(null)
const selectedDevice = ref<string>(typeof route.query.device === 'string' ? route.query.device : '')

const loading = ref(false)
const calls = ref<CSCallInfo[]>([])
const callsError = ref<{ message: string; status?: number; method?: string; url?: string; hint?: string } | null>(null)
const lastOkAt = ref<number | null>(null)

// 实时来电事件流
let eventStream: ReturnType<typeof useEventStream<CSCallEvent>> | null = null
const liveConnected = ref(false)
const lastEvent = ref<CSCallEvent | null>(null)

const stateText: Record<string, string> = {
  ringing: '振铃中',
  dialing: '拨号中',
  connected: '通话中',
  idle: '空闲'
}

const directionText: Record<string, string> = {
  in: '来电',
  out: '去电'
}

const deviceOptions = computed(() =>
  devices.value.map(d => ({ label: d.name || d.id, value: d.id }))
)

const hasCalls = computed(() => calls.value.length > 0)

function stateTagType(state: string) {
  switch (state) {
    case 'ringing': return 'warning'
    case 'dialing': return 'info'
    case 'connected': return 'success'
    default: return 'info'
  }
}

function directionIcon(direction: string) {
  return direction === 'in' ? CallInbound24Regular : CallOutbound24Regular
}

function formatTime(ts: number) {
  if (!ts) return ''
  return formatISOTime(ts * 1000)
}

async function fetchDevices() {
  devicesError.value = null
  try {
    const res = await api.get('/devices')
    const list = (res.data?.devices || []) as DeviceMgmtListItem[]
    devices.value = list.filter(d => d.running)
    if (selectedDevice.value && !devices.value.some(d => d.id === selectedDevice.value)) {
      selectedDevice.value = ''
    }
    if (!selectedDevice.value && devices.value.length === 1) {
      selectedDevice.value = devices.value[0].id
    }
  } catch (err) {
    devicesError.value = toAppError(err)
  }
}

async function fetchCalls(silent = false) {
  if (!selectedDevice.value) {
    calls.value = []
    return
  }
  if (!silent) loading.value = true
  callsError.value = null
  const result = await callsService.list(selectedDevice.value)
  if (result.ok) {
    calls.value = result.data.calls
    lastOkAt.value = Date.now()
  } else {
    callsError.value = result.error
  }
  if (!silent) loading.value = false
}

function connectEventStream() {
  eventStream?.disconnect()
  if (!selectedDevice.value) return

  eventStream = useEventStream<CSCallEvent>({
    path: `/devices/${selectedDevice.value}/calls/events`,
    eventName: 'cscall_event',
    reconnectDelayMs: 3000,
    parse: (payload: string) => JSON.parse(payload) as CSCallEvent,
    onEvent: (data: CSCallEvent) => {
      lastEvent.value = data
      if (data.type === 'incoming') {
        ElMessage({
          type: 'warning',
          message: `📞 新来电：${data.number || '未知号码'}`,
          duration: 5000
        })
      }
      void fetchCalls(true)
    },
    onRawEvent: (eventName: string, payload: string) => {
      if (eventName === 'cscall_snapshot') {
        try {
          const snap = JSON.parse(payload) as CSCallListResponse
          calls.value = snap.calls
          lastOkAt.value = Date.now()
        } catch {
          // ignore malformed snapshot
        }
      }
    },
    onConnected: () => {
      liveConnected.value = true
    }
  })
  eventStream.connect()
}

function disconnectEventStream() {
  eventStream?.disconnect()
  eventStream = null
  liveConnected.value = false
}

function handleDeviceChange() {
  void router.replace({ query: selectedDevice.value ? { device: selectedDevice.value } : {} })
  calls.value = []
  void fetchCalls()
  connectEventStream()
}

onMounted(async () => {
  await fetchDevices()
  if (selectedDevice.value) {
    await fetchCalls()
    connectEventStream()
  }
})

onUnmounted(() => {
  disconnectEventStream()
})
</script>

<template>
  <div class="calls-page">
    <PageHeader title="来电查询" subtitle="查看设备当前活跃的 CS 呼叫，实时接收来电通知">
      <template #actions>
        <div class="flex items-center gap-3">
          <el-select
            v-model="selectedDevice"
            placeholder="选择设备"
            class="w-56"
            clearable
            @change="handleDeviceChange"
          >
            <el-option
              v-for="opt in deviceOptions"
              :key="opt.value"
              :label="opt.label"
              :value="opt.value"
            />
          </el-select>
          <RefreshButton
            :last-ok-at="lastOkAt"
            :loading="loading"
            @refresh="fetchCalls()"
          />
        </div>
      </template>
    </PageHeader>

    <div v-if="devicesError" class="mb-4">
      <ErrorState
        :message="devicesError.message"
        @retry="fetchDevices"
      />
    </div>

    <div v-if="!selectedDevice" class="mt-8">
      <EmptyState
        title="请选择设备"
        description="选择一台设备以查看其当前活跃的呼叫状态"
      />
    </div>

    <div v-else>
      <!-- 实时连接状态 -->
      <div class="mb-4 flex items-center gap-2 text-sm">
        <span
          class="inline-block h-2.5 w-2.5 rounded-full"
          :class="liveConnected ? 'bg-emerald-500' : 'bg-gray-400'"
        />
        <span class="text-gray-600 dark:text-gray-300">
          {{ liveConnected ? '实时来电推送已连接' : '实时来电推送未连接' }}
        </span>
        <span v-if="lastEvent" class="text-gray-400 dark:text-gray-500">
          最近事件：{{ directionText[lastEvent.type === 'incoming' ? 'in' : 'out'] || lastEvent.type }} {{ lastEvent.number || '' }} {{ formatTime(lastEvent.ts) }}
        </span>
      </div>

      <div v-if="callsError" class="mb-4">
        <ErrorState
          :message="callsError.message"
          :hint="callsError.hint"
          @retry="fetchCalls()"
        />
      </div>

      <div v-if="loading && !hasCalls" class="mt-4">
        <ListSkeleton :rows="3" />
      </div>

      <div v-else-if="!hasCalls" class="mt-4">
        <EmptyState
          title="当前无活跃呼叫"
          description="该设备当前没有进行中的来电或去电"
        />
      </div>

      <div v-else class="grid gap-3">
        <div
          v-for="call in calls"
          :key="call.id"
          class="ui-glass-border rounded-xl p-4 flex items-center gap-4"
        >
          <div
            class="flex h-11 w-11 items-center justify-center rounded-full"
            :class="call.direction === 'in' ? 'bg-amber-500/15 text-amber-500' : 'bg-sky-500/15 text-sky-500'"
          >
            <component :is="directionIcon(call.direction)" class="h-5 w-5" />
          </div>
          <div class="flex-1 min-w-0">
            <div class="flex items-center gap-2">
              <span class="font-medium text-gray-800 dark:text-gray-100">
                {{ call.number || '未知号码' }}
              </span>
              <el-tag :type="stateTagType(call.state)" size="small">
                {{ stateText[call.state] || call.state }}
              </el-tag>
            </div>
            <div class="mt-1 text-sm text-gray-500 dark:text-gray-400">
              {{ directionText[call.direction] || call.direction }} · ID: {{ call.id }}
            </div>
          </div>
        </div>
      </div>
    </div>
  </div>
</template>