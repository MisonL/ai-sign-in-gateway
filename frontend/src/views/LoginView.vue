<script setup lang="ts">
import { CopyOutlined, LinkOutlined, LockOutlined, UserOutlined } from '@ant-design/icons-vue'
import { onMounted, ref } from 'vue'
import { useRouter } from 'vue-router'
import { getPublicInvites, login } from '../api'
import { useToast } from '../toast'
import type { PublicInvite } from '../types'

const router = useRouter()
const toast = useToast()
const username = ref('admin')
const password = ref('')
const loading = ref(false)
const invites = ref<PublicInvite[]>([])
const invitesLoading = ref(false)

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
    <div class="login-hero">
      <div class="login-hero__intro">
        <span class="login-kicker">ai-sign-in-gateway</span>
        <h1>爱签网关管理后台</h1>
        <p>
          面向 API 中转站的授权管理、签到执行、网关转发、接口连通性验证和运营巡检统一入口。
        </p>
        <ul class="login-hero__points">
          <li>统一管理多家中转站点的签到与额度</li>
          <li>智能路由 + 流式转发，自动熔断与切换</li>
          <li>实时观测：策略对比、5 分钟趋势、TTFB</li>
        </ul>

        <div class="invite-board">
          <div class="invite-board__head">
            <div>
              <span class="invite-board__kicker">Public Invites</span>
              <h2>邀请码列表</h2>
            </div>
            <a-button size="small" :loading="invitesLoading" @click="loadInvites">刷新</a-button>
          </div>
          <a-spin :spinning="invitesLoading">
            <div v-if="invites.length" class="invite-list">
              <div v-for="invite in invites" :key="invite.site_id" class="invite-item">
                <div class="invite-item__main">
                  <strong>{{ invite.site_name }}</strong>
                  <span>{{ inviteMeta(invite) }}</span>
                  <a :href="invite.invite_link || invite.base_url" target="_blank" rel="noreferrer">
                    <LinkOutlined />
                    {{ invite.invite_link || invite.base_url }}
                  </a>
                </div>
                <div class="invite-item__actions">
                  <a-tag v-if="invite.invite_code" color="processing">{{ invite.invite_code }}</a-tag>
                  <a-button size="small" :disabled="!invite.invite_link" @click="copyText(invite.invite_link, '邀请链接')">
                    <template #icon><CopyOutlined /></template>
                    链接
                  </a-button>
                  <a-button size="small" :disabled="!invite.invite_code" @click="copyText(invite.invite_code, '邀请码')">
                    <template #icon><CopyOutlined /></template>
                    码
                  </a-button>
                  <a-button size="small" type="primary" ghost @click="copyInviteBundle(invite)">复制</a-button>
                </div>
              </div>
            </div>
            <a-empty v-else description="暂无公开邀请码" />
          </a-spin>
        </div>
      </div>

      <a-card :bordered="false" class="login-card">
        <template #title>管理员登录</template>

        <a-form layout="vertical" @submit.prevent="submit">
          <a-form-item label="管理员账号">
            <a-input v-model:value="username" autocomplete="username">
              <template #prefix>
                <UserOutlined />
              </template>
            </a-input>
          </a-form-item>
          <a-form-item label="管理员密码">
            <a-input-password v-model:value="password" autocomplete="current-password">
              <template #prefix>
                <LockOutlined />
              </template>
            </a-input-password>
          </a-form-item>
          <a-button block type="primary" html-type="submit" :loading="loading">
            进入后台
          </a-button>
        </a-form>

        <div class="login-note">
          <p>初次使用默认 admin / admin123，登录后可在「设置 → 账号与密码」修改。</p>
        </div>
      </a-card>
    </div>
  </div>
</template>

<style scoped>
.login-screen {
  position: relative;
  min-height: 100vh;
  background:
    radial-gradient(1200px 600px at 10% 0%, rgba(79, 124, 255, 0.18), transparent 60%),
    radial-gradient(900px 500px at 100% 100%, rgba(139, 145, 255, 0.18), transparent 65%),
    linear-gradient(180deg, #f5f7fb 0%, #eef2f9 100%);
  display: grid;
  place-items: center;
  padding: 32px;
}

.login-hero {
  display: grid;
  grid-template-columns: minmax(0, 1.05fr) minmax(320px, 0.95fr);
  gap: 48px;
  width: min(1080px, 100%);
  align-items: center;
}

.login-hero__intro {
  color: var(--text-main);
}

.login-kicker {
  display: inline-block;
  background: var(--accent-soft);
  color: var(--accent-strong);
  padding: 4px 12px;
  border-radius: var(--radius-pill);
  font-size: 12px;
  letter-spacing: 0.04em;
  margin-bottom: 14px;
}

.login-hero h1 {
  font-size: clamp(24px, 2.2vw, 32px);
  margin-bottom: 12px;
  font-weight: 700;
  letter-spacing: -0.01em;
}

.login-hero p {
  color: var(--text-muted);
  font-size: 14px;
  line-height: 1.6;
  max-width: 540px;
}

.login-hero__points {
  margin: 18px 0 0;
  padding: 0;
  list-style: none;
  display: grid;
  gap: 8px;
}

.login-hero__points li {
  position: relative;
  padding-left: 20px;
  color: var(--text-muted);
  font-size: 13px;
}

.login-hero__points li::before {
  content: '';
  position: absolute;
  left: 0;
  top: 8px;
  width: 8px;
  height: 8px;
  border-radius: 50%;
  background: var(--accent);
  box-shadow: 0 0 0 4px rgba(79, 124, 255, 0.16);
}

.invite-board {
  margin-top: 22px;
  padding: 14px;
  border: 1px solid rgba(207, 217, 231, 0.9);
  border-radius: var(--radius-container);
  background: rgba(255, 255, 255, 0.72);
  box-shadow: 0 10px 28px rgba(31, 42, 68, 0.08);
  backdrop-filter: blur(10px);
}

.invite-board__head {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
  margin-bottom: 12px;
}

.invite-board__kicker {
  color: var(--accent-strong);
  font-size: 11px;
  font-weight: 700;
  letter-spacing: 0.08em;
  text-transform: uppercase;
}

.invite-board h2 {
  margin: 2px 0 0;
  font-size: 16px;
  line-height: 1.2;
}

.invite-list {
  display: grid;
  gap: 10px;
  max-height: 310px;
  overflow-y: auto;
  padding-right: 4px;
}

.invite-item {
  display: grid;
  grid-template-columns: minmax(0, 1fr) auto;
  gap: 10px;
  align-items: center;
  padding: 10px;
  border: 1px solid var(--border-muted);
  border-radius: var(--radius-control);
  background: rgba(248, 250, 252, 0.86);
}

.invite-item__main {
  display: grid;
  gap: 3px;
  min-width: 0;
}

.invite-item__main strong {
  color: var(--text-main);
  font-size: 13px;
}

.invite-item__main span,
.invite-item__main a {
  min-width: 0;
  color: var(--text-muted);
  font-size: 12px;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.invite-item__main a {
  display: inline-flex;
  align-items: center;
  gap: 4px;
}

.invite-item__actions {
  display: flex;
  align-items: center;
  gap: 6px;
  flex-wrap: wrap;
  justify-content: flex-end;
  max-width: 250px;
}

.login-card {
  background: rgba(255, 255, 255, 0.92) !important;
  border: 1px solid var(--border-muted) !important;
  box-shadow: var(--shadow-pop) !important;
  backdrop-filter: blur(12px);
}

.login-note {
  margin-top: 12px;
  padding: 10px 12px;
  background: var(--bg-muted);
  border-radius: var(--radius-control);
  color: var(--text-muted);
  font-size: 12px;
}

.login-note p {
  margin: 0;
}

@media (max-width: 880px) {
  .login-hero {
    grid-template-columns: 1fr;
    gap: 24px;
  }

  .invite-item {
    grid-template-columns: 1fr;
  }

  .invite-item__actions {
    justify-content: flex-start;
    max-width: none;
  }
}
</style>
