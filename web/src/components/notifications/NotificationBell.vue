<script setup lang="ts">
import { useRouter } from 'vue-router'
import { Alert24Regular } from '@vicons/fluent'
import { useNotificationsStore } from '../../stores/notifications'
import { formatISODateTime } from '../../utils/datetime'
import type { NotificationFeedItem } from '../../types/api'

const router = useRouter()
const store = useNotificationsStore()

function onShow() {
  void store.fetchFeed()
}

function itemKey(item: NotificationFeedItem) {
  return item.kind === 'sms' ? `sms:${item.imsi}|${item.peer}` : `call:${item.call_id}`
}

const outcomeText: Record<string, string> = {
  ringing: '振铃中',
  answered: '已接听',
  missed: '未接听'
}

function onItemClick(item: NotificationFeedItem) {
  if (item.kind === 'sms') {
    const contact = `${item.imsi || ''}|${item.peer || ''}`
    void router.push({ name: 'SMS', query: { device: item.device_id, contact } })
    void store.markSmsRead(item.imsi, item.peer || '')
  } else {
    void router.push({ name: 'Calls', query: { device: item.device_id, callId: String(item.call_id) } })
    void store.markCallRead(item.call_id as number)
  }
}

function onClearAll() {
  void store.clearAll()
}
</script>

<template>
  <el-popover trigger="click" :width="360" placement="bottom-end" @show="onShow">
    <template #reference>
      <el-badge :value="store.totalUnread" :max="99" :hidden="store.totalUnread === 0" class="notif-bell-badge">
        <el-button text circle class="!px-2">
          <el-icon :size="20"><Alert24Regular /></el-icon>
        </el-button>
      </el-badge>
    </template>
    <div class="notif-panel">
      <div class="notif-panel-header">
        <span class="notif-panel-title">通知中心</span>
        <el-button
          text
          size="small"
          class="notif-clear-all-btn"
          :disabled="store.totalUnread === 0 && store.feed.length === 0"
          @click="onClearAll"
        >清理全部通知</el-button>
      </div>
      <div v-if="store.feed.length === 0" class="notif-empty">暂无新通知</div>
      <div v-else class="notif-list">
        <div
          v-for="item in store.feed"
          :key="itemKey(item)"
          class="notif-item"
          @click="onItemClick(item)"
        >
          <template v-if="item.kind === 'sms'">
            <div class="notif-item-row">
              <span class="notif-item-title">{{ item.peer }}</span>
              <span class="notif-item-time">{{ formatISODateTime(item.timestamp) }}</span>
            </div>
            <div class="notif-item-sub">{{ item.device_name }} · {{ item.preview }}</div>
          </template>
          <template v-else>
            <div class="notif-item-row">
              <span class="notif-item-title">{{ item.number }}</span>
              <span class="notif-item-time">{{ formatISODateTime(item.timestamp) }}</span>
            </div>
            <div class="notif-item-sub">{{ item.device_name }} · {{ outcomeText[item.outcome || ''] || item.outcome }}</div>
          </template>
        </div>
      </div>
    </div>
  </el-popover>
</template>

<style scoped>
.notif-panel-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 8px;
  margin-bottom: 8px;
}

.notif-panel-title {
  font-weight: 600;
  font-size: 0.85rem;
}

.notif-clear-all-btn {
  font-size: 0.76rem;
  padding: 0 !important;
  height: auto !important;
}

.notif-empty {
  font-size: 0.85rem;
  color: var(--el-text-color-secondary, #94a3b8);
  padding: 12px 0;
  text-align: center;
}

.notif-list {
  max-height: 320px;
  overflow-y: auto;
}

.notif-item {
  padding: 8px 4px;
  border-bottom: 1px solid var(--el-border-color-lighter, rgba(0, 0, 0, 0.06));
  cursor: pointer;
}

.notif-item:last-child {
  border-bottom: none;
}

.notif-item:hover {
  background: var(--el-fill-color-light, rgba(0, 0, 0, 0.03));
}

.notif-item-row {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 8px;
}

.notif-item-title {
  font-weight: 600;
  font-size: 0.85rem;
}

.notif-item-time {
  font-size: 0.72rem;
  color: var(--el-text-color-secondary, #94a3b8);
  white-space: nowrap;
}

.notif-item-sub {
  font-size: 0.78rem;
  color: var(--el-text-color-secondary, #94a3b8);
  margin-top: 2px;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
</style>
