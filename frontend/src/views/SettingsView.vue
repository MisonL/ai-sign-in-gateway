<script setup lang="ts">
import { CopyOutlined, DatabaseOutlined, DeleteOutlined, DownloadOutlined, FolderOpenOutlined, PlusOutlined, ReloadOutlined, SaveOutlined } from '@ant-design/icons-vue'
import { computed, onMounted, reactive, ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import {
  backupRuntimeDatabaseNow,
  createAdminUser,
  deleteAdminUser,
  deleteRuntimeDatabaseBackup,
  downloadRuntimeConfigArchive,
  downloadRuntimeDatabaseBackup,
  getAdminUsers,
  getMe,
  getRuntimeDatabaseBackups,
  getSettings,
  logout,
  runSchedulerNow,
  setRuntimeConfigDir,
  stopStaleRuntimePorts,
  uploadRuntimeDatabase,
  updateAdminAccount,
  updateAdminUser,
  updateSettings,
} from '../api'
import ShellLayout from '../components/ShellLayout.vue'
import { useToast } from '../toast'
import type { AdminUser, GatewayModelPrice, GatewayPricingScheme, RuntimeDatabaseBackupFile, RuntimeStopPortResult, SettingsData } from '../types'
import '../styles/workspace-surfaces.css'

const toast = useToast()
const route = useRoute()
const router = useRouter()
const form = reactive<SettingsData>({
  timezone: 'Asia/Shanghai',
  schedule_enabled: true,
  daily_run_time: '09:00',
  checkin_concurrency: 1,
  checkin_global_concurrency: 4,
  checkin_interval_seconds: 1,
  retry_count: 1,
  request_timeout: 20,
  only_enabled_sites: true,
  desktop_keep_running: false,
  database_backup_enabled: false,
  database_backup_dir: '',
  database_backup_interval_minutes: 1440,
  database_backup_retention: 7,
  log_retention_days: 5,
  gateway_pricing_active_scheme_id: 'official',
  gateway_pricing_schemes: [],
  feature_flags: {},
  features: [],
  desktop_frontend_default_port: 3721,
  desktop_frontend_port: 0,
  desktop_frontend_url: '',
  desktop_frontend_default_port_occupant: '',
  desktop_backend_default_port: 8972,
  desktop_backend_port: 0,
  desktop_backend_url: '',
  desktop_backend_default_port_occupant: '',
  desktop_gateway_url: '',
  runtime_config_dir: '',
  runtime_default_config_dir: '',
  runtime_database_path: '',
  runtime_pending_config_dir: '',
  security_warnings: [],
})

const loading = ref(false)
const accountLoading = ref(false)
const adminUsersLoading = ref(false)
const adminUserSavingID = ref<number | null>(null)
const adminUserDeletingID = ref<number | null>(null)
const runtimeStopLoading = ref(false)
const configDirLoading = ref(false)
const configArchiveDownloading = ref(false)
const databaseImportLoading = ref(false)
const databaseBackupLoading = ref(false)
const databaseBackupDownloadName = ref('')
const runtimeStopResults = ref<RuntimeStopPortResult[]>([])
const databaseBackups = ref<RuntimeDatabaseBackupFile[]>([])
const databaseBackupDir = ref('')
const runtimeConfigDirInput = ref('')
const runtimeDatabaseFileInput = ref<HTMLInputElement | null>(null)
const currentUsername = ref('')
const currentAdmin = ref<AdminUser | null>(null)
const adminUsers = ref<AdminUser[]>([])
const activeTab = ref<'schedule' | 'runtime' | 'database' | 'pricing' | 'extensions' | 'config' | 'account'>('schedule')
const isDesktopEmbedded = computed(() => route.path === '/desktop')
const settingsFrameComponent = computed(() => (isDesktopEmbedded.value ? 'div' : ShellLayout))
const pricingProviderOptions = [
  { label: 'Codex / OpenAI', value: 'codex' },
  { label: 'Claude', value: 'claude' },
  { label: 'Gemini', value: 'gemini' },
]
const pricingSchemeOptions = computed(() =>
  form.gateway_pricing_schemes.map((scheme) => ({
    label: scheme.readonly ? `${scheme.name}（只读）` : scheme.name,
    value: scheme.id,
  })),
)
const activePricingScheme = computed(() =>
  form.gateway_pricing_schemes.find((scheme) => scheme.id === form.gateway_pricing_active_scheme_id) ?? form.gateway_pricing_schemes[0] ?? null,
)
const activePricingEditable = computed(() => Boolean(activePricingScheme.value && !activePricingScheme.value.readonly))
const canManageAdminUsers = computed(() => currentAdmin.value?.role === 'super_admin')
const roleOptions = [
  { label: '管理员', value: 'admin' },
  { label: '超级管理员', value: 'super_admin' },
]
const accountForm = reactive({
  new_username: '',
  current_password: '',
  new_password: '',
  confirm_password: '',
})
const adminUserCreateForm = reactive({
  username: '',
  password: '',
  role: 'admin',
  is_enabled: true,
})
const adminUserPasswordEdits = reactive<Record<number, string>>({})

function asAdminUser(record: unknown) {
  return record as AdminUser
}

async function loadData() {
  loading.value = true
  try {
    const settings = await getSettings()
    Object.assign(form, settings)
    if (!form.gateway_pricing_active_scheme_id) {
      form.gateway_pricing_active_scheme_id = 'official'
    }
    if (!Array.isArray(form.gateway_pricing_schemes)) {
      form.gateway_pricing_schemes = []
    }
    runtimeConfigDirInput.value = settings.runtime_pending_config_dir || settings.runtime_config_dir || settings.runtime_default_config_dir || ''
    await loadDatabaseBackups(false)
    try {
      const me = await getMe()
      currentAdmin.value = me
      currentUsername.value = me.username
      accountForm.new_username = me.username
      if (me.role === 'super_admin') {
        await loadAdminUsers(false)
      } else {
        adminUsers.value = []
      }
    } catch (err) {
      toast.error(err instanceof Error ? err.message : '获取当前账号失败')
    }
  } catch (err) {
    toast.error(err instanceof Error ? err.message : '加载失败')
  } finally {
    loading.value = false
  }
}

async function loadAdminUsers(showError = true) {
  if (!canManageAdminUsers.value) {
    adminUsers.value = []
    return
  }
  adminUsersLoading.value = true
  try {
    adminUsers.value = await getAdminUsers()
  } catch (err) {
    adminUsers.value = []
    if (showError) {
      toast.error(err instanceof Error ? err.message : '读取管理员列表失败')
    }
  } finally {
    adminUsersLoading.value = false
  }
}

function clonePricingScheme(source: GatewayPricingScheme): GatewayPricingScheme {
  return {
    ...source,
    prices: source.prices.map((price) => ({ ...price })),
  }
}

function duplicateActivePricingScheme() {
  const source = activePricingScheme.value
  if (!source) {
    return
  }
  const next = clonePricingScheme(source)
  next.id = `custom-${Date.now()}`
  next.name = source.readonly ? '官方价格副本' : `${source.name} 副本`
  next.readonly = false
  next.source = 'custom'
  form.gateway_pricing_schemes.push(next)
  form.gateway_pricing_active_scheme_id = next.id
}

function addPricingRow() {
  const scheme = activePricingScheme.value
  if (!scheme || scheme.readonly) {
    return
  }
  scheme.prices.push({
    provider: 'codex',
    model_prefix: '',
    display_name: '',
    input_per_mtok: 0,
    cached_input_per_mtok: 0,
    cache_write_per_mtok: 0,
    output_per_mtok: 0,
  })
}

function removePricingRow(index: number) {
  const scheme = activePricingScheme.value
  if (!scheme || scheme.readonly) {
    return
  }
  scheme.prices.splice(index, 1)
}

function priceRowKey(price: GatewayModelPrice, index: number) {
  return `${price.provider}-${price.model_prefix}-${index}`
}

async function save() {
  loading.value = true
  try {
    Object.assign(form, await updateSettings(form))
    await loadDatabaseBackups(false)
    toast.success('系统设置已保存并重载调度器。')
  } catch (err) {
    toast.error(err instanceof Error ? err.message : '保存失败')
  } finally {
    loading.value = false
  }
}

async function runNow() {
  loading.value = true
  try {
    const result = await runSchedulerNow()
    toast.success(result.message)
  } catch (err) {
    toast.error(err instanceof Error ? err.message : '执行失败')
  } finally {
    loading.value = false
  }
}

async function stopOldPorts() {
  runtimeStopLoading.value = true
  try {
    const result = await stopStaleRuntimePorts()
    runtimeStopResults.value = result.results
    const stoppedCount = result.results.filter((item) => item.stopped).length
    if (stoppedCount > 0) {
      toast.success(`已停止 ${stoppedCount} 个旧版本端口占用。`)
    } else {
      toast.info('没有可停止的旧版本端口占用。')
    }
    await loadData()
  } catch (err) {
    toast.error(err instanceof Error ? err.message : '停止旧版本端口失败')
  } finally {
    runtimeStopLoading.value = false
  }
}

async function loadRuntimeConfigDir() {
  const configDir = runtimeConfigDirInput.value.trim()
  if (!configDir) {
    toast.error('请填写配置目录路径。')
    return
  }
  configDirLoading.value = true
  try {
    const result = await setRuntimeConfigDir(configDir)
    form.runtime_pending_config_dir = result.config_dir
    runtimeConfigDirInput.value = result.config_dir
    toast.success(result.message)
  } catch (err) {
    toast.error(err instanceof Error ? err.message : '保存配置目录失败')
  } finally {
    configDirLoading.value = false
  }
}

function selectRuntimeDatabase() {
  runtimeDatabaseFileInput.value?.click()
}

async function loadRuntimeDatabase(event: Event) {
  const input = event.target as HTMLInputElement
  const file = input.files?.[0]
  input.value = ''
  if (!file) {
    return
  }
  databaseImportLoading.value = true
  try {
    const result = await uploadRuntimeDatabase(file)
    form.runtime_database_path = result.database_path
    toast.success(result.message)
    logout()
    router.push('/login')
  } catch (err) {
    toast.error(err instanceof Error ? err.message : '加载数据库失败')
  } finally {
    databaseImportLoading.value = false
  }
}

function runtimeStopTagColor(item: RuntimeStopPortResult) {
  if (item.stopped) {
    return 'success'
  }
  if (item.skipped) {
    return 'default'
  }
  return 'error'
}

async function loadDatabaseBackups(showError = true) {
  if (!form.database_backup_dir.trim()) {
    databaseBackups.value = []
    databaseBackupDir.value = ''
    return
  }
  databaseBackupLoading.value = true
  try {
    const result = await getRuntimeDatabaseBackups()
    databaseBackups.value = result.backups
    databaseBackupDir.value = result.backup_dir
  } catch (err) {
    databaseBackups.value = []
    databaseBackupDir.value = ''
    if (showError) {
      toast.error(err instanceof Error ? err.message : '读取备份列表失败')
    }
  } finally {
    databaseBackupLoading.value = false
  }
}

async function backupDatabaseNow() {
  databaseBackupLoading.value = true
  try {
    const result = await backupRuntimeDatabaseNow()
    databaseBackups.value = result.backups
    databaseBackupDir.value = result.backup_dir
    toast.success(result.message)
  } catch (err) {
    toast.error(err instanceof Error ? err.message : '创建数据库备份失败')
  } finally {
    databaseBackupLoading.value = false
  }
}

async function removeDatabaseBackup(name: string) {
  databaseBackupLoading.value = true
  try {
    const result = await deleteRuntimeDatabaseBackup(name)
    databaseBackups.value = result.backups
    databaseBackupDir.value = result.backup_dir
    toast.success('备份已删除。')
  } catch (err) {
    toast.error(err instanceof Error ? err.message : '删除备份失败')
  } finally {
    databaseBackupLoading.value = false
  }
}

function saveBlob(blob: Blob, filename: string) {
  const url = URL.createObjectURL(blob)
  const link = document.createElement('a')
  link.href = url
  link.download = filename
  document.body.appendChild(link)
  link.click()
  link.remove()
  URL.revokeObjectURL(url)
}

async function downloadDatabaseBackup(name: string) {
  databaseBackupDownloadName.value = name
  try {
    const result = await downloadRuntimeDatabaseBackup(name)
    saveBlob(result.blob, result.filename)
    toast.success('数据库备份下载已开始。')
  } catch (err) {
    toast.error(err instanceof Error ? err.message : '下载数据库备份失败')
  } finally {
    databaseBackupDownloadName.value = ''
  }
}

async function downloadConfigArchive() {
  configArchiveDownloading.value = true
  try {
    const result = await downloadRuntimeConfigArchive()
    saveBlob(result.blob, result.filename)
    toast.success('配置文件打包下载已开始。')
  } catch (err) {
    toast.error(err instanceof Error ? err.message : '打包下载配置文件失败')
  } finally {
    configArchiveDownloading.value = false
  }
}

function formatBackupTime(value: string) {
  if (!value) return '-'
  const date = new Date(value)
  if (Number.isNaN(date.getTime())) return value
  return date.toLocaleString()
}

function formatOptionalTime(value?: string | null) {
  if (!value) return '-'
  const date = new Date(value)
  if (Number.isNaN(date.getTime())) return value
  return date.toLocaleString()
}

function formatFileSize(size: number) {
  if (!Number.isFinite(size) || size <= 0) return '-'
  if (size < 1024) return `${size} B`
  if (size < 1024 * 1024) return `${(size / 1024).toFixed(1)} KB`
  if (size < 1024 * 1024 * 1024) return `${(size / 1024 / 1024).toFixed(1)} MB`
  return `${(size / 1024 / 1024 / 1024).toFixed(1)} GB`
}

async function saveAccount() {
  if (!accountForm.current_password) {
    toast.error('请填写当前密码以确认身份。')
    return
  }
  const trimmedUsername = accountForm.new_username.trim()
  const usernameChanged = trimmedUsername.length > 0 && trimmedUsername !== currentUsername.value
  const passwordChanged = accountForm.new_password.length > 0
  if (!usernameChanged && !passwordChanged) {
    toast.error('请至少修改用户名或密码中的一项。')
    return
  }
  if (passwordChanged && accountForm.new_password.length < 6) {
    toast.error('新密码至少 6 位。')
    return
  }
  if (passwordChanged && accountForm.new_password !== accountForm.confirm_password) {
    toast.error('两次输入的新密码不一致。')
    return
  }

  accountLoading.value = true
  try {
    const result = await updateAdminAccount({
      current_password: accountForm.current_password,
      new_username: usernameChanged ? trimmedUsername : undefined,
      new_password: passwordChanged ? accountForm.new_password : undefined,
    })
    currentAdmin.value = result.user
    currentUsername.value = result.user.username
    accountForm.new_username = result.user.username
    accountForm.current_password = ''
    accountForm.new_password = ''
    accountForm.confirm_password = ''
    toast.success('账号已更新，下次登录请使用新凭据。')
  } catch (err) {
    toast.error(err instanceof Error ? err.message : '账号更新失败')
  } finally {
    accountLoading.value = false
  }
}

function adminRoleLabel(role: string) {
  return role === 'super_admin' ? '超级管理员' : '管理员'
}

function adminRoleColor(role: string) {
  return role === 'super_admin' ? 'gold' : 'processing'
}

async function createAdmin() {
  const username = adminUserCreateForm.username.trim()
  if (!username) {
    toast.error('请填写用户名。')
    return
  }
  if (adminUserCreateForm.password.length < 6) {
    toast.error('密码至少 6 位。')
    return
  }
  adminUsersLoading.value = true
  try {
    await createAdminUser({
      username,
      password: adminUserCreateForm.password,
      role: adminUserCreateForm.role,
      is_enabled: adminUserCreateForm.is_enabled,
    })
    adminUserCreateForm.username = ''
    adminUserCreateForm.password = ''
    adminUserCreateForm.role = 'admin'
    adminUserCreateForm.is_enabled = true
    await loadAdminUsers(false)
    toast.success('管理员已创建。')
  } catch (err) {
    toast.error(err instanceof Error ? err.message : '创建管理员失败')
  } finally {
    adminUsersLoading.value = false
  }
}

async function saveAdminUser(user: AdminUser) {
  const username = user.username.trim()
  if (!username) {
    toast.error('用户名不能为空。')
    return
  }
  const newPassword = (adminUserPasswordEdits[user.id] || '').trim()
  if (newPassword && newPassword.length < 6) {
    toast.error('新密码至少 6 位。')
    return
  }
  adminUserSavingID.value = user.id
  try {
    await updateAdminUser(user.id, {
      username,
      role: user.role,
      is_enabled: user.is_enabled,
      new_password: newPassword || undefined,
    })
    adminUserPasswordEdits[user.id] = ''
    await loadAdminUsers(false)
    toast.success('管理员已更新。')
  } catch (err) {
    toast.error(err instanceof Error ? err.message : '更新管理员失败')
  } finally {
    adminUserSavingID.value = null
  }
}

async function removeAdminUser(user: AdminUser) {
  adminUserDeletingID.value = user.id
  try {
    await deleteAdminUser(user.id)
    await loadAdminUsers(false)
    toast.success('管理员已删除。')
  } catch (err) {
    toast.error(err instanceof Error ? err.message : '删除管理员失败')
  } finally {
    adminUserDeletingID.value = null
  }
}

onMounted(loadData)
</script>

<template>
  <component :is="settingsFrameComponent" :class="{ 'settings-embedded-frame': isDesktopEmbedded }">
    <div class="page-stack page-stack--fit settings-page">
      <a-row :gutter="[16, 16]" class="page-grid-fill">
        <a-col :xs="24">
          <a-card :bordered="false" class="admin-card admin-card--fill settings-tab-card">
            <a-tabs
              v-model:active-key="activeTab"
              aria-label="设置分类"
              class="settings-tabs"
              :animated="false"
            >
              <a-tab-pane key="schedule" tab="调度与执行">
                <div class="card-form">
                  <div class="card-scroll card-scroll--padded">
                    <a-form layout="vertical">
                  <a-row :gutter="16">
                    <a-col :xs="24" :md="12">
                      <a-form-item label="时区" html-for="settings-timezone">
                        <a-input id="settings-timezone" v-model:value="form.timezone" name="settings_timezone" />
                      </a-form-item>
                    </a-col>
                    <a-col :xs="24" :md="12">
                      <a-form-item label="每日执行时间" html-for="settings-daily-run-time">
                        <a-input id="settings-daily-run-time" v-model:value="form.daily_run_time" name="settings_daily_run_time" type="time" />
                      </a-form-item>
                    </a-col>
                  </a-row>

                  <a-row :gutter="16">
                    <a-col :xs="24" :md="12">
                      <a-form-item label="同站点 URL 并发数" html-for="settings-checkin-concurrency">
                        <a-input-number id="settings-checkin-concurrency" v-model:value="form.checkin_concurrency" name="settings_checkin_concurrency" style="width: 100%" :min="1" :max="20" />
                        <small class="field-help">限制同一 base_url 下多账号同时签到，默认 1 用于降低同站风控风险。</small>
                      </a-form-item>
                    </a-col>
                    <a-col :xs="24" :md="12">
                      <a-form-item label="不同站点总并发数" html-for="settings-checkin-global-concurrency">
                        <a-input-number id="settings-checkin-global-concurrency" v-model:value="form.checkin_global_concurrency" name="settings_checkin_global_concurrency" style="width: 100%" :min="1" :max="50" />
                        <small class="field-help">控制不同 base_url 之间可同时执行的总任务数。</small>
                      </a-form-item>
                    </a-col>
                  </a-row>

                  <a-row :gutter="16">
                    <a-col :xs="24" :md="12">
                      <a-form-item label="站点间隔（秒）" html-for="settings-checkin-interval-seconds">
                        <a-input-number id="settings-checkin-interval-seconds" v-model:value="form.checkin_interval_seconds" name="settings_checkin_interval_seconds" style="width: 100%" :min="0" :max="60" />
                      </a-form-item>
                    </a-col>
                  </a-row>

                  <a-row :gutter="16">
                    <a-col :xs="24" :md="12">
                      <a-form-item label="失败重试次数" html-for="settings-retry-count">
                        <a-input-number id="settings-retry-count" v-model:value="form.retry_count" name="settings_retry_count" style="width: 100%" :min="0" :max="5" />
                      </a-form-item>
                    </a-col>
                    <a-col :xs="24" :md="12">
                      <a-form-item label="请求超时（秒）" html-for="settings-request-timeout">
                        <a-input-number id="settings-request-timeout" v-model:value="form.request_timeout" name="settings_request_timeout" style="width: 100%" :min="5" :max="120" />
                      </a-form-item>
                    </a-col>
                  </a-row>

                  <a-space direction="vertical" size="middle">
                    <a-switch
                      v-model:checked="form.schedule_enabled"
                      checked-children="启用计划任务"
                      un-checked-children="关闭计划任务"
                    />
                    <a-switch
                      v-model:checked="form.only_enabled_sites"
                      checked-children="仅执行启用站点"
                      un-checked-children="执行全部站点"
                    />
                  </a-space>
                </a-form>

              <div class="card-actions card-actions--left">
                <a-space wrap>
                  <a-button type="primary" :loading="loading" @click="save">保存设置</a-button>
                  <a-button :loading="loading" @click="runNow">立即执行计划任务</a-button>
                </a-space>
              </div>
                  </div>
                </div>
              </a-tab-pane>
              <a-tab-pane key="runtime" tab="程序运行状况">
                <div class="card-form runtime-tab-form">
                  <div class="card-scroll card-scroll--padded">
                    <a-form layout="vertical">
                      <a-alert
                        v-if="form.security_warnings.length"
                        type="warning"
                        show-icon
                        message="安全与运维提醒"
                        :description="form.security_warnings.join('；')"
                        class="settings-security-alert"
                      />
                      <a-space direction="vertical" size="middle">
                        <a-switch
                          v-model:checked="form.desktop_keep_running"
                          checked-children="关闭桌面后保留本地服务"
                          un-checked-children="退出桌面时停止本地服务"
                        />
                      </a-space>

                      <a-row :gutter="16" class="desktop-runtime-row">
                        <a-col :xs="24" :md="12">
                          <a-form-item label="前端默认端口">
                            <a-input :value="String(form.desktop_frontend_default_port || 3721)" readonly />
                          </a-form-item>
                        </a-col>
                        <a-col :xs="24" :md="12">
                          <a-form-item label="前端当前端口">
                            <a-input :value="form.desktop_frontend_port ? String(form.desktop_frontend_port) : '-'" readonly />
                          </a-form-item>
                        </a-col>
                      </a-row>

                      <a-row :gutter="16">
                        <a-col :xs="24" :md="12">
                          <a-form-item label="前端地址">
                            <a-input :value="form.desktop_frontend_url || '-'" readonly />
                          </a-form-item>
                        </a-col>
                        <a-col :xs="24" :md="12">
                          <a-form-item label="前端默认端口占用">
                            <a-input :value="form.desktop_frontend_default_port_occupant || '未占用'" readonly />
                          </a-form-item>
                        </a-col>
                      </a-row>

                      <a-row :gutter="16" class="desktop-runtime-row">
                        <a-col :xs="24" :md="12">
                          <a-form-item label="后端默认端口">
                            <a-input :value="String(form.desktop_backend_default_port || 8972)" readonly />
                          </a-form-item>
                        </a-col>
                        <a-col :xs="24" :md="12">
                          <a-form-item label="后端当前端口">
                            <a-input :value="form.desktop_backend_port ? String(form.desktop_backend_port) : '-'" readonly />
                          </a-form-item>
                        </a-col>
                      </a-row>

                      <a-row :gutter="16">
                        <a-col :xs="24" :md="12">
                          <a-form-item label="后端地址">
                            <a-input :value="form.desktop_backend_url || '-'" readonly />
                          </a-form-item>
                        </a-col>
                        <a-col :xs="24" :md="12">
                          <a-form-item label="后端默认端口占用">
                            <a-input :value="form.desktop_backend_default_port_occupant || '未占用'" readonly />
                          </a-form-item>
                        </a-col>
                      </a-row>

                      <a-row :gutter="16">
                        <a-col :xs="24">
                          <a-form-item label="网关地址">
                            <a-input :value="form.desktop_gateway_url || '-'" readonly />
                          </a-form-item>
                        </a-col>
                      </a-row>

                      <div class="card-actions card-actions--left">
                        <a-space wrap>
                          <a-button type="primary" :loading="loading" @click="save">
                            <template #icon><SaveOutlined /></template>
                            保存设置
                          </a-button>
                          <a-button :loading="loading" @click="loadData">
                            <template #icon><ReloadOutlined /></template>
                            刷新运行状况
                          </a-button>
                          <a-button danger :loading="runtimeStopLoading" @click="stopOldPorts">
                            <template #icon><DeleteOutlined /></template>
                            停止旧版本端口
                          </a-button>
                        </a-space>
                      </div>

                      <div v-if="runtimeStopResults.length" class="runtime-stop-results">
                        <a-tag
                          v-for="item in runtimeStopResults"
                          :key="`${item.port}-${item.pid || 'none'}`"
                          :color="runtimeStopTagColor(item)"
                        >
                          {{ item.port }}：{{ item.message }}<span v-if="item.pid">（pid:{{ item.pid }}）</span>
                        </a-tag>
                      </div>
                    </a-form>
                  </div>
                </div>
              </a-tab-pane>
              <a-tab-pane key="database" tab="数据库">
                <div class="card-form runtime-tab-form">
                  <div class="card-scroll card-scroll--padded">
                    <a-form layout="vertical">
                      <a-form-item label="当前数据库文件">
                        <a-input :value="form.runtime_database_path || '-'" readonly />
                      </a-form-item>

                      <a-form-item label="加载数据库">
                        <input
                          ref="runtimeDatabaseFileInput"
                          class="runtime-database-file"
                          type="file"
                          accept=".db,.sqlite,.sqlite3,application/vnd.sqlite3,application/x-sqlite3"
                          hidden
                          tabindex="-1"
                          @change="loadRuntimeDatabase"
                        />
                        <a-button danger :loading="databaseImportLoading" @click="selectRuntimeDatabase">
                          <template #icon><DatabaseOutlined /></template>
                          选择并加载数据库
                        </a-button>
                        <small class="field-help">
                          选择 SQLite 数据库文件后会复制到当前配置目录并备份现有数据库；完成后会退出登录，请重新登录后生效。
                        </small>
                      </a-form-item>

                      <a-form-item label="日志配置">
                        <a-space direction="vertical" size="middle" class="runtime-backup-settings">
                          <a-row :gutter="16">
                            <a-col :xs="24" :md="12">
                              <a-form-item label="日志保留天数" html-for="settings-log-retention-days">
                                <a-input-number
                                  id="settings-log-retention-days"
                                  v-model:value="form.log_retention_days"
                                  name="settings_log_retention_days"
                                  style="width: 100%"
                                  :min="1"
                                  :max="365"
                                />
                              </a-form-item>
                            </a-col>
                          </a-row>
                        </a-space>
                        <small class="field-help">
                          默认保留 5 天；保存后会立即清理更早的签到运行日志和网关请求日志，后台也会每小时自动清理一次。
                        </small>
                      </a-form-item>

                      <a-form-item label="自动备份数据库">
                        <a-space direction="vertical" size="middle" class="runtime-backup-settings">
                          <a-switch
                            v-model:checked="form.database_backup_enabled"
                            checked-children="已启用"
                            un-checked-children="已关闭"
                          />
                          <a-input
                            v-model:value="form.database_backup_dir"
                            placeholder="~/ai-sign-in-gateway-backups"
                          />
                          <a-row :gutter="16">
                            <a-col :xs="24" :md="12">
                              <a-form-item label="备份间隔（分钟）">
                                <a-input-number
                                  v-model:value="form.database_backup_interval_minutes"
                                  style="width: 100%"
                                  :min="5"
                                  :max="10080"
                                />
                              </a-form-item>
                            </a-col>
                            <a-col :xs="24" :md="12">
                              <a-form-item label="保留份数">
                                <a-input-number
                                  v-model:value="form.database_backup_retention"
                                  style="width: 100%"
                                  :min="1"
                                  :max="365"
                                />
                              </a-form-item>
                            </a-col>
                          </a-row>
                        </a-space>
                        <small class="field-help">
                          保存设置后，只要服务进程还在，就会按间隔备份到该目录；目录需对当前运行用户可写。
                        </small>
                      </a-form-item>

                      <div class="card-actions card-actions--space">
                        <span class="backup-dir-label">备份目录：{{ databaseBackupDir || form.database_backup_dir || '未设置' }}</span>
                        <a-space wrap>
                          <a-button type="primary" :loading="loading" @click="save">
                            <template #icon><SaveOutlined /></template>
                            保存设置
                          </a-button>
                          <a-button :loading="databaseBackupLoading" @click="backupDatabaseNow">
                            立即备份
                          </a-button>
                          <a-button :loading="databaseBackupLoading" @click="() => loadDatabaseBackups()">
                            <template #icon><ReloadOutlined /></template>
                            刷新备份
                          </a-button>
                        </a-space>
                      </div>

                      <a-table
                        class="backup-table"
                        size="small"
                        :data-source="databaseBackups"
                        :loading="databaseBackupLoading"
                        :pagination="{ pageSize: 8, size: 'small' }"
                        row-key="name"
                      >
                        <a-table-column title="备份时间" key="created_at">
                          <template #default="{ record }">
                            {{ formatBackupTime(record.created_at) }}
                          </template>
                        </a-table-column>
                        <a-table-column title="文件名" data-index="name" key="name" />
                        <a-table-column title="大小" key="size" :width="110">
                          <template #default="{ record }">
                            {{ formatFileSize(record.size) }}
                          </template>
                        </a-table-column>
                        <a-table-column title="操作" key="actions" :width="160">
                          <template #default="{ record }">
                            <a-space size="small">
                              <a-button
                                size="small"
                                :loading="databaseBackupDownloadName === record.name"
                                @click="downloadDatabaseBackup(record.name)"
                              >
                                <template #icon><DownloadOutlined /></template>
                                下载
                              </a-button>
                              <a-popconfirm
                                title="确认删除这个备份？"
                                ok-text="删除"
                                cancel-text="取消"
                                @confirm="removeDatabaseBackup(record.name)"
                              >
                                <a-button danger size="small">删除</a-button>
                              </a-popconfirm>
                            </a-space>
                          </template>
                        </a-table-column>
                      </a-table>
                    </a-form>
                  </div>
                </div>
              </a-tab-pane>
              <a-tab-pane key="pricing" tab="价格">
                <div class="card-form runtime-tab-form">
                  <div class="card-scroll card-scroll--padded">
                    <a-form layout="vertical">
                      <a-alert
                        type="info"
                        show-icon
                        message="价格按上游返回的 usage token 计算"
                        description="官方价格方案不可修改；复制为自定义方案后可调整模型前缀和每 100 万 token 单价。未返回 usage 或未匹配价格的请求会计为未知费用。"
                        class="settings-security-alert"
                      />

                      <a-row :gutter="16">
                        <a-col :xs="24" :md="12">
                          <a-form-item label="当前价格方案">
                            <a-select
                              v-model:value="form.gateway_pricing_active_scheme_id"
                              :options="pricingSchemeOptions"
                            />
                          </a-form-item>
                        </a-col>
                        <a-col :xs="24" :md="12">
                          <a-form-item label="方案操作">
                            <a-space wrap>
                              <a-button @click="duplicateActivePricingScheme">
                                <template #icon><CopyOutlined /></template>
                                复制当前方案
                              </a-button>
                              <a-tag v-if="activePricingScheme?.readonly">官方只读</a-tag>
                              <a-tag v-else color="processing">自定义可编辑</a-tag>
                            </a-space>
                          </a-form-item>
                        </a-col>
                      </a-row>

                      <template v-if="activePricingScheme">
                        <a-row :gutter="16">
                          <a-col :xs="24" :md="12">
                            <a-form-item label="方案名称">
                              <a-input
                                v-model:value="activePricingScheme.name"
                                :readonly="!activePricingEditable"
                              />
                            </a-form-item>
                          </a-col>
                          <a-col :xs="24" :md="12">
                            <a-form-item label="来源">
                              <a-input
                                v-model:value="activePricingScheme.source"
                                :readonly="!activePricingEditable"
                              />
                            </a-form-item>
                          </a-col>
                        </a-row>

                        <div class="pricing-grid pricing-grid--head">
                          <span>提供方</span>
                          <span>模型前缀</span>
                          <span>显示名称</span>
                          <span>输入 / MTok</span>
                          <span>缓存读 / MTok</span>
                          <span>缓存写 / MTok</span>
                          <span>输出 / MTok</span>
                          <span>操作</span>
                        </div>
                        <div
                          v-for="(price, index) in activePricingScheme.prices"
                          :key="priceRowKey(price, index)"
                          class="pricing-grid"
                        >
                          <a-select
                            v-if="activePricingEditable"
                            v-model:value="price.provider"
                            :options="pricingProviderOptions"
                          />
                          <a-tag v-else>{{ price.provider }}</a-tag>
                          <a-input
                            v-model:value="price.model_prefix"
                            :readonly="!activePricingEditable"
                            placeholder="gpt-5.5"
                          />
                          <a-input
                            v-model:value="price.display_name"
                            :readonly="!activePricingEditable"
                            placeholder="显示名称"
                          />
                          <a-input-number
                            v-model:value="price.input_per_mtok"
                            style="width: 100%"
                            :min="0"
                            :step="0.001"
                            :disabled="!activePricingEditable"
                          />
                          <a-input-number
                            v-model:value="price.cached_input_per_mtok"
                            style="width: 100%"
                            :min="0"
                            :step="0.001"
                            :disabled="!activePricingEditable"
                          />
                          <a-input-number
                            v-model:value="price.cache_write_per_mtok"
                            style="width: 100%"
                            :min="0"
                            :step="0.001"
                            :disabled="!activePricingEditable"
                          />
                          <a-input-number
                            v-model:value="price.output_per_mtok"
                            style="width: 100%"
                            :min="0"
                            :step="0.001"
                            :disabled="!activePricingEditable"
                          />
                          <a-button
                            danger
                            size="small"
                            :disabled="!activePricingEditable"
                            @click="removePricingRow(index)"
                          >
                            删除
                          </a-button>
                        </div>

                        <div class="card-actions card-actions--left">
                          <a-space wrap>
                            <a-button :disabled="!activePricingEditable" @click="addPricingRow">
                              <template #icon><PlusOutlined /></template>
                              添加价格
                            </a-button>
                            <a-button type="primary" :loading="loading" @click="save">
                              <template #icon><SaveOutlined /></template>
                              保存设置
                            </a-button>
                          </a-space>
                        </div>
                      </template>
                    </a-form>
                  </div>
                </div>
              </a-tab-pane>
              <a-tab-pane key="extensions" tab="扩展">
                <div class="card-form runtime-tab-form">
                  <div class="card-scroll card-scroll--padded">
                    <a-form layout="vertical">
                      <a-list :data-source="form.features" item-layout="horizontal" class="extension-settings-list">
                        <template #renderItem="{ item }">
                          <a-list-item>
                            <a-list-item-meta :title="item.name" :description="item.description || item.key" />
                            <a-switch
                              :checked="Boolean(form.feature_flags[item.key] ?? item.default_enabled)"
                              checked-children="启用"
                              un-checked-children="关闭"
                              @change="(checked) => { form.feature_flags[item.key] = checked === true }"
                            />
                          </a-list-item>
                        </template>
                      </a-list>
                      <a-empty v-if="!form.features.length" description="未安装扩展" />
                      <div class="card-actions card-actions--left">
                        <a-button type="primary" :loading="loading" @click="save">
                          <template #icon><SaveOutlined /></template>
                          保存设置
                        </a-button>
                      </div>
                    </a-form>
                  </div>
                </div>
              </a-tab-pane>
              <a-tab-pane key="config" tab="配置文件">
                <div class="card-form runtime-tab-form">
                  <div class="card-scroll card-scroll--padded">
                    <a-form layout="vertical">
                      <div class="runtime-config-panel">
                        <a-row :gutter="16">
                          <a-col :xs="24" :md="12">
                            <a-form-item label="当前配置目录">
                              <a-input :value="form.runtime_config_dir || '-'" readonly />
                            </a-form-item>
                          </a-col>
                          <a-col :xs="24" :md="12">
                            <a-form-item label="默认配置目录">
                              <a-input :value="form.runtime_default_config_dir || '-'" readonly />
                            </a-form-item>
                          </a-col>
                        </a-row>

                        <a-form-item label="下次启动配置目录">
                          <a-input :value="form.runtime_pending_config_dir || form.runtime_config_dir || '-'" readonly />
                        </a-form-item>

                        <a-form-item label="加载配置目录">
                          <div class="runtime-config-loader">
                            <a-input
                              v-model:value="runtimeConfigDirInput"
                              placeholder="~/.ai-sign-in-gateway"
                            />
                            <a-button type="primary" :loading="configDirLoading" @click="loadRuntimeConfigDir">
                              <template #icon><FolderOpenOutlined /></template>
                              加载配置目录
                            </a-button>
                          </div>
                          <small class="field-help">
                            仅保存目录指针，重启后从该目录加载数据库；不会复制、导入或覆盖当前目录数据。
                          </small>
                        </a-form-item>

                        <a-form-item label="配置文件打包下载">
                          <div class="runtime-config-loader">
                            <a-input :value="form.runtime_config_dir || '-'" readonly />
                            <a-button :loading="configArchiveDownloading" @click="downloadConfigArchive">
                              <template #icon><DownloadOutlined /></template>
                              打包下载
                            </a-button>
                          </div>
                          <small class="field-help">
                            下载当前配置目录下的数据库、日志和配置文件 ZIP 包；用于迁移或离线备份。
                          </small>
                        </a-form-item>
                      </div>
                    </a-form>
                  </div>
                </div>
              </a-tab-pane>
              <a-tab-pane key="account" tab="账号与密码">
                <div class="card-form">
                  <div class="card-scroll card-scroll--padded">
                    <a-form layout="vertical">
                      <div class="account-meta">当前登录用户：<strong>{{ currentUsername || '...' }}</strong></div>
                      <a-row :gutter="16">
                        <a-col :xs="24" :md="12">
                          <a-form-item label="新用户名">
                            <a-input
                              v-model:value="accountForm.new_username"
                              placeholder="留空表示不修改用户名"
                              autocomplete="off"
                            />
                          </a-form-item>
                        </a-col>
                        <a-col :xs="24" :md="12">
                          <a-form-item label="当前密码" required>
                            <a-input-password
                              v-model:value="accountForm.current_password"
                              placeholder="必须填写当前密码以确认身份"
                              autocomplete="current-password"
                            />
                          </a-form-item>
                        </a-col>
                      </a-row>
                      <a-row :gutter="16">
                        <a-col :xs="24" :md="12">
                          <a-form-item label="新密码">
                            <a-input-password
                              v-model:value="accountForm.new_password"
                              placeholder="留空表示不修改密码（至少 6 位）"
                              autocomplete="new-password"
                            />
                          </a-form-item>
                        </a-col>
                        <a-col :xs="24" :md="12">
                          <a-form-item label="确认新密码">
                            <a-input-password
                              v-model:value="accountForm.confirm_password"
                              placeholder="再次输入新密码"
                              autocomplete="new-password"
                            />
                          </a-form-item>
                        </a-col>
                      </a-row>
                      <div class="card-actions card-actions--left">
                        <a-space>
                          <a-button type="primary" :loading="accountLoading" @click="saveAccount">
                            更新账号
                          </a-button>
                        </a-space>
                      </div>
                      <small class="account-hint">
                        修改成功后会自动续签登录凭据，建议下次登录使用新账号密码。
                      </small>

                      <template v-if="canManageAdminUsers">
                        <a-divider />
                        <div class="admin-users-panel">
                          <div class="admin-users-create">
                            <a-input
                              v-model:value="adminUserCreateForm.username"
                              placeholder="新管理员用户名"
                              autocomplete="off"
                            />
                            <a-input-password
                              v-model:value="adminUserCreateForm.password"
                              placeholder="初始密码"
                              autocomplete="new-password"
                            />
                            <a-select
                              v-model:value="adminUserCreateForm.role"
                              :options="roleOptions"
                            />
                            <a-switch
                              v-model:checked="adminUserCreateForm.is_enabled"
                              checked-children="启用"
                              un-checked-children="停用"
                            />
                            <a-button type="primary" :loading="adminUsersLoading" @click="createAdmin">
                              <template #icon><PlusOutlined /></template>
                              新增管理员
                            </a-button>
                          </div>

                          <a-table
                            class="admin-users-table"
                            size="small"
                            :data-source="adminUsers"
                            :loading="adminUsersLoading"
                            :pagination="false"
                            row-key="id"
                          >
                            <a-table-column title="用户名" key="username" :width="180">
                              <template #default="{ record }">
                                <a-input v-model:value="asAdminUser(record).username" autocomplete="off" />
                              </template>
                            </a-table-column>
                            <a-table-column title="角色" key="role" :width="150">
                              <template #default="{ record }">
                                <a-select v-model:value="asAdminUser(record).role" :options="roleOptions" style="width: 100%" />
                              </template>
                            </a-table-column>
                            <a-table-column title="状态" key="is_enabled" :width="120">
                              <template #default="{ record }">
                                <a-switch
                                  v-model:checked="asAdminUser(record).is_enabled"
                                  checked-children="启用"
                                  un-checked-children="停用"
                                  :disabled="asAdminUser(record).id === currentAdmin?.id"
                                />
                              </template>
                            </a-table-column>
                            <a-table-column title="新密码" key="new_password" :width="180">
                              <template #default="{ record }">
                                <a-input-password
                                  v-model:value="adminUserPasswordEdits[asAdminUser(record).id]"
                                  placeholder="留空不修改"
                                  autocomplete="new-password"
                                />
                              </template>
                            </a-table-column>
                            <a-table-column title="最后登录" key="last_login_at" :width="170">
                              <template #default="{ record }">
                                {{ formatOptionalTime(asAdminUser(record).last_login_at) }}
                              </template>
                            </a-table-column>
                            <a-table-column title="标签" key="tag" :width="120">
                              <template #default="{ record }">
                                <a-space size="small">
                                  <a-tag :color="adminRoleColor(asAdminUser(record).role)">{{ adminRoleLabel(asAdminUser(record).role) }}</a-tag>
                                </a-space>
                              </template>
                            </a-table-column>
                            <a-table-column title="操作" key="actions" :width="180">
                              <template #default="{ record }">
                                <a-space size="small">
                                  <a-button
                                    size="small"
                                    type="primary"
                                    :loading="adminUserSavingID === asAdminUser(record).id"
                                    @click="saveAdminUser(asAdminUser(record))"
                                  >
                                    保存
                                  </a-button>
                                  <a-popconfirm
                                    title="确认删除这个管理员？"
                                    ok-text="删除"
                                    cancel-text="取消"
                                    :disabled="asAdminUser(record).id === currentAdmin?.id"
                                    @confirm="removeAdminUser(asAdminUser(record))"
                                  >
                                    <a-button
                                      danger
                                      size="small"
                                      :disabled="asAdminUser(record).id === currentAdmin?.id"
                                      :loading="adminUserDeletingID === asAdminUser(record).id"
                                    >
                                      删除
                                    </a-button>
                                  </a-popconfirm>
                                </a-space>
                              </template>
                            </a-table-column>
                          </a-table>
                        </div>
                      </template>
                    </a-form>
                  </div>
                </div>
              </a-tab-pane>
            </a-tabs>
          </a-card>
        </a-col>
      </a-row>
    </div>
  </component>
</template>

<style scoped>
.settings-embedded-frame {
  display: block;
  min-width: 0;
  min-height: 0;
}

.settings-embedded-frame .page-stack {
  height: auto;
  min-height: 0;
  padding: 0;
}

.settings-embedded-frame .page-grid-fill {
  min-height: 0;
}

.settings-tab-card :deep(.ant-card-body) {
  padding: 0 16px 16px;
}

.settings-tabs {
  display: flex;
  flex: 1;
  flex-direction: column;
  height: 100%;
  min-height: 0;
  overflow: hidden;
}

.settings-tabs :deep(.ant-tabs-nav) {
  flex: 0 0 auto;
  margin-bottom: 8px;
}

.settings-tabs :deep(.ant-tabs-content-holder),
.settings-tabs :deep(.ant-tabs-content),
.settings-tabs :deep(.ant-tabs-tabpane) {
  flex: 1;
  width: 100%;
  height: 100%;
  min-width: 0;
  min-height: 0;
}

.settings-tabs :deep(.ant-tabs-content-holder) {
  overflow: hidden;
}

.settings-tabs :deep(.ant-tabs-content) {
  display: flex;
}

.settings-tabs :deep(.ant-tabs-tabpane) {
  display: flex;
  flex-direction: column;
  overflow: hidden;
}

.settings-tabs :deep(.ant-tabs-ink-bar),
.settings-tabs :deep(.ant-tabs-content) {
  transition: none !important;
}

@media (max-width: 720px) {
  .settings-tabs :deep(.ant-tabs-nav) {
    margin-bottom: 10px;
  }

  .settings-tabs :deep(.ant-tabs-nav-wrap) {
    overflow-x: auto !important;
    overflow-y: hidden !important;
    scrollbar-width: none;
    -webkit-overflow-scrolling: touch;
  }

  .settings-tabs :deep(.ant-tabs-nav-wrap::-webkit-scrollbar) {
    display: none;
  }

  .settings-tabs :deep(.ant-tabs-nav-list) {
    min-width: max-content;
    transform: none !important;
  }

  .settings-tabs :deep(.ant-tabs-nav-operations) {
    display: none !important;
  }

  .settings-tabs :deep(.ant-tabs-tab) {
    flex: 0 0 auto;
    min-width: 44px;
    min-height: 36px;
    padding: 8px 8px;
  }

  .settings-tabs :deep(.ant-tabs-tab-btn) {
    display: inline-flex;
    align-items: center;
    justify-content: center;
    min-width: 32px;
    min-height: 32px;
    line-height: 20px;
  }

  .settings-tab-card :deep(.ant-input-number-handler) {
    min-width: 32px;
    height: 20px;
  }

  .settings-tab-card :deep(.ant-input-number-handler-wrap) {
    width: 32px;
  }

  .settings-tab-card :deep(.ant-input-number-handler-up-inner),
  .settings-tab-card :deep(.ant-input-number-handler-down-inner) {
    min-width: 32px;
  }
}

.settings-tab-card :deep(.card-scroll) {
  overflow-x: hidden;
  overflow-y: auto;
  -webkit-overflow-scrolling: touch;
}

.runtime-tab-form {
  min-height: 0;
}

.runtime-tab-form .card-scroll {
  max-height: 100%;
  overscroll-behavior: contain;
}

.settings-tab-card :deep(.ant-form),
.settings-tab-card :deep(.ant-row),
.settings-tab-card :deep(.ant-col),
.settings-tab-card :deep(.ant-form-item),
.settings-tab-card :deep(.ant-form-item-control),
.settings-tab-card :deep(.ant-form-item-control-input),
.settings-tab-card :deep(.ant-form-item-control-input-content) {
  max-width: 100%;
  min-width: 0;
}

.account-meta {
  font-size: 12px;
  color: #6b7280;
  margin-bottom: 12px;
}

.account-meta strong {
  color: #24334d;
  margin-left: 4px;
}

.account-hint {
  display: block;
  margin-top: 8px;
  font-size: 11px;
  color: #94a3b8;
}

.admin-users-panel {
  display: grid;
  gap: 14px;
  min-width: 0;
}

.admin-users-create {
  display: grid;
  grid-template-columns: minmax(140px, 1fr) minmax(140px, 1fr) minmax(130px, 0.8fr) auto auto;
  gap: 8px;
  align-items: center;
  min-width: 0;
}

.admin-users-table {
  min-width: 0;
}

.admin-users-table :deep(.ant-table-cell) {
  overflow-wrap: anywhere;
}

.settings-security-alert {
  margin-bottom: 14px;
}

.runtime-config-panel {
  margin-top: 14px;
}

.runtime-config-loader {
  display: grid;
  grid-template-columns: minmax(0, 1fr) auto;
  gap: 8px;
  width: 100%;
  min-width: 0;
}

.runtime-config-loader :deep(.ant-input) {
  width: 100%;
  min-width: 0;
}

.runtime-config-loader :deep(.ant-btn) {
  width: auto;
  min-width: 0;
  white-space: nowrap;
}

.runtime-database-file {
  position: absolute;
  width: 1px;
  height: 1px;
  overflow: hidden;
  clip: rect(0 0 0 0);
  clip-path: inset(50%);
  white-space: nowrap;
}

.runtime-backup-settings {
  width: 100%;
}

.runtime-backup-settings :deep(.ant-space-item),
.runtime-backup-settings :deep(.ant-input),
.runtime-backup-settings :deep(.ant-row) {
  width: 100%;
}

.backup-dir-label {
  min-width: 0;
  color: var(--text-muted);
  font-size: 12px;
  overflow-wrap: anywhere;
}

.backup-table {
  margin-top: 12px;
}

.backup-table :deep(.ant-table-cell) {
  overflow-wrap: anywhere;
}

.pricing-grid {
  display: grid;
  grid-template-columns: minmax(110px, 0.75fr) minmax(140px, 1.1fr) minmax(130px, 1fr) repeat(4, minmax(96px, 0.75fr)) 72px;
  gap: 8px;
  align-items: center;
  margin-bottom: 8px;
  min-width: 0;
}

.pricing-grid--head {
  margin-top: 6px;
  color: var(--text-muted);
  font-size: 12px;
  font-weight: 600;
}

.pricing-grid :deep(.ant-tag) {
  margin-inline-end: 0;
}

.desktop-runtime-row {
  margin-top: 8px;
}

.runtime-stop-results {
  display: flex;
  flex-wrap: wrap;
  gap: 6px;
  margin-top: 12px;
  min-width: 0;
}

.runtime-stop-results :deep(.ant-tag) {
  max-width: 100%;
  overflow-wrap: anywhere;
  white-space: normal;
}

@media (max-width: 680px) {
  .admin-users-create {
    grid-template-columns: minmax(0, 1fr);
    align-items: stretch;
  }

  .admin-users-create :deep(.ant-btn) {
    width: 100%;
  }

  .runtime-config-loader {
    grid-template-columns: minmax(0, 1fr);
  }

  .runtime-config-loader :deep(.ant-btn) {
    width: 100%;
  }

  .pricing-grid {
    min-width: 0;
    grid-template-columns: minmax(0, 1fr);
    align-items: stretch;
  }

  .pricing-grid--head {
    display: none;
  }
}
</style>
