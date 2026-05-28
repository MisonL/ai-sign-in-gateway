<script setup lang="ts">
defineProps<{
  totalSiteCount: number
  enabledSiteCount: number
  readyGatewayCount: number
  totalBalanceSummary: string
  totalBalanceTone: string
  quantifiedBalanceSiteCount: number
  successSiteCount: number
  failedSiteCount: number
  pendingSiteCount: number
}>()
</script>

<template>
  <a-row :gutter="[16, 16]" class="sites-metric-grid">
    <a-col :xs="24" :sm="12" :xl="8" :xxl="4">
      <a-card :bordered="false" class="admin-card stat-card stat-card--signal sites-stat-card">
        <div class="sites-stat-card__meta">
          <div class="sites-stat-card__eyebrow">站点总数</div>
          <div class="sites-stat-card__value">{{ totalSiteCount }}</div>
          <div class="sites-stat-card__desc">所有已配置站点</div>
        </div>
        <div class="sites-stat-card__icon sites-stat-card__icon--sheet" />
      </a-card>
    </a-col>
    <a-col :xs="24" :sm="12" :xl="8" :xxl="4">
      <a-card :bordered="false" class="admin-card stat-card stat-card--signal sites-stat-card">
        <div class="sites-stat-card__meta">
          <div class="sites-stat-card__eyebrow">已启用</div>
          <div class="sites-stat-card__value">{{ enabledSiteCount }}</div>
          <div class="sites-stat-card__desc">当前启用站点</div>
        </div>
        <div class="sites-stat-card__icon sites-stat-card__icon--stack" />
      </a-card>
    </a-col>
    <a-col :xs="24" :sm="12" :xl="8" :xxl="4">
      <a-card :bordered="false" class="admin-card stat-card stat-card--signal sites-stat-card">
        <div class="sites-stat-card__meta">
          <div class="sites-stat-card__eyebrow">网关就绪</div>
          <div class="sites-stat-card__value">{{ readyGatewayCount }}</div>
          <div class="sites-stat-card__desc">可稳定转发站点</div>
        </div>
        <div class="sites-stat-card__icon sites-stat-card__icon--gateway" />
      </a-card>
    </a-col>
    <a-col :xs="24" :sm="12" :xl="8" :xxl="6">
      <a-card :bordered="false" class="admin-card stat-card stat-card--signal sites-stat-card sites-stat-card--balance">
        <div class="sites-stat-card__meta">
          <div class="sites-stat-card__eyebrow">余额合计</div>
          <div
            class="sites-stat-card__value sites-stat-card__value--balance"
            :class="`sites-stat-card__value--${totalBalanceTone}`"
            :title="totalBalanceSummary"
          >
            {{ totalBalanceSummary }}
          </div>
          <div class="sites-stat-card__desc">{{ quantifiedBalanceSiteCount }} 个站点</div>
        </div>
        <div class="sites-stat-card__icon sites-stat-card__icon--wallet" />
      </a-card>
    </a-col>
    <a-col :xs="24" :sm="12" :xl="8" :xxl="3">
      <a-card :bordered="false" class="admin-card stat-card stat-card--signal sites-stat-card">
        <div class="sites-stat-card__meta">
          <div class="sites-stat-card__eyebrow">状态成功</div>
          <div class="sites-stat-card__value">{{ successSiteCount }}</div>
          <div class="sites-stat-card__desc">连通检测成功</div>
        </div>
        <div class="sites-stat-card__icon sites-stat-card__icon--check" />
      </a-card>
    </a-col>
    <a-col :xs="24" :sm="12" :xl="8" :xxl="3">
      <a-card :bordered="false" class="admin-card stat-card stat-card--signal sites-stat-card">
        <div class="sites-stat-card__meta">
          <div class="sites-stat-card__eyebrow">失败 / 未测</div>
          <div class="sites-stat-card__value">{{ `${failedSiteCount} / ${pendingSiteCount}` }}</div>
          <div class="sites-stat-card__desc">失败 / 未检测站点</div>
        </div>
        <div class="sites-stat-card__icon sites-stat-card__icon--users" />
      </a-card>
    </a-col>
  </a-row>
</template>
