import { computed, onMounted, reactive, ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import {
  createAdminUser,
  deleteAdminUser,
  getAdminUsers,
  getMe,
  getSettings,
  runSchedulerNow,
  updateAdminAccount,
  updateAdminUser,
  updateSettings,
} from './api'
import ShellLayout from './components/ShellLayout.vue'
import {
  adminRoleColor,
  adminRoleLabel,
  asAdminUser,
  clonePricingScheme,
  createAdminUserForm,
  createPricingRow,
  createSettingsForm,
  formatBackupTime,
  formatFileSize,
  formatOptionalTime,
  priceRowKey,
  pricingProviderOptions,
  roleOptions,
  runtimeStopTagColor,
} from './settingsViewModel'
import { useSettingsRuntimeController } from './settingsRuntimeController'
import { useToast } from './toast'
import type { AdminUser, SettingsData } from './types'

type SettingsTabKey = 'schedule' | 'runtime' | 'database' | 'pricing' | 'extensions' | 'config' | 'account'

export function useSettingsViewController() {
  const toast = useToast()
  const route = useRoute()
  const router = useRouter()
  const form = reactive<SettingsData>(createSettingsForm())
  const loading = ref(false)
  const accountLoading = ref(false)
  const adminUsersLoading = ref(false)
  const adminUserSavingID = ref<number | null>(null)
  const adminUserDeletingID = ref<number | null>(null)
  const currentUsername = ref('')
  const currentAdmin = ref<AdminUser | null>(null)
  const adminUsers = ref<AdminUser[]>([])
  const activeTab = ref<SettingsTabKey>('schedule')
  const isDesktopEmbedded = computed(() => route.path === '/desktop')
  const settingsFrameComponent = computed(() => (isDesktopEmbedded.value ? 'div' : ShellLayout))
  const canManageAdminUsers = computed(() => currentAdmin.value?.role === 'super_admin')
  const accountForm = reactive({
    new_username: '',
    current_password: '',
    new_password: '',
    confirm_password: '',
  })
  const adminUserCreateForm = reactive(createAdminUserForm())
  const adminUserPasswordEdits = reactive<Record<number, string>>({})
  const pricingSchemeOptions = computed(() =>
    form.gateway_pricing_schemes.map((scheme) => ({
      label: scheme.readonly ? `${scheme.name}（只读）` : scheme.name,
      value: scheme.id,
    })),
  )
  const activePricingScheme = computed(() =>
    form.gateway_pricing_schemes.find((scheme) => scheme.id === form.gateway_pricing_active_scheme_id) ??
    form.gateway_pricing_schemes[0] ??
    null,
  )
  const activePricingEditable = computed(() => Boolean(activePricingScheme.value && !activePricingScheme.value.readonly))

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
      runtime.runtimeConfigDirInput.value =
        settings.runtime_pending_config_dir || settings.runtime_config_dir || settings.runtime_default_config_dir || ''
      await runtime.loadDatabaseBackups(false)
      await loadCurrentAccount()
    } catch (err) {
      toast.error(err instanceof Error ? err.message : '加载失败')
    } finally {
      loading.value = false
    }
  }

  const runtime = useSettingsRuntimeController({
    form,
    toast,
    reloadData: loadData,
    goLogin: () => router.push('/login'),
  })

  async function loadCurrentAccount() {
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

  function duplicateActivePricingScheme() {
    const source = activePricingScheme.value
    if (!source) return
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
    if (!scheme || scheme.readonly) return
    scheme.prices.push(createPricingRow())
  }

  function removePricingRow(index: number) {
    const scheme = activePricingScheme.value
    if (!scheme || scheme.readonly) return
    scheme.prices.splice(index, 1)
  }

  async function save() {
    loading.value = true
    try {
      Object.assign(form, await updateSettings(form))
      await runtime.loadDatabaseBackups(false)
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

  async function saveAccount() {
    if (!validateAccountForm()) return
    const trimmedUsername = accountForm.new_username.trim()
    const usernameChanged = trimmedUsername.length > 0 && trimmedUsername !== currentUsername.value
    const passwordChanged = accountForm.new_password.length > 0
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

  function validateAccountForm() {
    const usernameChanged = accountForm.new_username.trim().length > 0 && accountForm.new_username.trim() !== currentUsername.value
    const passwordChanged = accountForm.new_password.length > 0
    if (!accountForm.current_password) {
      toast.error('请填写当前密码以确认身份。')
      return false
    }
    if (!usernameChanged && !passwordChanged) {
      toast.error('请至少修改用户名或密码中的一项。')
      return false
    }
    if (passwordChanged && accountForm.new_password.length < 6) {
      toast.error('新密码至少 6 位。')
      return false
    }
    if (passwordChanged && accountForm.new_password !== accountForm.confirm_password) {
      toast.error('两次输入的新密码不一致。')
      return false
    }
    return true
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
      Object.assign(adminUserCreateForm, createAdminUserForm())
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

  return reactive({
    form,
    loading,
    accountLoading,
    adminUsersLoading,
    adminUserSavingID,
    adminUserDeletingID,
    currentUsername,
    currentAdmin,
    adminUsers,
    activeTab,
    isDesktopEmbedded,
    settingsFrameComponent,
    canManageAdminUsers,
    pricingSchemeOptions,
    activePricingScheme,
    activePricingEditable,
    accountForm,
    adminUserCreateForm,
    adminUserPasswordEdits,
    pricingProviderOptions,
    roleOptions,
    asAdminUser,
    adminRoleLabel,
    adminRoleColor,
    formatBackupTime,
    formatFileSize,
    formatOptionalTime,
    priceRowKey,
    runtimeStopTagColor,
    loadData,
    loadAdminUsers,
    duplicateActivePricingScheme,
    addPricingRow,
    removePricingRow,
    save,
    runNow,
    saveAccount,
    createAdmin,
    saveAdminUser,
    removeAdminUser,
    ...runtime,
  })
}

export type SettingsViewController = ReturnType<typeof useSettingsViewController>
