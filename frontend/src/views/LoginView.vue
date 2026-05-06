<script setup lang="ts">
import {
  ApiOutlined,
  BarChartOutlined,
  CheckCircleOutlined,
  CopyOutlined,
  DashboardOutlined,
  ExportOutlined,
  LinkOutlined,
  LockOutlined,
  LoginOutlined,
  NodeIndexOutlined,
  ReloadOutlined,
  SafetyCertificateOutlined,
  UserOutlined,
} from '@ant-design/icons-vue'
import { onMounted, ref } from 'vue'
import { useRouter } from 'vue-router'
import { getPublicInvites, login } from '../api'
import { useToast } from '../toast'
import type { PublicInvite } from '../types'

const router = useRouter()
const toast = useToast()
const rememberedUsername = localStorage.getItem('ai-sign-in-gateway-login-name') ?? ''
const username = ref(rememberedUsername || 'admin')
const password = ref('')
const rememberMe = ref(Boolean(rememberedUsername))
const loading = ref(false)
const invites = ref<PublicInvite[]>([])
const invitesLoading = ref(false)
const inviteModalOpen = ref(false)

const capabilityCards = [
  {
    icon: ApiOutlined,
    title: '统一管理多家中转站点的签到与额度',
    description: '集中维护授权信息、签到任务、额度状态与账号可用性，减少重复操作和人工巡检成本。',
  },
  {
    icon: NodeIndexOutlined,
    title: '智能路由 + 流式转发，自动熔断与切换',
    description: '支持策略分流、故障隔离与自动切换，提升上游可用性与请求转发稳定性。',
  },
  {
    icon: BarChartOutlined,
    title: '实时观测：策略对比、5 分钟趋势、TTFB',
    description: '以分钟级趋势和响应指标快速定位异常，帮助你持续优化策略、延迟与成功率表现。',
  },
]

const metricCards = [
  {
    icon: ApiOutlined,
    label: '在线站点数',
    value: '28',
    unit: '个',
    trend: '+3',
    caption: '较昨日 25 个',
    waveform: 'M0 48 L24 38 L48 44 L72 30 L96 42 L120 34 L144 21 L168 35 L192 25 L216 29',
  },
  {
    icon: CheckCircleOutlined,
    label: '今日签到成功率',
    value: '98.62',
    unit: '%',
    trend: '+1.24%',
    caption: '成功 1,285 / 1,303',
    progress: 92,
  },
  {
    icon: DashboardOutlined,
    label: '平均 TTFB',
    value: '312',
    unit: 'ms',
    trend: '-28ms',
    caption: '较昨日 340 ms',
    waveform: 'M0 44 L20 32 L40 39 L60 24 L80 41 L100 29 L120 18 L140 38 L160 27 L180 36 L200 26 L220 40',
  },
]

async function loadInvites() {
  invitesLoading.value = true
  try {
    invites.value = await getPublicInvites()
  } catch (err) {
    toast.error(err instanceof Error ? err.message : '邀请码加载失败')
  } finally {
    invitesLoading.value = false
  }
}

async function submit() {
  loading.value = true
  try {
    await login(username.value, password.value)
    if (rememberMe.value) {
      localStorage.setItem('ai-sign-in-gateway-login-name', username.value.trim())
    } else {
      localStorage.removeItem('ai-sign-in-gateway-login-name')
    }
    toast.success('登录成功。')
    router.push('/overview')
  } catch (err) {
    toast.error(err instanceof Error ? err.message : '登录失败')
  } finally {
    loading.value = false
  }
}

async function copyText(value: string, label: string) {
  const normalized = value.trim()
  if (!normalized) {
    toast.error(`${label}为空。`)
    return
  }
  try {
    await navigator.clipboard.writeText(normalized)
    toast.success(`${label}已复制。`)
  } catch {
    toast.error('复制失败，请手动复制。')
  }
}

function copyInviteBundle(invite: PublicInvite) {
  const parts = [
    invite.invite_link ? `邀请链接：${invite.invite_link}` : '',
    invite.invite_code ? `邀请码：${invite.invite_code}` : '',
  ].filter(Boolean)
  void copyText(parts.join('\n'), '邀请信息')
}

function inviteUrl(invite: PublicInvite) {
  return String(invite.invite_link || invite.base_url || '').trim()
}

async function openInviteModal() {
  inviteModalOpen.value = true
  if (!invites.value.length) {
    await loadInvites()
  }
}

function openInviteURL(invite: PublicInvite) {
  const url = inviteUrl(invite)
  if (!url) {
    toast.error('邀请链接为空。')
    return
  }
  window.open(url, '_blank', 'noopener,noreferrer')
}

function inviteMeta(invite: PublicInvite) {
  return [invite.package_name, invite.group_name]
    .map((item) => String(item ?? '').trim())
    .filter(Boolean)
    .join(' / ') || '公开邀请'
}

onMounted(loadInvites)
</script>

<template>
  <div class="login-screen">
    <div class="login-page">
      <header class="login-header">
        <div class="brand-lockup">
          <span class="brand-badge">爱签</span>
          <strong>爱签<span>网关</span></strong>
          <em>运维控制台</em>
        </div>
        <div class="security-line">
          <SafetyCertificateOutlined />
          <span>安全 · 稳定 · 高效 · 可观测</span>
        </div>
      </header>

      <main class="login-main">
        <section class="login-story" aria-label="产品能力">
          <div class="login-copy">
            <h1>爱签网关管理后台</h1>
            <p>面向 API 中转站的授权管理、自动签到、网关转发、接口连通性验证和运营巡检统一入口。</p>
          </div>

          <div class="capability-list">
            <article v-for="item in capabilityCards" :key="item.title" class="capability-item">
              <span class="capability-icon">
                <component :is="item.icon" />
              </span>
              <div>
                <h2>{{ item.title }}</h2>
                <p>{{ item.description }}</p>
              </div>
            </article>
          </div>

          <div class="gateway-visual" aria-hidden="true">
            <span class="route-label route-label--auth">权限管理</span>
            <span class="route-label route-label--checkin">签到执行</span>
            <span class="route-label route-label--stream">流式转发</span>
            <span class="route-label route-label--health">连通性检测</span>
            <div class="gateway-node gateway-node--top"></div>
            <div class="gateway-node gateway-node--left"></div>
            <div class="gateway-node gateway-node--right"></div>
            <div class="gateway-core">
              <span class="gateway-core__base"></span>
              <span class="gateway-core__shield">爱签</span>
            </div>
          </div>

          <div class="metric-panel">
            <article v-for="item in metricCards" :key="item.label" class="metric-card">
              <div class="metric-head">
                <span class="metric-icon">
                  <component :is="item.icon" />
                </span>
                <strong>{{ item.label }}</strong>
              </div>
              <div class="metric-value">
                <span>{{ item.value }}</span>
                <small>{{ item.unit }}</small>
                <em>{{ item.trend }}</em>
              </div>
              <div v-if="item.progress" class="metric-progress">
                <span :style="{ width: `${item.progress}%` }"></span>
              </div>
              <svg v-else viewBox="0 0 220 60" class="metric-wave" aria-hidden="true">
                <path :d="item.waveform" />
              </svg>
              <p>{{ item.caption }}</p>
            </article>

            <article class="metric-card metric-card--score">
              <div class="metric-head">
                <span class="metric-icon">
                  <SafetyCertificateOutlined />
                </span>
                <strong>路由健康度</strong>
              </div>
              <div class="score-ring">
                <span>96</span>
                <small>分</small>
              </div>
              <p><CheckCircleOutlined /> 状态良好</p>
            </article>
          </div>
        </section>

        <div class="login-side">
          <section class="login-card" aria-label="管理员登录">
            <div class="login-card__glow"></div>
            <div class="login-card__head">
              <span class="brand-badge brand-badge--large">爱签</span>
              <div>
                <h2>欢迎登录</h2>
                <p>登录后进入爱签网关管理后台，查看站点状态、路由策略与连通性巡检结果。</p>
              </div>
            </div>

            <a-form class="login-form" layout="vertical" @submit.prevent="submit">
              <a-form-item label="账号" html-for="login-username">
                <a-input
                  id="login-username"
                  v-model:value="username"
                  name="username"
                  size="large"
                  autocomplete="username"
                  placeholder="请输入邮箱 / 用户名"
                >
                  <template #prefix>
                    <UserOutlined />
                  </template>
                </a-input>
              </a-form-item>

              <a-form-item label="密码" html-for="login-password">
                <a-input-password
                  id="login-password"
                  v-model:value="password"
                  name="password"
                  size="large"
                  autocomplete="current-password"
                  placeholder="请输入登录密码"
                >
                  <template #prefix>
                    <LockOutlined />
                  </template>
                </a-input-password>
              </a-form-item>

              <div class="login-options">
                <a-checkbox v-model:checked="rememberMe">记住我</a-checkbox>
              </div>

              <a-button class="login-submit" block type="primary" html-type="submit" size="large" :loading="loading">
                <template #icon><LoginOutlined /></template>
                登录后台
              </a-button>
            </a-form>
          </section>

          <button class="invite-entry" type="button" @click="openInviteModal">
            <span>
              <strong>公开邀请入口</strong>
              <small>查看可注册站点与邀请链接</small>
            </span>
            <LinkOutlined />
          </button>
        </div>
      </main>

      <footer class="login-footer">
        <span>爱签网关 · 统一授权 · 策略路由 · 健康巡检 · 数据观测</span>
        <span>© 2025 爱签网关 版权所有</span>
        <span>版本 v2.1.0</span>
      </footer>
    </div>

    <a-modal
      v-model:open="inviteModalOpen"
      class="invite-modal"
      width="min(920px, calc(100vw - 32px))"
      title="公开邀请站点"
      :footer="null"
    >
      <div class="invite-modal__body">
        <div class="invite-modal__toolbar">
          <span>{{ invites.length ? `共 ${invites.length} 个公开邀请站点` : '暂无公开邀请站点' }}</span>
          <a-button size="small" :loading="invitesLoading" @click="loadInvites">
            <template #icon><ReloadOutlined /></template>
            刷新
          </a-button>
        </div>

        <a-spin :spinning="invitesLoading">
          <div v-if="invites.length" class="invite-list">
            <div v-for="invite in invites" :key="invite.site_id" class="invite-item">
              <div class="invite-item__main">
                <strong>{{ invite.site_name }}</strong>
                <span>{{ inviteMeta(invite) }}</span>
                <a :href="inviteUrl(invite)" target="_blank" rel="noreferrer">
                  <LinkOutlined />
                  {{ inviteUrl(invite) }}
                </a>
              </div>
              <div class="invite-item__actions">
                <a-tag v-if="invite.invite_code" color="processing">{{ invite.invite_code }}</a-tag>
                <a-button size="small" :disabled="!inviteUrl(invite)" @click="copyText(inviteUrl(invite), '邀请链接')">
                  <template #icon><CopyOutlined /></template>
                  链接
                </a-button>
                <a-button
                  size="small"
                  :disabled="!invite.invite_code && !inviteUrl(invite)"
                  @click="copyInviteBundle(invite)"
                >
                  <template #icon><CopyOutlined /></template>
                  信息
                </a-button>
                <a-button size="small" type="primary" :disabled="!inviteUrl(invite)" @click="openInviteURL(invite)">
                  <template #icon><ExportOutlined /></template>
                  打开
                </a-button>
              </div>
            </div>
          </div>
          <a-empty v-else description="暂无公开邀请码" />
        </a-spin>
      </div>
    </a-modal>
  </div>
</template>

<style scoped>
.login-screen {
  position: relative;
  height: 100vh;
  min-height: 100vh;
  overflow: hidden;
  color: #102654;
  background:
    linear-gradient(116deg, rgba(255, 255, 255, 0.96) 0%, rgba(226, 244, 255, 0.86) 42%, rgba(89, 181, 255, 0.86) 100%),
    #d9efff;
}

.login-screen::before,
.login-screen::after {
  position: absolute;
  inset: 0;
  pointer-events: none;
  content: '';
}

.login-screen::before {
  background:
    linear-gradient(130deg, transparent 0 45%, rgba(255, 255, 255, 0.34) 45.2% 45.8%, transparent 46%),
    linear-gradient(150deg, transparent 0 61%, rgba(28, 128, 255, 0.22) 61.2% 61.7%, transparent 62%),
    radial-gradient(circle at 44% 12%, rgba(255, 255, 255, 0.78) 0 2px, transparent 3px),
    radial-gradient(circle at 68% 7%, rgba(255, 255, 255, 0.7) 0 1px, transparent 2px);
  opacity: 0.85;
}

.login-screen::after {
  top: auto;
  height: 34vh;
  background:
    linear-gradient(10deg, rgba(34, 142, 255, 0.46), rgba(255, 255, 255, 0) 58%),
    linear-gradient(158deg, transparent 0 34%, rgba(255, 255, 255, 0.64) 34.2% 35.1%, transparent 35.3%),
    linear-gradient(172deg, transparent 0 46%, rgba(28, 128, 255, 0.42) 46.2% 47.1%, transparent 47.3%);
}

.login-page {
  position: relative;
  z-index: 1;
  display: grid;
  grid-template-rows: auto minmax(0, 1fr) auto;
  height: 100vh;
  padding: 24px 42px 14px;
  overflow: hidden;
}

.login-header,
.login-footer,
.login-main,
.brand-lockup,
.security-line,
.capability-item,
.metric-head,
.metric-value,
.login-card__head,
.login-options,
.invite-item__actions,
.login-side {
  display: flex;
}

.login-header,
.login-footer {
  align-items: center;
  justify-content: space-between;
  gap: 18px;
}

.brand-lockup {
  align-items: center;
  gap: 10px;
}

.brand-badge {
  position: relative;
  display: grid;
  place-items: center;
  width: 46px;
  height: 52px;
  color: #ffffff;
  font-size: 14px;
  font-weight: 800;
  line-height: 1;
  text-shadow: 0 2px 8px rgba(9, 55, 129, 0.32);
  background: linear-gradient(142deg, #1b77ff 0%, #1e92ff 52%, #1054df 100%);
  clip-path: polygon(50% 0, 94% 24%, 94% 76%, 50% 100%, 6% 76%, 6% 24%);
  box-shadow: 0 14px 28px rgba(18, 101, 227, 0.28);
}

.brand-badge--large {
  width: 70px;
  height: 80px;
  flex: 0 0 auto;
  font-size: 22px;
}

.brand-lockup strong {
  color: #071b49;
  font-size: 31px;
  font-weight: 800;
  line-height: 1;
}

.brand-lockup strong span,
.brand-lockup em {
  color: #1377ff;
}

.brand-lockup em {
  padding: 6px 14px;
  border-radius: 999px;
  background: rgba(30, 127, 255, 0.14);
  font-size: 14px;
  font-style: normal;
  font-weight: 700;
}

.security-line {
  align-items: center;
  gap: 12px;
  color: #4070ad;
  font-size: 15px;
  font-weight: 600;
}

.login-main {
  align-items: center;
  justify-content: space-between;
  gap: 34px;
  width: min(1508px, 100%);
  min-height: 0;
  margin: 0 auto;
}

.login-story {
  position: relative;
  display: grid;
  grid-template-columns: minmax(500px, 0.78fr) minmax(360px, 0.72fr);
  grid-template-rows: auto 1fr auto;
  column-gap: 26px;
  row-gap: 14px;
  flex: 1 1 auto;
  min-width: 0;
  min-height: 0;
}

.login-copy {
  grid-column: 1;
}

.login-copy h1 {
  margin: 0 0 12px;
  color: #06194a;
  font-size: 46px;
  font-weight: 900;
  line-height: 1.08;
  text-shadow: 0 10px 28px rgba(70, 139, 219, 0.18);
}

.login-copy p {
  max-width: 560px;
  margin: 0;
  color: #244676;
  font-size: 17px;
  font-weight: 600;
  line-height: 1.75;
}

.capability-list {
  z-index: 2;
  grid-column: 1;
  display: grid;
  gap: 16px;
  align-self: center;
  max-width: 640px;
}

.capability-item {
  align-items: flex-start;
  gap: 20px;
}

.capability-icon {
  display: grid;
  flex: 0 0 auto;
  place-items: center;
  width: 50px;
  height: 50px;
  border-radius: 14px;
  color: #ffffff;
  font-size: 28px;
  background: linear-gradient(150deg, #2d88ff, #1268f4);
  box-shadow: 0 12px 22px rgba(15, 111, 245, 0.32), inset 0 1px 0 rgba(255, 255, 255, 0.45);
}

.capability-item h2 {
  margin: 0 0 6px;
  color: #071d50;
  font-size: 18px;
  font-weight: 800;
  line-height: 1.3;
}

.capability-item p {
  max-width: 520px;
  margin: 0;
  color: #355781;
  font-size: 14px;
  font-weight: 600;
  line-height: 1.55;
}

.gateway-visual {
  position: relative;
  grid-column: 2;
  grid-row: 1 / span 2;
  align-self: center;
  min-height: 372px;
}

.gateway-visual::before {
  position: absolute;
  inset: 17% 5% 6% 4%;
  content: '';
  background:
    linear-gradient(90deg, rgba(46, 164, 255, 0.24) 1px, transparent 1px),
    linear-gradient(0deg, rgba(46, 164, 255, 0.24) 1px, transparent 1px);
  background-size: 72px 72px;
  border-radius: 44px;
  transform: perspective(640px) rotateX(58deg) rotateZ(-34deg);
  filter: drop-shadow(0 28px 24px rgba(42, 126, 224, 0.2));
  opacity: 0.9;
}

.gateway-core,
.gateway-core__base,
.gateway-core__shield,
.gateway-node {
  position: absolute;
  user-select: none;
  pointer-events: none;
}

.gateway-core {
  left: 10%;
  top: 54px;
  width: min(72%, 470px);
  height: 300px;
  filter: drop-shadow(0 30px 36px rgba(18, 105, 224, 0.28));
}

.gateway-core__base {
  left: 50%;
  bottom: 16px;
  width: 250px;
  height: 168px;
  border: 1px solid rgba(159, 215, 255, 0.82);
  border-radius: 34px;
  background:
    linear-gradient(180deg, rgba(255, 255, 255, 0.9), rgba(177, 220, 255, 0.72) 42%, rgba(43, 134, 255, 0.9) 100%);
  box-shadow:
    inset 0 3px 0 rgba(255, 255, 255, 0.84),
    inset 0 -16px 28px rgba(4, 85, 217, 0.32),
    0 32px 48px rgba(28, 111, 224, 0.24);
  transform: translateX(-50%) perspective(620px) rotateX(62deg) rotateZ(-45deg);
}

.gateway-core__base::before,
.gateway-core__base::after {
  position: absolute;
  content: '';
}

.gateway-core__base::before {
  inset: 24px 26px;
  border-radius: 22px;
  background: linear-gradient(145deg, #74c4ff, #096cff);
  box-shadow: inset 0 2px 0 rgba(255, 255, 255, 0.55), 0 12px 20px rgba(8, 99, 220, 0.24);
}

.gateway-core__base::after {
  left: 42px;
  right: 42px;
  bottom: 22px;
  height: 7px;
  border-radius: 999px;
  background: linear-gradient(90deg, transparent, rgba(125, 225, 255, 0.9), transparent);
}

.gateway-core__shield {
  left: 50%;
  top: 0;
  display: grid;
  place-items: center;
  width: 130px;
  height: 146px;
  color: #ffffff;
  font-size: 34px;
  font-weight: 900;
  background: linear-gradient(150deg, #54b8ff 0%, #1479ff 54%, #0758dc 100%);
  clip-path: polygon(50% 0, 92% 22%, 92% 62%, 50% 100%, 8% 62%, 8% 22%);
  box-shadow:
    inset 0 5px 0 rgba(255, 255, 255, 0.54),
    inset -12px -16px 20px rgba(6, 67, 181, 0.28),
    0 20px 34px rgba(12, 107, 232, 0.3);
  transform: translateX(-50%);
}

.gateway-core__shield::after {
  position: absolute;
  inset: 10px;
  border: 2px solid rgba(255, 255, 255, 0.72);
  clip-path: inherit;
  content: '';
}

.gateway-node {
  width: 92px;
  height: 74px;
  border: 1px solid rgba(157, 217, 255, 0.84);
  border-radius: 18px;
  background:
    linear-gradient(180deg, rgba(255, 255, 255, 0.86), rgba(155, 216, 255, 0.72) 42%, rgba(29, 122, 245, 0.9));
  box-shadow: 0 18px 26px rgba(35, 123, 224, 0.18), inset 0 2px 0 rgba(255, 255, 255, 0.76);
  transform: perspective(420px) rotateX(58deg) rotateZ(-45deg);
}

.gateway-node::before,
.gateway-node::after {
  position: absolute;
  left: 22px;
  right: 22px;
  height: 5px;
  border-radius: 999px;
  background: rgba(137, 229, 255, 0.82);
  content: '';
}

.gateway-node::before {
  top: 22px;
}

.gateway-node::after {
  bottom: 20px;
}

.gateway-node--top {
  right: 8%;
  top: 72px;
}

.gateway-node--left {
  left: 4%;
  bottom: 64px;
}

.gateway-node--right {
  right: 6%;
  bottom: 76px;
}

.route-label {
  position: absolute;
  z-index: 3;
  padding: 5px 10px;
  border: 1px solid rgba(82, 169, 255, 0.36);
  border-radius: 8px;
  color: #1675f7;
  background: rgba(255, 255, 255, 0.86);
  box-shadow: 0 10px 18px rgba(31, 128, 240, 0.13);
  font-size: 14px;
  font-weight: 800;
  transform: rotate(-8deg);
}

.route-label--auth {
  left: 0;
  top: 51%;
}

.route-label--checkin {
  left: 9%;
  bottom: 14%;
}

.route-label--stream {
  right: 1%;
  top: 32%;
}

.route-label--health {
  right: 8%;
  bottom: 12%;
}

.metric-panel {
  grid-column: 1 / -1;
  display: grid;
  grid-template-columns: repeat(4, minmax(0, 1fr));
  gap: 12px;
  width: min(928px, 100%);
  padding: 12px;
  border: 1px solid rgba(255, 255, 255, 0.86);
  border-radius: 22px;
  background: rgba(255, 255, 255, 0.54);
  box-shadow: 0 18px 50px rgba(33, 119, 226, 0.16), inset 0 1px 0 rgba(255, 255, 255, 0.72);
  backdrop-filter: blur(18px);
}

.metric-card {
  min-width: 0;
  min-height: 154px;
  padding: 13px;
  border: 1px solid rgba(185, 217, 255, 0.7);
  border-radius: 14px;
  background: linear-gradient(156deg, rgba(255, 255, 255, 0.9), rgba(230, 245, 255, 0.72));
  box-shadow: inset 0 1px 0 rgba(255, 255, 255, 0.82);
}

.metric-head {
  align-items: center;
  gap: 9px;
  color: #102c61;
}

.metric-icon {
  display: grid;
  place-items: center;
  width: 30px;
  height: 30px;
  border-radius: 10px;
  color: #1979ff;
  background: rgba(37, 136, 255, 0.13);
}

.metric-head strong {
  overflow: hidden;
  font-size: 14px;
  font-weight: 800;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.metric-value {
  align-items: baseline;
  gap: 5px;
  margin-top: 14px;
}

.metric-value span {
  color: #07194a;
  font-size: 30px;
  font-weight: 900;
  line-height: 1;
}

.metric-value small {
  color: #173768;
  font-size: 14px;
  font-weight: 800;
}

.metric-value em {
  margin-left: auto;
  padding: 3px 9px;
  border-radius: 999px;
  color: #0aa76c;
  background: rgba(30, 211, 138, 0.16);
  font-size: 13px;
  font-style: normal;
  font-weight: 800;
}

.metric-wave {
  width: 100%;
  height: 34px;
  margin-top: 8px;
  overflow: visible;
}

.metric-wave path {
  fill: none;
  stroke: #1a7cff;
  stroke-linecap: round;
  stroke-linejoin: round;
  stroke-width: 3;
}

.metric-progress {
  height: 10px;
  margin-top: 24px;
  overflow: hidden;
  border-radius: 999px;
  background: rgba(117, 183, 255, 0.22);
}

.metric-progress span {
  display: block;
  height: 100%;
  border-radius: inherit;
  background: linear-gradient(90deg, #116dff, #37a7ff);
  box-shadow: 0 5px 15px rgba(19, 122, 255, 0.28);
}

.metric-card p {
  margin: 8px 0 0;
  color: #506f98;
  font-size: 14px;
  font-weight: 600;
}

.metric-card--score {
  text-align: center;
}

.score-ring {
  display: grid;
  place-items: center;
  width: 72px;
  height: 72px;
  margin: 10px auto 4px;
  border-radius: 50%;
  background:
    radial-gradient(circle at center, rgba(255, 255, 255, 0.96) 0 58%, transparent 59%),
    conic-gradient(#1780ff 0 96%, rgba(177, 215, 255, 0.5) 96% 100%);
  box-shadow: 0 10px 20px rgba(20, 113, 245, 0.18);
}

.score-ring span {
  color: #07194a;
  font-size: 30px;
  font-weight: 900;
  line-height: 1;
}

.score-ring small {
  color: #385c89;
  font-weight: 700;
}

.metric-card--score p {
  display: inline-flex;
  align-items: center;
  gap: 4px;
  color: #0a9c64;
}

.login-side {
  flex: 0 0 min(520px, 36vw);
  flex-direction: column;
  gap: 14px;
  min-width: 420px;
  max-height: 100%;
  min-height: 0;
}

.login-card {
  position: relative;
  overflow: hidden;
  padding: 36px 44px 34px;
  border: 1px solid rgba(255, 255, 255, 0.88);
  border-radius: 28px;
  background: linear-gradient(152deg, rgba(255, 255, 255, 0.92), rgba(238, 248, 255, 0.74));
  box-shadow: 0 28px 70px rgba(34, 101, 193, 0.22), inset 0 1px 0 rgba(255, 255, 255, 0.92);
  backdrop-filter: blur(22px);
}

.login-card::before {
  position: absolute;
  top: -64px;
  right: -56px;
  width: 210px;
  height: 164px;
  border-radius: 0 0 0 88px;
  background: rgba(211, 235, 255, 0.78);
  box-shadow: -38px 36px 0 rgba(229, 244, 255, 0.88);
  content: '';
}

.login-card__glow {
  position: absolute;
  inset: auto -20% -18% 4%;
  height: 170px;
  background: radial-gradient(circle at 50% 0%, rgba(44, 154, 255, 0.24), transparent 62%);
  pointer-events: none;
}

.login-card__head {
  position: relative;
  z-index: 1;
  align-items: flex-start;
  gap: 20px;
  margin-bottom: 18px;
}

.login-card__head h2 {
  margin: 6px 0 8px;
  color: #07194a;
  font-size: 30px;
  font-weight: 900;
  line-height: 1.15;
}

.login-card__head p {
  margin: 0;
  color: #446489;
  font-size: 15px;
  font-weight: 600;
  line-height: 1.6;
}

.login-form {
  position: relative;
  z-index: 1;
}

.login-form :deep(.ant-form-item) {
  margin-bottom: 13px;
}

.login-form :deep(.ant-form-item-label) {
  padding-bottom: 8px !important;
}

.login-form :deep(.ant-form-item-label > label) {
  color: #102755;
  font-size: 15px;
  font-weight: 800;
}

.login-form :deep(.ant-input-affix-wrapper) {
  min-height: 46px;
  border-color: rgba(155, 181, 215, 0.75);
  border-radius: 7px !important;
  background: rgba(255, 255, 255, 0.72);
  box-shadow: inset 0 1px 0 rgba(255, 255, 255, 0.7);
}

.login-form :deep(.ant-input-prefix),
.login-form :deep(.ant-input-suffix) {
  color: #7893b8;
  font-size: 17px;
}

.login-form :deep(.ant-input) {
  color: #14315e;
  font-weight: 600;
  background: transparent;
}

.invite-entry {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 14px;
  width: 100%;
  min-height: 64px;
  padding: 12px 18px;
  border: 1px solid rgba(255, 255, 255, 0.86);
  border-radius: 18px;
  color: #102755;
  background: linear-gradient(152deg, rgba(255, 255, 255, 0.72), rgba(235, 248, 255, 0.52));
  box-shadow: 0 16px 36px rgba(34, 101, 193, 0.14), inset 0 1px 0 rgba(255, 255, 255, 0.82);
  cursor: pointer;
  backdrop-filter: blur(18px);
}

.invite-entry strong,
.invite-entry small {
  display: block;
  text-align: left;
}

.invite-entry strong {
  font-size: 14px;
  font-weight: 900;
}

.invite-entry small {
  margin-top: 3px;
  color: #5d7ba3;
  font-size: 12px;
  font-weight: 700;
}

.invite-entry > .anticon {
  display: grid;
  flex: 0 0 auto;
  place-items: center;
  width: 34px;
  height: 34px;
  border-radius: 12px;
  color: #1979ff;
  background: rgba(37, 136, 255, 0.12);
}

:global(.invite-modal .ant-modal-content) {
  max-height: 80vh;
  overflow: hidden;
  border: 1px solid rgba(207, 229, 255, 0.82);
  border-radius: 18px !important;
  background: linear-gradient(152deg, rgba(255, 255, 255, 0.96), rgba(235, 248, 255, 0.9));
}

:global(.invite-modal .ant-modal-body) {
  padding: 0 20px 20px;
}

.invite-modal__body {
  max-height: calc(80vh - 78px);
  overflow: hidden;
}

.invite-modal__toolbar {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
  margin-bottom: 12px;
  color: #5d7ba3;
  font-size: 13px;
  font-weight: 800;
}

.invite-list {
  display: grid;
  gap: 10px;
  max-height: calc(80vh - 142px);
  overflow-y: auto;
  padding-right: 4px;
}

.invite-item {
  display: grid;
  grid-template-columns: minmax(0, 1fr) auto;
  gap: 10px;
  align-items: center;
  padding: 9px 10px;
  border: 1px solid rgba(204, 223, 246, 0.78);
  border-radius: 8px;
  background: rgba(246, 251, 255, 0.75);
}

.invite-item__main {
  display: grid;
  gap: 2px;
  min-width: 0;
}

.invite-item__main strong {
  overflow: hidden;
  color: #102755;
  font-size: 13px;
  font-weight: 800;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.invite-item__main span,
.invite-item__main a {
  min-width: 0;
  overflow: hidden;
  color: #617da4;
  font-size: 12px;
  font-weight: 600;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.invite-item__main a {
  display: inline-flex;
  align-items: center;
  gap: 4px;
}

.invite-item__actions {
  align-items: center;
  justify-content: flex-end;
  gap: 6px;
}

.login-options {
  align-items: center;
  justify-content: flex-start;
  margin: 2px 0 14px;
  color: #4770a5;
  font-size: 13px;
  font-weight: 700;
}

.login-submit {
  min-height: 48px;
  border: 0;
  border-radius: 7px !important;
  background: linear-gradient(90deg, #0969f2, #21a3ff) !important;
  box-shadow: 0 14px 24px rgba(17, 114, 245, 0.28);
  font-size: 17px;
  font-weight: 900;
}

.login-footer {
  color: #416b9d;
  font-size: 14px;
  font-weight: 600;
}

.login-footer span {
  margin: 0;
  min-width: 0;
}

@media (max-width: 1280px) {
  .login-page {
    padding: 20px;
  }

  .login-main {
    align-items: stretch;
    gap: 20px;
  }

  .login-story {
    grid-template-columns: 1fr;
  }

  .gateway-visual {
    display: none;
  }

  .metric-panel {
    grid-template-columns: repeat(2, minmax(0, 1fr));
    width: 100%;
  }

  .login-copy h1 {
    font-size: 40px;
  }

  .login-side {
    flex-basis: 420px;
    min-width: 360px;
  }
}

@media (max-width: 880px) {
  .login-screen {
    height: auto;
    min-height: 100dvh;
    overflow-x: hidden;
    overflow-y: auto;
  }

  .login-page {
    height: auto;
    min-height: 100dvh;
    padding: 16px 16px calc(16px + env(safe-area-inset-bottom, 0px));
    overflow: visible;
  }

  .login-header,
  .login-footer,
  .login-main {
    flex-direction: column;
    align-items: stretch;
  }

  .security-line,
  .login-footer {
    font-size: 12px;
  }

  .brand-lockup {
    flex-wrap: wrap;
  }

  .brand-lockup strong {
    font-size: 22px;
  }

  .login-main {
    gap: 12px;
    min-height: 0;
    margin-top: 14px;
    overflow: visible;
  }

  .login-copy h1 {
    font-size: 28px;
    line-height: 1.16;
  }

  .login-copy p,
  .capability-item p {
    font-size: 14px;
  }

  .login-story {
    display: none;
  }

  .capability-list,
  .metric-panel {
    margin-top: 24px;
  }

  .metric-panel {
    grid-template-columns: 1fr;
  }

  .metric-card {
    min-height: auto;
  }

  .login-side {
    flex: 1 1 auto;
    gap: 12px;
    min-width: 0;
    overflow: hidden;
  }

  .login-card {
    flex: 0 0 auto;
    width: 100%;
    padding: 22px 20px;
    border-radius: 22px;
  }

  .login-card__head {
    gap: 14px;
  }

  .login-card__head h2 {
    margin-top: 6px;
    font-size: 24px;
  }

  .brand-badge--large {
    width: 48px;
    height: 56px;
    font-size: 16px;
  }

  .invite-entry {
    flex: 0 0 auto;
  }

  .invite-list {
    max-height: calc(80vh - 142px);
  }

  .invite-item {
    grid-template-columns: 1fr;
  }

  .invite-item__actions {
    justify-content: flex-start;
  }

}
</style>
