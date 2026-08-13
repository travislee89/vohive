<script setup lang="ts">
import type { PropType } from 'vue'
import LoadingScreen from '../components/LoadingScreen.vue'
import SwitchDark from '../components/SwitchDark.vue'
import type { ThemeMode } from '../theme'

defineProps({
  theme: {
    type: String as PropType<ThemeMode>,
    required: true
  }
})

const emit = defineEmits(['toggle-theme'])
</script>

<template>
  <div class="h-screen flex items-center justify-center bg-gray-100 dark:bg-gray-950 transition-colors duration-300">
    <div class="absolute top-4 right-4 z-50">
      <SwitchDark :theme="theme" @toggle="() => emit('toggle-theme')" />
    </div>
    <router-view v-slot="{ Component }">
      <Suspense>
        <template #default>
          <component :is="Component" />
        </template>
        <template #fallback>
          <LoadingScreen />
        </template>
      </Suspense>
    </router-view>
  </div>
</template>
