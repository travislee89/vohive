<script setup lang="ts">
import { computed, onMounted, reactive, ref } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import { notifyService } from '../../services/notify'
import type { NotifyChannelKey, NotifyRule, NotifyRulePayload } from '../../types/notify'
import TemplateVariableInput from '../TemplateVariableInput.vue'
import EmptyState from '../EmptyState.vue'
import { Add20Regular, Delete20Regular, ChevronDown20Regular, ChevronUp20Regular } from '@vicons/fluent'

const CHANNEL_OPTIONS: { key: NotifyChannelKey; label: string }[] = [
  { key: 'telegram', label: 'Telegram Bot' },
  { key: 'feishu', label: '飞书 Bot' },
  { key: 'qq', label: 'QQ Bot' },
  { key: 'webhook', label: 'Webhook' },
  { key: 'bark', label: 'Bark' },
  { key: 'email', label: 'Email' },
  { key: 'pushplus', label: 'Pushplus' }
]

const SMS_TEMPLATE_VARIABLES = [
  { key: 'sender', label: '发送方号码' },
  { key: 'local_phone', label: '本机号码' },
  { key: 'operator', label: '运营商' },
  { key: 'content', label: '短信内容' },
  { key: 'timestamp', label: '时间' },
  { key: 'source', label: '短信方向' },
  { key: 'code', label: '验证码（暂未实现，值将为空）', disabled: true }
]

const AUTOMATION_TEMPLATE_VARIABLES = [
  { key: 'task_name', label: '任务名称' },
  { key: 'action_type', label: '动作类型' },
  { key: 'device_id', label: '设备' },
  { key: 'status', label: '执行状态' },
  { key: 'result_summary', label: '执行结果' },
  { key: 'error_detail', label: '错误详情' },
  { key: 'timestamp', label: '时间' }
]

const MESSAGE_TYPES = [
  { key: 'sms', label: '短信', supported: true },
  { key: 'ddns', label: 'DDNS', supported: false },
  { key: 'version_update', label: '版本更新', supported: false },
  { key: 'system_event', label: '系统事件', supported: false },
  { key: 'device_status', label: '设备状态', supported: false },
  { key: 'automation_event', label: '自动化事件', supported: true }
]

const activeType = ref('sms')
const rules = ref<NotifyRule[]>([])
const counts = reactive<Record<string, { enabled: number; total: number }>>({})
const loading = ref(false)
const expandedIds = ref<Set<string>>(new Set())
const savingIds = ref<Set<string>>(new Set())

const activeTypeMeta = computed(() => MESSAGE_TYPES.find((t) => t.key === activeType.value))
const activeTemplateVariables = computed(() => (activeType.value === 'automation_event' ? AUTOMATION_TEMPLATE_VARIABLES : SMS_TEMPLATE_VARIABLES))

async function loadCounts() {
  for (const t of MESSAGE_TYPES) {
    const result = await notifyService.listRules(t.key)
    if (result.ok) {
      counts[t.key] = { enabled: result.data.enabled, total: result.data.total }
    }
  }
}

async function loadRules() {
  if (!activeTypeMeta.value?.supported) {
    rules.value = []
    return
  }
  loading.value = true
  try {
    const result = await notifyService.listRules(activeType.value)
    if (!result.ok) throw new Error(result.error.message || '加载转发规则失败')
    rules.value = result.data.rules || []
    counts[activeType.value] = { enabled: result.data.enabled, total: result.data.total }
    expandedIds.value = new Set(rules.value.map((r) => r.id))
  } catch (e: unknown) {
    ElMessage.error(e instanceof Error ? e.message : '加载转发规则失败')
  } finally {
    loading.value = false
  }
}

function selectType(key: string) {
  activeType.value = key
  loadRules()
}

function toggleExpand(id: string) {
  const next = new Set(expandedIds.value)
  if (next.has(id)) next.delete(id)
  else next.add(id)
  expandedIds.value = next
}

function ruleToPayload(rule: NotifyRule): NotifyRulePayload {
  return {
    message_type: rule.message_type,
    name: rule.name,
    enabled: rule.enabled,
    priority: rule.priority,
    match_field: rule.match_field,
    match_method: rule.match_method,
    match_content: rule.match_content,
    target_channels: rule.target_channels,
    title_template: rule.title_template,
    body_mode: rule.body_mode,
    body_template: rule.body_template
  }
}

async function saveRule(rule: NotifyRule) {
  if (!rule.name.trim()) {
    ElMessage.error('规则名称不能为空')
    return
  }
  savingIds.value.add(rule.id)
  try {
    const result = await notifyService.updateRule(rule.id, ruleToPayload(rule))
    if (!result.ok) throw new Error(result.error.message || '保存失败')
    ElMessage.success('规则已保存')
    await loadRules()
  } catch (e: unknown) {
    ElMessage.error(e instanceof Error ? e.message : '保存失败')
  } finally {
    savingIds.value.delete(rule.id)
  }
}

async function toggleEnabled(rule: NotifyRule) {
  try {
    const result = await notifyService.updateRule(rule.id, { enabled: rule.enabled })
    if (!result.ok) throw new Error(result.error.message || '更新失败')
    counts[activeType.value] = counts[activeType.value] || { enabled: 0, total: 0 }
    await loadRules()
  } catch (e: unknown) {
    rule.enabled = !rule.enabled
    ElMessage.error(e instanceof Error ? e.message : '更新失败')
  }
}

async function deleteRule(rule: NotifyRule) {
  try {
    await ElMessageBox.confirm(`确定删除规则「${rule.name}」吗？`, '删除规则', {
      type: 'warning',
      confirmButtonText: '删除',
      cancelButtonText: '取消'
    })
    const result = await notifyService.deleteRule(rule.id)
    if (!result.ok) throw new Error(result.error.message || '删除失败')
    ElMessage.success('规则已删除')
    await loadRules()
  } catch (e: unknown) {
    if (e === 'cancel') return
    ElMessage.error(e instanceof Error ? e.message : '删除失败')
  }
}

async function createRule() {
  try {
    const result = await notifyService.createRule({
      message_type: activeType.value,
      name: `新规则 ${rules.value.length + 1}`,
      enabled: true,
      match_field: 'any',
      match_method: 'all',
      match_content: '',
      target_channels: [],
      title_template: '',
      body_mode: 'plain',
      body_template: ''
    })
    if (!result.ok) throw new Error(result.error.message || '创建失败')
    ElMessage.success('规则已创建')
    await loadRules()
  } catch (e: unknown) {
    ElMessage.error(e instanceof Error ? e.message : '创建失败')
  }
}

onMounted(() => {
  loadCounts()
  loadRules()
})
</script>

<template>
  <div class="grid grid-cols-1 sm:grid-cols-[220px_minmax(0,1fr)] gap-6">
    <div class="ui-card p-3">
      <div class="px-2 pb-2 text-xs font-bold text-gray-500 uppercase tracking-wider">消息类型</div>
      <div class="space-y-1">
        <button
          v-for="t in MESSAGE_TYPES"
          :key="t.key"
          type="button"
          class="w-full flex items-center justify-between px-3 py-2 rounded-lg text-sm transition-colors"
          :class="[
            activeType === t.key
              ? 'bg-indigo-50 dark:bg-indigo-500/10 text-indigo-600 dark:text-indigo-400 font-bold'
              : 'text-gray-600 dark:text-gray-300 hover:bg-gray-50 dark:hover:bg-white/5',
            !t.supported && 'opacity-50 cursor-not-allowed'
          ]"
          @click="selectType(t.key)"
        >
          <span>{{ t.label }}</span>
          <span class="text-xs text-gray-400">({{ counts[t.key]?.enabled ?? 0 }}/{{ counts[t.key]?.total ?? 0 }})</span>
        </button>
      </div>
    </div>

    <div class="space-y-4">
      <div v-if="!activeTypeMeta?.supported" class="ui-card p-8">
        <EmptyState title="该消息类型暂不支持" subtitle="该类型的转发规则尚未开放配置" />
      </div>

      <template v-else>
        <div class="flex items-center justify-between">
          <div class="text-sm text-gray-500">{{ activeTypeMeta?.label }} 规则 <span class="font-bold text-gray-700 dark:text-gray-200">共 {{ rules.length }} 条</span></div>
          <el-button type="primary" @click="createRule" class="!border-0">
            <el-icon><Add20Regular /></el-icon>
            <span class="ml-1">新建规则</span>
          </el-button>
        </div>

        <div v-loading="loading" class="space-y-4">
          <EmptyState v-if="!loading && rules.length === 0" title="暂无转发规则" subtitle="点击右上角新建规则，未匹配到规则的消息将不会转发" />

          <div v-for="rule in rules" :key="rule.id" class="ui-card p-6">
            <div class="flex items-center justify-between gap-3 mb-4">
              <div class="flex items-center gap-3 min-w-0">
                <span class="font-bold text-gray-800 dark:text-gray-100 truncate">{{ rule.name }}</span>
                <el-tag size="small" :type="rule.enabled ? 'success' : 'info'">{{ rule.enabled ? '已启用' : '已停用' }}</el-tag>
                <span class="text-xs text-gray-400">已绑定 {{ rule.target_channels.length }} 个通道</span>
              </div>
              <div class="flex items-center gap-2 shrink-0">
                <el-switch v-model="rule.enabled" @change="toggleEnabled(rule)" />
                <el-button text @click="toggleExpand(rule.id)">
                  <el-icon><component :is="expandedIds.has(rule.id) ? ChevronUp20Regular : ChevronDown20Regular" /></el-icon>
                </el-button>
                <el-button type="danger" plain circle size="small" :disabled="rule.is_default" @click="deleteRule(rule)">
                  <el-icon><Delete20Regular /></el-icon>
                </el-button>
              </div>
            </div>

            <div v-show="expandedIds.has(rule.id)" class="space-y-4">
              <div class="grid grid-cols-1 sm:grid-cols-2 gap-4">
                <div class="space-y-1">
                  <label class="text-xs font-bold text-gray-500 uppercase tracking-wider">规则名称</label>
                  <el-input v-model="rule.name" placeholder="规则名称" />
                </div>
                <div class="space-y-1">
                  <label class="text-xs font-bold text-gray-500 uppercase tracking-wider">匹配字段</label>
                  <el-select v-model="rule.match_field" class="w-full">
                    <el-option label="任意内容" value="any" />
                    <el-option label="短信内容" value="content" />
                    <el-option label="发送方号码" value="sender" />
                  </el-select>
                </div>
              </div>

              <div class="grid grid-cols-1 sm:grid-cols-2 gap-4">
                <div class="space-y-1">
                  <label class="text-xs font-bold text-gray-500 uppercase tracking-wider">匹配方式</label>
                  <el-select v-model="rule.match_method" class="w-full">
                    <el-option label="全部匹配" value="all" />
                    <el-option label="包含" value="contains" />
                    <el-option label="不包含" value="not_contains" />
                    <el-option label="等于" value="equals" />
                    <el-option label="正则表达式" value="regex" />
                  </el-select>
                </div>
                <div class="space-y-1">
                  <label class="text-xs font-bold text-gray-500 uppercase tracking-wider">匹配内容</label>
                  <el-input v-model="rule.match_content" :disabled="rule.match_method === 'all'" placeholder="匹配的关键字/表达式" />
                </div>
              </div>

              <div class="space-y-1">
                <label class="text-xs font-bold text-gray-500 uppercase tracking-wider">发送通道</label>
                <el-checkbox-group v-model="rule.target_channels" class="flex flex-wrap gap-3 mt-1">
                  <el-checkbox v-for="c in CHANNEL_OPTIONS" :key="c.key" :value="c.key" :label="c.label" border />
                </el-checkbox-group>
              </div>

              <div class="space-y-1">
                <label class="text-xs font-bold text-gray-500 uppercase tracking-wider">标题模板</label>
                <TemplateVariableInput v-model="rule.title_template" :variables="activeTemplateVariables" placeholder="留空则不附加标题" />
              </div>

              <div class="space-y-1">
                <label class="text-xs font-bold text-gray-500 uppercase tracking-wider">消息模板</label>
                <el-tabs v-model="rule.body_mode" class="notify-body-mode-tabs">
                  <el-tab-pane label="纯文本" name="plain">
                    <TemplateVariableInput
                      v-model="rule.body_template"
                      type="textarea"
                      :rows="4"
                      :variables="activeTemplateVariables"
                      placeholder="留空则直接转发原始短信内容"
                    />
                  </el-tab-pane>
                  <el-tab-pane label="自定义请求体" name="custom_json">
                    <TemplateVariableInput
                      v-model="rule.body_template"
                      type="textarea"
                      :rows="4"
                      :variables="activeTemplateVariables"
                      placeholder='{"text": "{{content}}"}'
                    />
                    <div class="text-[10px] text-gray-400 mt-1">仅 Webhook 通道支持自定义请求体，其余通道将回退为纯文本发送。</div>
                  </el-tab-pane>
                </el-tabs>
              </div>

              <div class="flex justify-end pt-2">
                <el-button type="primary" :loading="savingIds.has(rule.id)" @click="saveRule(rule)" class="!border-0">保存规则</el-button>
              </div>
            </div>
          </div>
        </div>
      </template>
    </div>
  </div>
</template>

<style scoped>
:deep(.notify-body-mode-tabs .el-tabs__header) {
  margin-bottom: 12px;
}
</style>
