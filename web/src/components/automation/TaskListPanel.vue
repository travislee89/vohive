<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import { automationService } from '../../services/automation'
import { devicesService } from '../../services/devices'
import type { AutomationTask } from '../../types/automation'
import TaskFormDialog from './TaskFormDialog.vue'
import EmptyState from '../EmptyState.vue'
import { Add20Regular, Delete20Regular, Edit20Regular, Play20Regular } from '@vicons/fluent'

const tasks = ref<AutomationTask[]>([])
const loading = ref(false)
const deviceNames = ref<Record<string, string>>({})

const formVisible = ref(false)
const editingTask = ref<AutomationTask | null>(null)
const runningIds = ref<Set<string>>(new Set())

const WEEKDAY_LABELS = ['一', '二', '三', '四', '五', '六', '日']

const ACTION_LABEL: Record<string, string> = {
  reboot: '重启基带',
  sms: '发送短信'
}

function deviceLabel(id: string) {
  if (id === 'all') return '全部设备'
  return deviceNames.value[id] || id
}

function summarizeTrigger(task: AutomationTask) {
  if (task.trigger_type === 'fixed_schedule') {
    const times = (task.fixed_times && task.fixed_times.length > 0 ? task.fixed_times : []).join(', ')
    const days = task.weekdays && task.weekdays.length > 0
      ? `周${task.weekdays.map((d) => WEEKDAY_LABELS[d - 1]).join('、')}`
      : '每天'
    return `${days} ${times}`
  }
  const unitLabel = task.interval_unit === 'hours' ? '小时' : '天'
  return `每隔 ${task.interval_value} ${unitLabel}`
}

function formatNextRun(task: AutomationTask) {
  if (!task.next_run_at) return '无运行历史，即将触发'
  return new Date(task.next_run_at).toLocaleString()
}

function formatLastRun(task: AutomationTask) {
  if (!task.last_run_at) return '未曾运行'
  return `上次运行 ${new Date(task.last_run_at).toLocaleString()}`
}

async function loadTasks() {
  loading.value = true
  try {
    const result = await automationService.listTasks()
    if (!result.ok) throw new Error(result.error.message || '加载自动化任务失败')
    tasks.value = result.data.tasks || []
  } catch (e: unknown) {
    ElMessage.error(e instanceof Error ? e.message : '加载自动化任务失败')
  } finally {
    loading.value = false
  }
}

async function loadDeviceNames() {
  const result = await devicesService.listManaged()
  if (result.ok) {
    const map: Record<string, string> = {}
    for (const d of result.data.devices) {
      map[d.id] = d.name || d.id
    }
    deviceNames.value = map
  }
}

function openCreateDialog() {
  editingTask.value = null
  formVisible.value = true
}

function openEditDialog(task: AutomationTask) {
  editingTask.value = task
  formVisible.value = true
}

async function toggleEnabled(task: AutomationTask) {
  try {
    const result = await automationService.toggleTask(task.id, task.enabled)
    if (!result.ok) throw new Error(result.error.message || '更新失败')
    await loadTasks()
  } catch (e: unknown) {
    task.enabled = !task.enabled
    ElMessage.error(e instanceof Error ? e.message : '更新失败')
  }
}

async function runNow(task: AutomationTask) {
  runningIds.value.add(task.id)
  try {
    const result = await automationService.runTask(task.id)
    if (!result.ok) throw new Error(result.error.message || '执行失败')
    ElMessage.success('任务已开始执行，可在运行日志中查看结果')
  } catch (e: unknown) {
    ElMessage.error(e instanceof Error ? e.message : '执行失败')
  } finally {
    runningIds.value.delete(task.id)
  }
}

async function deleteTask(task: AutomationTask) {
  try {
    await ElMessageBox.confirm(`确定删除任务「${task.name}」吗？`, '删除任务', {
      type: 'warning',
      confirmButtonText: '删除',
      cancelButtonText: '取消'
    })
    const result = await automationService.deleteTask(task.id)
    if (!result.ok) throw new Error(result.error.message || '删除失败')
    ElMessage.success('任务已删除')
    await loadTasks()
  } catch (e: unknown) {
    if (e === 'cancel') return
    ElMessage.error(e instanceof Error ? e.message : '删除失败')
  }
}

onMounted(() => {
  loadTasks()
  loadDeviceNames()
})
</script>

<template>
  <div class="space-y-4">
    <div class="flex items-center justify-between">
      <div class="text-sm text-gray-500">共 <span class="font-bold text-gray-700 dark:text-gray-200">{{ tasks.length }}</span> 个任务</div>
      <el-button type="primary" @click="openCreateDialog" class="!border-0">
        <el-icon><Add20Regular /></el-icon>
        <span class="ml-1">新建任务</span>
      </el-button>
    </div>

    <div v-loading="loading" class="grid grid-cols-1 sm:grid-cols-2 gap-4">
      <EmptyState v-if="!loading && tasks.length === 0" class="sm:col-span-2" title="暂无自动化任务" subtitle="点击右上角新建任务开始添加" />

      <div v-for="task in tasks" :key="task.id" class="ui-card p-5">
        <div class="flex items-center justify-between gap-3 mb-3">
          <div class="flex items-center gap-2 min-w-0">
            <span class="font-bold text-gray-800 dark:text-gray-100 truncate">{{ task.name }}</span>
          </div>
          <el-switch v-model="task.enabled" @change="toggleEnabled(task)" />
        </div>

        <el-tag size="small" type="warning" class="mb-3">{{ ACTION_LABEL[task.action_type] || task.action_type }}</el-tag>

        <div class="space-y-1.5 text-sm">
          <div class="flex justify-between text-gray-500">
            <span>设备</span>
            <span class="text-gray-700 dark:text-gray-200">{{ deviceLabel(task.device_id) }}</span>
          </div>
          <div class="flex justify-between text-gray-500">
            <span>触发机制</span>
            <span class="text-gray-700 dark:text-gray-200">{{ summarizeTrigger(task) }}</span>
          </div>
          <template v-if="task.action_type === 'sms'">
            <div class="flex justify-between text-gray-500">
              <span>接收号码</span>
              <span class="text-gray-700 dark:text-gray-200">{{ task.sms_phone }}</span>
            </div>
            <div class="flex justify-between text-gray-500 gap-4">
              <span class="shrink-0">短信内容</span>
              <span class="text-gray-700 dark:text-gray-200 truncate">{{ task.sms_content }}</span>
            </div>
          </template>
          <div class="flex justify-between text-gray-500">
            <span>下次运行</span>
            <span class="text-indigo-500 dark:text-indigo-400">{{ formatNextRun(task) }}</span>
          </div>
        </div>

        <div class="flex items-center justify-between mt-4 pt-3 border-t border-gray-100 dark:border-white/5">
          <span class="text-xs text-gray-400">{{ formatLastRun(task) }}</span>
          <div class="flex items-center gap-1">
            <el-button text :loading="runningIds.has(task.id)" @click="runNow(task)">
              <el-icon><Play20Regular /></el-icon>
            </el-button>
            <el-button text @click="openEditDialog(task)">
              <el-icon><Edit20Regular /></el-icon>
            </el-button>
            <el-button text type="danger" @click="deleteTask(task)">
              <el-icon><Delete20Regular /></el-icon>
            </el-button>
          </div>
        </div>
      </div>
    </div>

    <TaskFormDialog v-model="formVisible" :task="editingTask" @saved="loadTasks" />
  </div>
</template>
