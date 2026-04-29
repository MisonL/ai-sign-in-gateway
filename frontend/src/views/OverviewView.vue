<script setup lang="ts">
import { ReloadOutlined } from '@ant-design/icons-vue'
import { computed, onMounted, ref } from 'vue'
import { getOverview } from '../api'
import ShellLayout from '../components/ShellLayout.vue'
import StatusPill from '../components/StatusPill.vue'
import { useToast } from '../toast'
import type { OverviewData } from '../types'

const toast = useToast()
const data = ref<OverviewData | null>(null)
const loading = ref(false)

type MetricTone = 'primary' | 'info' | 'success' | 'warning'

const metrics = computed<Array<{
  key: string
  title: string
  value: number
  suffix: string
  tone: MetricTone
  caption: string
}>>(() => {
  if (!data.value) {
    return []
  }
  const d = data.value
  const total = d.today_success + d.today_failed
  const ratio = total > 0 ? Math.round((d.today_success / total) * 1000) / 10 : 0
  return [
    {
      key: 'sites',
      title: '站点总数',
      value: d.site_count,
      suffix: '个站点',
      tone: 'primary',
      caption: `已启用 ${d.enabled_site_count} 个`,
    },
    {
      key: 'enabled',
      title: '启用站点',
      value: d.enabled_site_count,
      suffix: '参与计划任务',
      tone: 'info',
      caption: `占总数 ${
        d.site_count > 0 ? Math.round((d.enabled_site_count / d.site_count) * 100) : 0
      }%`,
    },
    {
      key: 'success',
      title: '今日成功',
      value: d.today_success,
      suffix: '执行成功',
      tone: 'success',
      caption: total > 0 ? `当日成功率 ${ratio}%` : '今日尚未执行',
    },
    {
      key: 'failed',
      title: '今日失败',
      value: d.today_failed,
      suffix: '待处理异常',
      tone: 'warning',
      caption: d.today_failed > 0 ? '查看待处理列表' : '今日没有异常',
    },
  ]
})

const todaySuccessRate = computed(() => {
  if (!data.value) return 0
  const total = data.value.today_success + data.value.today_failed
  return total > 0 ? Math.round((data.value.today_success / total) * 1000) / 10 : 0
})

function formatTime(value: string | null) {
  if (!value) return '暂无'
  return new Date(value).toLocaleString('zh-CN')
}

async function loadData() {
  loading.value = true
  try {
    data.value = await getOverview()
  } catch (err) {
    toast.error(err instanceof Error ? err.message : '加载失败')
  } finally {
    loading.value = false
  }
}

onMounted(loadData)
</script>

<template>
  <ShellLayout>
    <div class="page-stack page-stack--dashboard">
      <div class="page-toolbar page-toolbar--actions">
        <div class="overview-strap" v-if="data">
          <span>最新同步 <strong>{{ formatTime(data.latest_sync) }}</strong></span>
          <span class="overview-strap__sep">·</span>
          <span>下次计划 <strong>{{ formatTime(data.next_run_at) }}</strong></span>
          <span class="overview-strap__sep">·</span>
          <span>当日成功率 <strong>{{ todaySuccessRate }}%</strong></span>
        </div>
        <a-button :loading="loading" @click="loadData">
          <template #icon>
            <ReloadOutlined />
          </template>
          刷新数据
        </a-button>
      </div>

      <div class="overview-metrics">
        <a-card
          v-for="metric in metrics"
          :key="metric.key"
          :bordered="false"
          class="admin-card overview-metric-card"
          :class="`overview-metric-card--${metric.tone}`"
        >
          <div class="overview-metric-card__title">{{ metric.title }}</div>
          <div class="overview-metric-card__value">
            <a-statistic :value="metric.value" :value-style="{ fontSize: 'inherit', color: 'inherit', fontWeight: 700 }" />
          </div>
          <div class="overview-metric-card__caption">{{ metric.caption }}</div>
        </a-card>
      </div>

      <a-row v-if="data" :gutter="[16, 16]" class="page-grid-fill">
        <a-col :xs="24" :xl="14">
          <a-card :bordered="false" class="admin-card admin-card--fill overview-list-card" title="最近任务">
            <template #extra>最新同步：{{ formatTime(data.latest_sync) }}</template>
            <div class="card-scroll card-scroll--padded overview-list-scroll">
              <ul v-if="data.recent_runs.length" class="overview-feed">
                <li
                  v-for="run in data.recent_runs"
                  :key="run.id"
                  class="overview-feed__row"
                  :class="`overview-feed__row--${run.status === 'success' ? 'success' : run.status === 'failed' ? 'failed' : 'neutral'}`"
                >
                  <span class="overview-feed__dot" />
                  <div class="overview-feed__main">
                    <div class="overview-feed__title">
                      <strong>{{ run.site_name ?? '未知站点' }}</strong>
                      <StatusPill :value="run.status" />
                    </div>
                    <p class="overview-feed__text">{{ run.message || '暂无消息' }}</p>
                  </div>
                  <span class="overview-feed__time">{{ formatTime(run.started_at) }}</span>
                </li>
              </ul>
              <a-empty v-else description="暂无最近任务记录。" />
            </div>
          </a-card>
        </a-col>

        <a-col :xs="24" :xl="10">
          <a-card :bordered="false" class="admin-card admin-card--fill overview-list-card" title="待处理站点">
            <template #extra>下次计划：{{ formatTime(data.next_run_at) }}</template>
            <div class="card-scroll card-scroll--padded overview-list-scroll">
              <ul v-if="data.attention_sites.length" class="overview-feed">
                <li
                  v-for="site in data.attention_sites"
                  :key="site.id"
                  class="overview-feed__row overview-feed__row--attention"
                >
                  <span class="overview-feed__dot" />
                  <div class="overview-feed__main">
                    <div class="overview-feed__title">
                      <strong>{{ site.name }}</strong>
                      <StatusPill :value="site.last_status" />
                    </div>
                    <p class="overview-feed__text">{{ site.last_message || '暂无异常说明' }}</p>
                  </div>
                  <span class="overview-feed__time">{{ formatTime(site.last_run_at) }}</span>
                </li>
              </ul>
              <a-empty v-else description="当前没有失败或停用站点。" />
            </div>
          </a-card>
        </a-col>
      </a-row>
    </div>
  </ShellLayout>
</template>

<style scoped>
.overview-strap {
  display: flex;
  align-items: center;
  flex-wrap: wrap;
  gap: 4px 10px;
  font-size: 12px;
  color: var(--text-muted);
  padding: 0;
  background: transparent;
  border: 0;
  box-shadow: none;
}

.overview-strap strong {
  margin-left: 4px;
  color: var(--text-main);
  font-weight: 600;
  font-feature-settings: 'tnum';
}

.overview-strap__sep {
  color: var(--text-faint);
}

.overview-metrics {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(220px, 1fr));
  gap: 16px;
}

.overview-metric-card.ant-card {
  position: relative;
  overflow: hidden;
  border: 1px solid var(--border-muted) !important;
  border-radius: var(--radius-container) !important;
  box-shadow: var(--shadow-card) !important;
  transition: transform 0.18s ease, box-shadow 0.18s ease;
}

.overview-metric-card.ant-card:hover {
  transform: translateY(-2px);
  box-shadow: var(--shadow-card-hover) !important;
}

.overview-metric-card.ant-card::before {
  content: '';
  position: absolute;
  inset: 0 auto 0 0;
  width: 4px;
  background: linear-gradient(180deg, rgba(79, 124, 255, 0.45), rgba(79, 124, 255, 0.08));
}

.overview-metric-card.ant-card::after {
  content: '';
  position: absolute;
  inset: -50% -30% auto auto;
  width: 220px;
  height: 220px;
  border-radius: 50%;
  background: radial-gradient(circle, rgba(79, 124, 255, 0.16) 0%, rgba(79, 124, 255, 0) 70%);
  pointer-events: none;
  filter: blur(2px);
}

.overview-metric-card--primary.ant-card {
  background: linear-gradient(155deg, rgba(233, 239, 255, 0.85) 0%, rgba(255, 255, 255, 0.95) 60%);
}
.overview-metric-card--primary.ant-card::before {
  background: linear-gradient(180deg, rgba(79, 124, 255, 0.6), rgba(79, 124, 255, 0.12));
}
.overview-metric-card--primary.ant-card::after {
  background: radial-gradient(circle, rgba(79, 124, 255, 0.18) 0%, rgba(79, 124, 255, 0) 70%);
}

.overview-metric-card--info.ant-card {
  background: linear-gradient(155deg, rgba(230, 241, 255, 0.85) 0%, rgba(255, 255, 255, 0.95) 60%);
}
.overview-metric-card--info.ant-card::before {
  background: linear-gradient(180deg, rgba(94, 163, 255, 0.55), rgba(94, 163, 255, 0.12));
}
.overview-metric-card--info.ant-card::after {
  background: radial-gradient(circle, rgba(94, 163, 255, 0.18) 0%, rgba(94, 163, 255, 0) 70%);
}

.overview-metric-card--success.ant-card {
  background: linear-gradient(155deg, rgba(231, 245, 236, 0.9) 0%, rgba(255, 255, 255, 0.95) 60%);
}
.overview-metric-card--success.ant-card::before {
  background: linear-gradient(180deg, rgba(52, 168, 83, 0.55), rgba(52, 168, 83, 0.12));
}
.overview-metric-card--success.ant-card::after {
  background: radial-gradient(circle, rgba(52, 168, 83, 0.16) 0%, rgba(52, 168, 83, 0) 70%);
}

.overview-metric-card--warning.ant-card {
  background: linear-gradient(155deg, rgba(255, 245, 224, 0.9) 0%, rgba(255, 255, 255, 0.95) 60%);
}
.overview-metric-card--warning.ant-card::before {
  background: linear-gradient(180deg, rgba(245, 158, 11, 0.55), rgba(245, 158, 11, 0.12));
}
.overview-metric-card--warning.ant-card::after {
  background: radial-gradient(circle, rgba(245, 158, 11, 0.18) 0%, rgba(245, 158, 11, 0) 70%);
}

.overview-metric-card :deep(.ant-card-body) {
  display: grid;
  gap: 6px;
  padding: 18px 18px 16px 22px;
  position: relative;
  z-index: 1;
}

.overview-metric-card__title {
  color: var(--text-muted);
  font-size: 12px;
  font-weight: 600;
  letter-spacing: 0.02em;
}

.overview-metric-card__value {
  color: var(--text-main);
  font-size: clamp(1.6rem, 1.8vw, 2rem);
  font-weight: 700;
  line-height: 1.1;
  font-feature-settings: 'tnum';
}

.overview-metric-card__value :deep(.ant-statistic-content) {
  color: inherit;
  font-size: inherit;
  line-height: inherit;
}

.overview-metric-card__caption {
  color: var(--text-faint);
  font-size: 12px;
}

.overview-list-card .stack-list {
  display: grid;
  gap: 8px;
}

.overview-list-card :deep(.ant-card-head) {
  padding: 0 18px;
}

.overview-list-card :deep(.ant-card-body) {
  padding: 6px 6px 12px;
}

.overview-list-scroll {
  padding: 4px 8px 8px !important;
}

.overview-feed {
  list-style: none;
  margin: 0;
  padding: 0;
  display: grid;
  gap: 0;
}

.overview-feed__row {
  position: relative;
  display: grid;
  grid-template-columns: 14px minmax(0, 1fr) auto;
  align-items: center;
  gap: 12px;
  padding: 12px 12px 12px 6px;
  border-radius: var(--radius-control);
  transition: background 0.15s ease;
}

.overview-feed__row + .overview-feed__row {
  border-top: 1px solid var(--border-muted);
}

.overview-feed__row:hover {
  background: linear-gradient(180deg, rgba(79, 124, 255, 0.06), rgba(79, 124, 255, 0));
}

.overview-feed__dot {
  width: 8px;
  height: 8px;
  border-radius: 50%;
  background: var(--text-faint);
  box-shadow: 0 0 0 3px rgba(133, 147, 170, 0.15);
  margin-left: 3px;
}

.overview-feed__row--success .overview-feed__dot {
  background: var(--success);
  box-shadow: 0 0 0 3px rgba(52, 168, 83, 0.18);
}

.overview-feed__row--failed .overview-feed__dot {
  background: var(--danger);
  box-shadow: 0 0 0 3px rgba(227, 91, 91, 0.2);
}

.overview-feed__row--attention .overview-feed__dot {
  background: var(--warning);
  box-shadow: 0 0 0 3px rgba(245, 158, 11, 0.22);
}

.overview-feed__row--attention:hover {
  background: linear-gradient(180deg, rgba(245, 158, 11, 0.08), rgba(245, 158, 11, 0));
}

.overview-feed__main {
  min-width: 0;
  display: grid;
  gap: 2px;
}

.overview-feed__title {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 8px;
  min-width: 0;
}

.overview-feed__title strong {
  font-size: 13px;
  font-weight: 600;
  color: var(--text-main);
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  max-width: 100%;
}

.overview-feed__text {
  margin: 0;
  font-size: 12px;
  color: var(--text-muted);
  line-height: 1.5;
  display: -webkit-box;
  -webkit-line-clamp: 2;
  -webkit-box-orient: vertical;
  overflow: hidden;
  word-break: break-word;
}

.overview-feed__time {
  font-size: 11px;
  color: var(--text-faint);
  font-feature-settings: 'tnum';
  white-space: nowrap;
  padding: 4px 8px;
  border-radius: var(--radius-pill);
  background: var(--bg-muted);
}

.overview-feed__row:hover .overview-feed__time {
  background: rgba(79, 124, 255, 0.08);
  color: var(--accent-strong);
}

.overview-feed__row--attention:hover .overview-feed__time {
  background: rgba(245, 158, 11, 0.14);
  color: #b86b00;
}
</style>
