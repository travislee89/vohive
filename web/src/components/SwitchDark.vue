<script setup lang="ts">
import { computed } from 'vue'
import { WeatherMoon24Regular, WeatherSunny24Regular, BrightnessHigh24Regular } from '@vicons/fluent'
import type { ThemeMode } from '../theme'

const props = defineProps<{
  theme: ThemeMode
}>()

const emit = defineEmits<{
  toggle: []
}>()

const MODE_LABEL: Record<ThemeMode, string> = {
  light: '白色',
  dark: '深色',
  auto: '自动'
}

const label = computed(() => MODE_LABEL[props.theme])

const icon = computed(() => {
  if (props.theme === 'dark') return WeatherSunny24Regular
  if (props.theme === 'auto') return BrightnessHigh24Regular
  return WeatherMoon24Regular
})

function onToggle() {
  emit('toggle')
}
</script>

<template>
  <el-tooltip :content="`主题：${label}`" placement="bottom" :show-after="300">
    <el-button circle @click="onToggle" class="!border-0 !bg-gray-100/70 dark:!bg-white/5">
      <el-icon><component :is="icon" /></el-icon>
    </el-button>
  </el-tooltip>
</template>
