<script setup lang="ts">
import { onMounted, reactive, ref } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import { automationService } from '../../services/automation'
import type { AutomationLogRetention, AutomationRunLog } from '../../types/automation'
import EmptyState from '../EmptyState.vue'
import { Delete20Regular } from '@vicons/fluent'

const logs = ref<AutomationRunLog[]>([])
const total = ref(0)
const loading = ref(false)
const page = ref(1)
const pageSize = ref(20)

const typeFilter = ref('')
const statusFilter = ref('')
const keyword = ref('')
const dateRange = ref<[string, string] | null>(null)

const retention = reactive<AutomationLogRetention>({ auto_cleanup_enabled: true, retention_days: 30 })
const savingRetention = ref(false)
const cleanupDialogVisible = ref(false)
const cleanupBefore = ref('')

const ACTION_LABEL: Record<string, string> = {
  reboot: '重启基带',
  sms: '发送短信'
}

const STATUS_LABEL: Record<string, string> = {
  success: '成功',
  failed: '失败'
}

function statusTagType(status: string) {
  return status === 'success' ? 'success' : 'danger'
}

async function loadLogs() {
  loading.value = true
  try {
    const result = await automationService.listLogs({
      page: page.value,
      page_size: pageSize.value,
      action_type: typeFilter.value || undefined,
      status: statusFilter.value || undefined,
      q: keyword.value || undefined,
      start: dateRange.value?.[0],
      end: dateRange.value?.[1]
    })
    if (!result.ok) throw new Error(result.error.message || '加载运行日志失败')
    logs.value = result.data.logs || []
    total.value = result.data.total || 0
  } catch (e: unknown) {
    ElMessage.error(e instanceof Error ? e.message : '加载运行日志失败')
  } finally {
    loading.value = false
  }
}

function onFilterChange() {
  page.value = 1
  loadLogs()
}

function onPageChange(p: number) {
  page.value = p
  loadLogs()
}

async function loadRetention() {
  const result = await automationService.getRetention()
  if (result.ok) {
    retention.auto_cleanup_enabled = result.data.auto_cleanup_enabled
    retention.retention_days = result.data.retention_days
  }
}

async function saveRetention() {
  savingRetention.value = true
  try {
    const result = await automationService.updateRetention({ ...retention })
    if (!result.ok) throw new Error(result.error.message || '保存自动清理设置失败')
    ElMessage.success('自动清理设置已保存')
  } catch (e: unknown) {
    ElMessage.error(e instanceof Error ? e.message : '保存自动清理设置失败')
  } finally {
    savingRetention.value = false
  }
}

async function runAdvancedCleanup() {
  try {
    await ElMessageBox.confirm(
      cleanupBefore.value
        ? `将删除 ${cleanupBefore.value} 之前的运行日志，确定继续吗？`
        : '将删除全部运行日志，确定继续吗？',
      '高级清理',
      { type: 'warning', confirmButtonText: '确定删除', cancelButtonText: '取消' }
    )
    const result = await automationService.cleanupLogs({
      action_type: typeFilter.value || undefined,
      status: statusFilter.value || undefined,
      before: cleanupBefore.value || undefined
    })
    if (!result.ok) throw new Error(result.error.message || '清理失败')
    ElMessage.success(`已删除 ${result.data.deleted} 条运行日志`)
    cleanupDialogVisible.value = false
    page.value = 1
    loadLogs()
  } catch (e: unknown) {
    if (e === 'cancel') return
    ElMessage.error(e instanceof Error ? e.message : '清理失败')
  }
}

onMounted(() => {
  loadLogs()
  loadRetention()
})
</script>

<template>
  <div class="space-y-4">
    <div class="ui-card p-4">
      <div class="flex flex-wrap items-center gap-3">
        <el-select v-model="typeFilter" placeholder="任务类型" class="w-32" clearable @change="onFilterChange">
          <el-option label="重启基带" value="reboot" />
          <el-option label="发送短信" value="sms" />
        </el-select>
        <el-select v-model="statusFilter" placeholder="执行状态" class="w-32" clearable @change="onFilterChange">
          <el-option label="成功" value="success" />
          <el-option label="失败" value="failed" />
        </el-select>
        <el-date-picker
          v-model="dateRange"
          type="datetimerange"
          value-format="YYYY-MM-DDTHH:mm:ssZ"
          start-placeholder="开始时间"
          end-placeholder="结束时间"
          class="!w-80"
          @change="onFilterChange"
        />
        <el-input v-model="keyword" placeholder="搜索关键字…" clearable class="w-56" @keyup.enter="onFilterChange" @clear="onFilterChange" />
        <el-button type="primary" plain @click="onFilterChange">搜索</el-button>
        <div class="flex-1" />
        <span class="text-xs text-gray-400">共 {{ total }} 条</span>
      </div>
    </div>

    <div class="ui-card overflow-hidden">
      <el-table v-loading="loading" :data="logs" style="width: 100%">
        <el-table-column prop="timestamp" label="时间" width="180">
          <template #default="{ row }">{{ new Date(row.timestamp).toLocaleString() }}</template>
        </el-table-column>
        <el-table-column prop="task_name" label="任务名称" min-width="140" show-overflow-tooltip />
        <el-table-column prop="action_type" label="任务类型" width="100">
          <template #default="{ row }">{{ ACTION_LABEL[row.action_type] || row.action_type }}</template>
        </el-table-column>
        <el-table-column prop="status" label="执行结果" width="100">
          <template #default="{ row }">
            <el-tag size="small" :type="statusTagType(row.status)">{{ STATUS_LABEL[row.status] || row.status }}</el-tag>
          </template>
        </el-table-column>
        <el-table-column label="执行详情" min-width="260" show-overflow-tooltip>
          <template #default="{ row }">{{ row.error_detail || row.result_summary }}</template>
        </el-table-column>
      </el-table>
      <div v-if="!loading && logs.length === 0" class="p-8">
        <EmptyState title="暂无运行日志" subtitle="任务触发后，执行记录会展示在这里" />
      </div>
      <div v-if="total > pageSize" class="flex justify-end p-4">
        <el-pagination
          layout="prev, pager, next"
          :current-page="page"
          :page-size="pageSize"
          :total="total"
          @current-change="onPageChange"
        />
      </div>
    </div>

    <div class="ui-card p-4">
      <div class="flex flex-wrap items-center gap-4">
        <div class="flex items-center gap-2">
          <span class="text-sm font-bold text-gray-700 dark:text-gray-200">自动清理</span>
          <el-switch v-model="retention.auto_cleanup_enabled" @change="saveRetention" />
        </div>
        <div class="flex items-center gap-2">
          <span class="text-xs text-gray-500">保留天数</span>
          <el-input-number
            v-model="retention.retention_days"
            :min="1"
            :max="365"
            size="small"
            controls-position="right"
            :disabled="!retention.auto_cleanup_enabled"
            @change="saveRetention"
          />
        </div>
        <div :class="{ 'opacity-60': savingRetention }" class="text-xs text-gray-400">超过保留天数的日志将每小时自动清理一次</div>
        <div class="flex-1" />
        <el-button type="danger" plain @click="cleanupDialogVisible = true">
          <el-icon><Delete20Regular /></el-icon>
          <span class="ml-1">高级清理</span>
        </el-button>
      </div>
    </div>

    <el-dialog v-model="cleanupDialogVisible" title="高级清理" width="420px">
      <div class="space-y-3">
        <p class="text-sm text-gray-500">按当前筛选的任务类型/执行状态，删除指定时间之前的运行日志（留空表示不限时间，将删除全部匹配记录）。</p>
        <el-date-picker
          v-model="cleanupBefore"
          type="datetime"
          value-format="YYYY-MM-DDTHH:mm:ssZ"
          placeholder="选择截止时间（可选）"
          class="w-full"
        />
      </div>
      <template #footer>
        <el-button @click="cleanupDialogVisible = false">取消</el-button>
        <el-button type="danger" @click="runAdvancedCleanup">确定删除</el-button>
      </template>
    </el-dialog>
  </div>
</template>
