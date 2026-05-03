<script setup lang="ts">
import { DatabaseOutlined, DeleteOutlined, DownloadOutlined, FolderOpenOutlined, ReloadOutlined, SaveOutlined } from '@ant-design/icons-vue'
import { computed, onMounted, reactive, ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import {
  backupRuntimeDatabaseNow,
  deleteRuntimeDatabaseBackup,
  downloadRuntimeConfigArchive,
  downloadRuntimeDatabaseBackup,
  getMe,
  getRuntimeDatabaseBackups,
  getSettings,
  logout,
  runSchedulerNow,
  setRuntimeConfigDir,
  stopStaleRuntimePorts,
  uploadRuntimeDatabase,
  updateAdminAccount,
  updateSettings,
} from '../api'
import ShellLayout from '../components/ShellLayout.vue'
import { useToast } from '../toast'
import type { RuntimeDatabaseBackupFile, RuntimeStopPortResult, SettingsData } from '../types'

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
const activeTab = ref<'schedule' | 'runtime' | 'database' | 'config' | 'account'>('schedule')
const isDesktopEmbedded = computed(() => route.path === '/desktop')
const settingsFrameComponent = computed(() => (isDesktopEmbedded.value ? 'div' : ShellLayout))
const accountForm = reactive({
  new_username: '',
  current_password: '',
  new_password: '',
  confirm_password: '',
})

async function loadData() {
  loading.value = true
  try {
    const settings = await getSettings()
    Object.assign(form, settings)
    runtimeConfigDirInput.value = settings.runtime_pending_config_dir || settings.runtime_config_dir || settings.runtime_default_config_dir || ''
    await loadDatabaseBackups(false)
    try {
      const me = await getMe()
      currentUsername.value = me.username
      accountForm.new_username = me.username
    } catch (err) {
      toast.error(err instanceof Error ? err.message : '获取当前账号失败')
    }
  } catch (err) {
    toast.error(err instanceof Error ? err.message : '加载失败')
  } finally {
    loading.value = false
  }
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

onMounted(loadData)
</script>

<template>
  <component :is="settingsFrameComponent" :class="{ 'settings-embedded-frame': isDesktopEmbedded }">
    <div class="page-stack page-stack--fit">
      <a-row :gutter="[16, 16]" class="page-grid-fill">
        <a-col :xs="24">
          <a-card :bordered="false" class="admin-card admin-card--fill settings-tab-card">
            <a-tabs v-model:active-key="activeTab" class="settings-tabs" :animated="false">
              <a-tab-pane key="schedule" tab="调度与执行">
                <div class="card-form">
                  <div class="card-scroll card-scroll--padded">
                    <a-form layout="vertical">
                  <a-row :gutter="16">
                    <a-col :xs="24" :md="12">
                      <a-form-item label="时区">
                        <a-input v-model:value="form.timezone" />
                      </a-form-item>
                    </a-col>
                    <a-col :xs="24" :md="12">
                      <a-form-item label="每日执行时间">
                        <a-input v-model:value="form.daily_run_time" type="time" />
                      </a-form-item>
                    </a-col>
                  </a-row>

                  <a-row :gutter="16">
                    <a-col :xs="24" :md="12">
                      <a-form-item label="同站点 URL 并发数">
                        <a-input-number v-model:value="form.checkin_concurrency" style="width: 100%" :min="1" :max="20" />
                        <small class="field-help">限制同一 base_url 下多账号同时签到，默认 1 用于降低同站风控风险。</small>
                      </a-form-item>
                    </a-col>
                    <a-col :xs="24" :md="12">
                      <a-form-item label="不同站点总并发数">
                        <a-input-number v-model:value="form.checkin_global_concurrency" style="width: 100%" :min="1" :max="50" />
                        <small class="field-help">控制不同 base_url 之间可同时执行的总任务数。</small>
                      </a-form-item>
                    </a-col>
                  </a-row>

                  <a-row :gutter="16">
                    <a-col :xs="24" :md="12">
                      <a-form-item label="站点间隔（秒）">
                        <a-input-number v-model:value="form.checkin_interval_seconds" style="width: 100%" :min="0" :max="60" />
                      </a-form-item>
                    </a-col>
                  </a-row>

                  <a-row :gutter="16">
                    <a-col :xs="24" :md="12">
                      <a-form-item label="失败重试次数">
                        <a-input-number v-model:value="form.retry_count" style="width: 100%" :min="0" :max="5" />
                      </a-form-item>
                    </a-col>
                    <a-col :xs="24" :md="12">
                      <a-form-item label="请求超时（秒）">
                        <a-input-number v-model:value="form.request_timeout" style="width: 100%" :min="5" :max="120" />
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
                        修改成功后会自动续签登录凭据，老 token 仍有效；建议下次登录使用新账号密码。
                      </small>
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
  .runtime-config-loader {
    grid-template-columns: minmax(0, 1fr);
  }

  .runtime-config-loader :deep(.ant-btn) {
    width: 100%;
  }
}
</style>
