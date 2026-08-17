<script setup lang="ts">
import { ref } from 'vue'
import PageHeader from '../components/PageHeader.vue'
import TaskListPanel from '../components/automation/TaskListPanel.vue'
import RunLogPanel from '../components/automation/RunLogPanel.vue'

const activeTab = ref('tasks')
</script>

<template>
  <div class="max-w-7xl mx-auto">
    <PageHeader title="自动化中心" subtitle="定时或按间隔自动执行重启基带、发送短信等任务" />

    <el-tabs v-model="activeTab" class="automation-center-tabs">
      <el-tab-pane label="自动化控制台" name="tasks">
        <TaskListPanel v-if="activeTab === 'tasks'" />
      </el-tab-pane>
      <el-tab-pane label="运行日志" name="logs">
        <RunLogPanel v-if="activeTab === 'logs'" />
      </el-tab-pane>
    </el-tabs>
  </div>
</template>

<style scoped>
:deep(.automation-center-tabs) {
  border: none;
  background: transparent;
}
:deep(.automation-center-tabs .el-tabs__header) {
  margin-bottom: 24px;
  background-color: var(--el-fill-color-light);
  border-radius: 12px;
  border-bottom: none;
  display: inline-flex;
  padding: 4px;
}
:deep(.automation-center-tabs .el-tabs__nav-wrap::after) {
  display: none;
}
:deep(.automation-center-tabs .el-tabs__active-bar) {
  display: none;
}
:deep(.automation-center-tabs .el-tabs__item) {
  height: 38px;
  line-height: 38px;
  padding: 0 20px !important;
  border-radius: 8px;
  margin-right: 4px;
  color: var(--el-text-color-regular);
  transition: all 0.3s cubic-bezier(0.4, 0, 0.2, 1);
  font-weight: 500;
}
:deep(.automation-center-tabs .el-tabs__item:last-child) {
  margin-right: 0;
}
:deep(.automation-center-tabs .el-tabs__item:hover) {
  color: var(--el-color-primary);
}
:deep(.automation-center-tabs .el-tabs__item.is-active) {
  background-color: var(--el-bg-color);
  color: var(--el-color-primary);
  font-weight: 600;
  box-shadow: 0 1px 4px rgba(0, 0, 0, 0.05), 0 2px 8px rgba(0, 0, 0, 0.03);
}
</style>
