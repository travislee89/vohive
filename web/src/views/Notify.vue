<script setup lang="ts">
import { ref } from 'vue'
import PageHeader from '../components/PageHeader.vue'
import ChannelSettingsPanel from '../components/notify/ChannelSettingsPanel.vue'
import RulesPanel from '../components/notify/RulesPanel.vue'
import LogsPanel from '../components/notify/LogsPanel.vue'

const activeTab = ref('logs')
</script>

<template>
  <div class="max-w-7xl mx-auto">
    <PageHeader title="通知中心" subtitle="管理转发日志、转发规则与转发通道" />

    <el-tabs v-model="activeTab" class="notify-center-tabs">
      <el-tab-pane label="转发日志" name="logs">
        <LogsPanel v-if="activeTab === 'logs'" />
      </el-tab-pane>
      <el-tab-pane label="转发规则" name="rules">
        <RulesPanel v-if="activeTab === 'rules'" />
      </el-tab-pane>
      <el-tab-pane label="转发通道" name="channels">
        <ChannelSettingsPanel v-if="activeTab === 'channels'" />
      </el-tab-pane>
    </el-tabs>
  </div>
</template>

<style scoped>
:deep(.notify-center-tabs) {
  border: none;
  background: transparent;
}
:deep(.notify-center-tabs .el-tabs__header) {
  margin-bottom: 24px;
  background-color: var(--el-fill-color-light);
  border-radius: 12px;
  border-bottom: none;
  display: inline-flex;
  padding: 4px;
}
:deep(.notify-center-tabs .el-tabs__nav-wrap::after) {
  display: none;
}
:deep(.notify-center-tabs .el-tabs__active-bar) {
  display: none;
}
:deep(.notify-center-tabs .el-tabs__item) {
  height: 38px;
  line-height: 38px;
  padding: 0 20px !important;
  border-radius: 8px;
  margin-right: 4px;
  color: var(--el-text-color-regular);
  transition: all 0.3s cubic-bezier(0.4, 0, 0.2, 1);
  font-weight: 500;
}
:deep(.notify-center-tabs .el-tabs__item:last-child) {
  margin-right: 0;
}
:deep(.notify-center-tabs .el-tabs__item:hover) {
  color: var(--el-color-primary);
}
:deep(.notify-center-tabs .el-tabs__item.is-active) {
  background-color: var(--el-bg-color);
  color: var(--el-color-primary);
  font-weight: 600;
  box-shadow: 0 1px 4px rgba(0, 0, 0, 0.05), 0 2px 8px rgba(0, 0, 0, 0.03);
}
</style>
