<script setup lang="ts">
import { computed, reactive, ref, watch } from 'vue'
import { ElMessage } from 'element-plus'
import { automationService } from '../../services/automation'
import { devicesService } from '../../services/devices'
import type { AutomationActionType, AutomationIntervalUnit, AutomationTask, AutomationTriggerType } from '../../types/automation'
import TemplateVariableInput from '../TemplateVariableInput.vue'

const props = defineProps<{
  modelValue: boolean
  task?: AutomationTask | null
}>()

const emit = defineEmits<{
  'update:modelValue': [value: boolean]
  saved: []
}>()

const SMS_TEMPLATE_VARIABLES = [
  { key: '时间', label: '当前时间' },
  { key: '随机字符串', label: '随机字符串' }
]

const WEEKDAY_OPTIONS = [
  { value: 1, label: '一' },
  { value: 2, label: '二' },
  { value: 3, label: '三' },
  { value: 4, label: '四' },
  { value: 5, label: '五' },
  { value: 6, label: '六' },
  { value: 7, label: '日' }
]

const HHMM_PATTERN = /^([01]\d|2[0-3]):[0-5]\d$/

type DeviceOption = { id: string; name: string }

const devices = ref<DeviceOption[]>([])
const saving = ref(false)

const isEdit = computed(() => !!props.task)

const form = reactive({
  name: '',
  actionType: 'reboot' as AutomationActionType,
  deviceId: '',
  smsPhone: '',
  smsContent: '',
  smsDelayMinSec: 0,
  smsDelayMaxSec: 0,
  smsRetryCount: 0,
  triggerType: 'fixed_schedule' as AutomationTriggerType,
  weekdays: [] as number[],
  fixedTimesText: '04:00',
  intervalValue: 1,
  intervalUnit: 'days' as AutomationIntervalUnit
})

function resetForm() {
  form.name = ''
  form.actionType = 'reboot'
  form.deviceId = ''
  form.smsPhone = ''
  form.smsContent = ''
  form.smsDelayMinSec = 0
  form.smsDelayMaxSec = 0
  form.smsRetryCount = 0
  form.triggerType = 'fixed_schedule'
  form.weekdays = []
  form.fixedTimesText = '04:00'
  form.intervalValue = 1
  form.intervalUnit = 'days'
}

function loadFromTask(task: AutomationTask) {
  form.name = task.name
  form.actionType = task.action_type
  form.deviceId = task.device_id
  form.smsPhone = task.sms_phone || ''
  form.smsContent = task.sms_content || ''
  form.smsDelayMinSec = task.sms_delay_min_sec || 0
  form.smsDelayMaxSec = task.sms_delay_max_sec || 0
  form.smsRetryCount = task.sms_retry_count || 0
  form.triggerType = task.trigger_type
  form.weekdays = task.weekdays ? [...task.weekdays] : []
  form.fixedTimesText = (task.fixed_times && task.fixed_times.length > 0 ? task.fixed_times : ['04:00']).join(', ')
  form.intervalValue = task.interval_value || 1
  form.intervalUnit = task.interval_unit || 'days'
}

watch(
  () => props.modelValue,
  async (open) => {
    if (!open) return
    if (devices.value.length === 0) {
      const result = await devicesService.listManaged()
      if (result.ok) {
        devices.value = result.data.devices.map((d) => ({ id: d.id, name: d.name || d.id }))
      }
    }
    if (props.task) {
      loadFromTask(props.task)
    } else {
      resetForm()
    }
  }
)

function parseFixedTimes(): string[] | null {
  const raw = form.fixedTimesText
    .split(/[,，]/)
    .map((s) => s.trim())
    .filter((s) => s.length > 0)
  if (raw.length === 0) return null
  for (const t of raw) {
    if (!HHMM_PATTERN.test(t)) return null
  }
  return raw
}

function validate(): string | null {
  if (!form.name.trim()) return '任务名称不能为空'
  if (!form.deviceId) return '请选择设备'
  if (form.actionType === 'sms') {
    if (!form.smsPhone.trim()) return '接收号码不能为空'
    if (!form.smsContent.trim()) return '短信内容不能为空'
    if (form.smsDelayMinSec < 0 || form.smsDelayMaxSec < 0) return '随机延迟范围不能为负数'
    if (form.smsDelayMinSec > form.smsDelayMaxSec) return '随机延迟范围下限不能大于上限'
    if (form.smsRetryCount < 0) return '失败重试次数不能为负数'
  }
  if (form.triggerType === 'fixed_schedule') {
    if (!parseFixedTimes()) return '触发时刻格式有误，请输入英文或中文逗号隔开的 HH:MM 时刻，例如 04:00, 16:30'
  } else if (form.intervalValue <= 0) {
    return '间隔时长必须大于 0'
  }
  return null
}

async function handleSave() {
  const error = validate()
  if (error) {
    ElMessage.error(error)
    return
  }

  const payload = {
    name: form.name.trim(),
    action_type: form.actionType,
    device_id: form.deviceId,
    trigger_type: form.triggerType,
    ...(form.actionType === 'sms'
      ? {
          sms_phone: form.smsPhone.trim(),
          sms_content: form.smsContent,
          sms_delay_min_sec: form.smsDelayMinSec,
          sms_delay_max_sec: form.smsDelayMaxSec,
          sms_retry_count: form.smsRetryCount
        }
      : {}),
    ...(form.triggerType === 'fixed_schedule'
      ? { fixed_times: parseFixedTimes() || [], weekdays: form.weekdays }
      : { interval_value: form.intervalValue, interval_unit: form.intervalUnit })
  }

  saving.value = true
  try {
    const result = isEdit.value
      ? await automationService.updateTask(props.task!.id, payload)
      : await automationService.createTask(payload)
    if (!result.ok) throw new Error(result.error.message || '保存失败')
    ElMessage.success('任务已保存')
    emit('update:modelValue', false)
    emit('saved')
  } catch (e: unknown) {
    ElMessage.error(e instanceof Error ? e.message : '保存失败')
  } finally {
    saving.value = false
  }
}

function handleClose() {
  emit('update:modelValue', false)
}
</script>

<template>
  <el-dialog
    :model-value="modelValue"
    :title="isEdit ? '编辑自动化任务' : '添加自动化任务'"
    width="520px"
    @update:model-value="(v: boolean) => emit('update:modelValue', v)"
  >
    <el-form label-position="top">
      <el-form-item label="任务名称">
        <el-input v-model="form.name" placeholder="任务名称" />
      </el-form-item>

      <el-form-item label="执行动作">
        <el-select v-model="form.actionType" class="w-full">
          <el-option label="重启基带" value="reboot" />
          <el-option label="发送短信" value="sms" />
        </el-select>
      </el-form-item>

      <el-form-item label="设备">
        <el-select v-model="form.deviceId" placeholder="选择设备" class="w-full" filterable>
          <el-option label="全部设备" value="all" />
          <el-option v-for="d in devices" :key="d.id" :label="d.name" :value="d.id" />
        </el-select>
      </el-form-item>

      <template v-if="form.actionType === 'sms'">
        <el-form-item label="接收号码">
          <el-input v-model="form.smsPhone" placeholder="接收号码" />
        </el-form-item>
        <el-form-item label="短信内容">
          <TemplateVariableInput v-model="form.smsContent" type="textarea" :rows="3" resize="none" :variables="SMS_TEMPLATE_VARIABLES" placeholder="可在内容中插入变量，短信发送时会自动替换" />
        </el-form-item>
        <div class="grid grid-cols-2 gap-4">
          <el-form-item label="随机延迟范围 (秒)">
            <div class="flex items-center gap-2">
              <el-input-number v-model="form.smsDelayMinSec" :min="0" controls-position="right" class="w-full" />
              <span class="text-gray-400">-</span>
              <el-input-number v-model="form.smsDelayMaxSec" :min="0" controls-position="right" class="w-full" />
            </div>
          </el-form-item>
          <el-form-item label="失败重试次数">
            <el-input-number v-model="form.smsRetryCount" :min="0" controls-position="right" class="w-full" />
          </el-form-item>
        </div>
      </template>

      <el-form-item label="触发机制">
        <el-select v-model="form.triggerType" class="w-full">
          <el-option label="定点定时" value="fixed_schedule" />
          <el-option label="时间间隔" value="interval" />
        </el-select>
      </el-form-item>

      <template v-if="form.triggerType === 'fixed_schedule'">
        <el-form-item label="重复星期（不选则每天都触发）">
          <div class="flex flex-wrap gap-2">
            <el-check-tag
              v-for="opt in WEEKDAY_OPTIONS"
              :key="opt.value"
              :checked="form.weekdays.includes(opt.value)"
              @change="(checked: boolean) => {
                const idx = form.weekdays.indexOf(opt.value)
                if (checked && idx < 0) form.weekdays.push(opt.value)
                else if (!checked && idx >= 0) form.weekdays.splice(idx, 1)
              }"
            >
              {{ opt.label }}
            </el-check-tag>
          </div>
        </el-form-item>
        <el-form-item label="触发时刻 (HH:MM，多个用逗号隔开)">
          <el-input v-model="form.fixedTimesText" placeholder="04:00, 16:30" />
          <div class="text-[10px] text-gray-400 mt-1">输入英文或中文逗号隔开的 HH:MM 时刻，例如 04:00, 16:30</div>
        </el-form-item>
      </template>

      <template v-else>
        <div class="grid grid-cols-2 gap-4">
          <el-form-item label="间隔时长">
            <el-input-number v-model="form.intervalValue" :min="1" controls-position="right" class="w-full" />
          </el-form-item>
          <el-form-item label="时间单位">
            <el-select v-model="form.intervalUnit" class="w-full">
              <el-option label="小时" value="hours" />
              <el-option label="天" value="days" />
            </el-select>
          </el-form-item>
        </div>
      </template>
    </el-form>

    <template #footer>
      <el-button @click="handleClose">取消</el-button>
      <el-button type="primary" :loading="saving" @click="handleSave">保存</el-button>
    </template>
  </el-dialog>
</template>
