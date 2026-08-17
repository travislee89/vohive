<script setup lang="ts">
import { nextTick, ref } from 'vue'
import type { InputInstance } from 'element-plus'

export type TemplateVariable = { key: string; label: string; disabled?: boolean }

const props = withDefaults(
  defineProps<{
    modelValue: string
    variables: TemplateVariable[]
    placeholder?: string
    type?: 'text' | 'textarea'
    rows?: number
    resize?: 'none' | 'both' | 'horizontal' | 'vertical'
  }>(),
  { type: 'text', rows: 3, placeholder: '', resize: 'vertical' }
)

const emit = defineEmits<{ 'update:modelValue': [value: string] }>()

const inputRef = ref<InputInstance>()

function insertVariable(key: string) {
  const token = `{{${key}}}`
  const el = (props.type === 'textarea' ? inputRef.value?.textarea : inputRef.value?.input) as
    | HTMLInputElement
    | HTMLTextAreaElement
    | undefined

  const current = props.modelValue || ''
  if (!el) {
    emit('update:modelValue', current + token)
    return
  }

  const start = el.selectionStart ?? current.length
  const end = el.selectionEnd ?? current.length
  const next = current.slice(0, start) + token + current.slice(end)
  emit('update:modelValue', next)

  nextTick(() => {
    const pos = start + token.length
    el.focus()
    el.setSelectionRange(pos, pos)
  })
}
</script>

<template>
  <div class="space-y-2 w-full">
    <div class="flex flex-wrap gap-1.5">
      <el-tag
        v-for="v in variables"
        :key="v.key"
        size="small"
        :type="v.disabled ? 'info' : 'primary'"
        :effect="v.disabled ? 'plain' : 'light'"
        class="!cursor-pointer select-none"
        :class="{ 'opacity-50': v.disabled }"
        @click="!v.disabled && insertVariable(v.key)"
      >
        {{ v.label }}
      </el-tag>
    </div>
    <el-input
      ref="inputRef"
      :model-value="modelValue"
      :type="type"
      :rows="type === 'textarea' ? rows : undefined"
      :resize="type === 'textarea' ? resize : undefined"
      :placeholder="placeholder"
      @update:model-value="(val: string) => emit('update:modelValue', val)"
    />
  </div>
</template>
