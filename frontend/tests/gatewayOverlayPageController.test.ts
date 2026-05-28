import test from 'node:test'
import assert from 'node:assert/strict'
import { readFile } from 'node:fs/promises'
import { reactive, ref } from 'vue'

import { useGatewayOverlayPageBindings } from '../src/gatewayOverlayPageController.ts'
import type { AddUpstreamForm } from '../src/gatewayAddUpstreamModel.ts'
import type { GatewayPriorityPresetMode } from '../src/gatewayPriorityModel.ts'
import type { GatewayLog, GatewayRoute, GatewaySettingsData } from '../src/types.ts'

const gatewayViewPath = new URL('../src/views/GatewayView.vue', import.meta.url)
const gatewayPageControllerPath = new URL('../src/gatewayPageController.ts', import.meta.url)
const shellBindingsControllerPath = new URL('../src/gatewayPageShellBindingsController.ts', import.meta.url)
const pageBindingsControllerPath = new URL('../src/gatewayPageBindingsController.ts', import.meta.url)

function route(overrides: Partial<GatewayRoute> = {}): GatewayRoute {
  return {
    id: 1,
    route_type: 'codex',
    is_enabled: true,
    ...overrides,
  } as GatewayRoute
}

function log(overrides: Partial<GatewayLog> = {}): GatewayLog {
  return {
    id: 1,
    route_id: 1,
    success: true,
    method: 'POST',
    request_path: '/v1/chat/completions',
    model: 'gpt-test',
    user_agent: 'test-agent',
    latency_ms: 100,
    attempt_index: 0,
    is_stream: false,
    created_at: '2026-05-27T00:00:00.000Z',
    ...overrides,
  } as GatewayLog
}

test('useGatewayOverlayPageBindings maps overlay props and events from page state', () => {
  const events: string[] = []
  const priorityDialog = {
    open: ref(false),
    loading: ref(false),
    route: ref<GatewayRoute | null>(route({ id: 1 })),
    insertIndex: ref<number | undefined>(undefined),
    routes: ref([route({ id: 1 })]),
    rowClassName: () => 'priority-route-row',
  }
  const balanceManualDialog = {
    open: ref(false),
    loading: ref(false),
    route: ref<GatewayRoute | null>(route({ id: 2 })),
    url: ref('https://api.example.com/balance'),
    message: ref('余额失败'),
  }
  const settingsDialog = {
    open: ref(false),
    loading: ref(false),
    form: reactive({ gateway_api_key: 'key-test' }),
  }
  const addUpstreamDialog = {
    open: ref(false),
    loading: ref(false),
    form: reactive({ name: 'upstream' }),
    groupNames: ref(['default']),
  }
  const routeModelsDialog = {
    open: ref(false),
    saving: ref(false),
    route: ref<GatewayRoute | null>(route({ id: 3 })),
    requestURLs: ref('https://api.example.com/v1'),
    supportedModels: ref(['gpt-test']),
  }
  const logsDrawer = {
    open: ref(false),
    search: ref('latest'),
    logs: ref([log({ id: 10 })]),
  }
  const routeLogsDrawer = {
    open: ref(false),
    loading: ref(false),
    route: ref<GatewayRoute | null>(route({ id: 4 })),
    search: ref('route-log'),
    logs: ref([log({ id: 11 })]),
  }
  const routeDiagnosisDrawer = {
    open: ref(false),
    loading: ref(false),
    diagnosis: ref(null),
  }
  const priorityColumns = [{ key: 'priority' }]
  const groupOptions = ref([{ label: '默认', value: 'default' }])
  const logColumns = [{ key: 'created_at' }]
  const filteredLogs = ref([log({ id: 20 })])
  const filteredRouteLogs = ref([log({ id: 21 })])
  const drawerTableY = ref(480)
  const routeRowKey = (targetRoute: GatewayRoute) => targetRoute.id
  const logRowKey = (targetLog: GatewayLog) => targetLog.id
  const loadRouteLabel = (targetRoute: GatewayRoute) => `route-${targetRoute.id}`

  const { overlayPageProps, overlayPageHandlers } = useGatewayOverlayPageBindings({
    priorityDialog,
    balanceManualDialog,
    settingsDialog,
    addUpstreamDialog,
    routeModelsDialog,
    logsDrawer,
    routeLogsDrawer,
    routeDiagnosisDrawer,
    priorityColumns,
    routeRowKey,
    loadRouteLabel,
    routePriorityLabel: (targetRoute) => `priority-${targetRoute?.id ?? 'none'}`,
    formatGroupNames: (value) => String(value ?? ''),
    groupOptions,
    logColumns,
    logs: filteredLogs,
    routeLogs: filteredRouteLogs,
    pageSize: 20,
    drawerTableY,
    logRowKey,
    formatTime: (value) => value ?? '暂无',
    requestMethodColor: () => 'blue',
    logMethodLabel: (targetLog) => targetLog.method,
    logRequestLabel: (targetLog) => targetLog.request_path,
    logRequestURL: (targetLog) => targetLog.request_path,
    logRouteLabel: (targetLog) => `route-${targetLog.route_id}`,
    logRouteMeta: (targetLog) => `meta-${targetLog.route_id}`,
    logModelMeta: (targetLog) => targetLog.model,
    logUserAgent: (targetLog) => targetLog.user_agent,
    handlePriorityMove: () => events.push('priority-move'),
    handlePriorityPreset: (mode) => events.push(`priority-preset:${mode}`),
    submitManualRouteBalanceProbe: () => events.push('balance-submit'),
    saveSettings: (settings) => events.push(`settings-save:${settings.gateway_api_key}`),
    submitAddUpstream: (form, groupNames) => events.push(`add-upstream-submit:${form.name}:${groupNames.join(',')}`),
    resetAddUpstreamForm: () => events.push('add-upstream-reset'),
    saveRouteModelsDialog: () => events.push('route-models-save'),
  })

  assert.equal(overlayPageProps.value.priorityDialog, priorityDialog)
  assert.equal(overlayPageProps.value.balanceManualDialog, balanceManualDialog)
  assert.equal(overlayPageProps.value.settingsDialog, settingsDialog)
  assert.equal(overlayPageProps.value.addUpstreamDialog, addUpstreamDialog)
  assert.equal(overlayPageProps.value.routeModelsDialog, routeModelsDialog)
  assert.equal(overlayPageProps.value.logsDrawer, logsDrawer)
  assert.equal(overlayPageProps.value.routeLogsDrawer, routeLogsDrawer)
  assert.equal(overlayPageProps.value.routeDiagnosisDrawer, routeDiagnosisDrawer)
  assert.equal(overlayPageProps.value.priorityColumns, priorityColumns)
  assert.equal(overlayPageProps.value.routeRowKey, routeRowKey)
  assert.equal(overlayPageProps.value.loadRouteLabel, loadRouteLabel)
  assert.equal(overlayPageProps.value.groupOptions, groupOptions.value)
  assert.equal(overlayPageProps.value.logColumns, logColumns)
  assert.equal(overlayPageProps.value.logs, filteredLogs.value)
  assert.equal(overlayPageProps.value.routeLogs, filteredRouteLogs.value)
  assert.equal(overlayPageProps.value.pageSize, 20)
  assert.equal(overlayPageProps.value.drawerTableY, 480)
  assert.equal(overlayPageProps.value.logRowKey, logRowKey)

  groupOptions.value = [{ label: 'VIP', value: 'vip' }]
  filteredLogs.value = [log({ id: 30 })]
  filteredRouteLogs.value = [log({ id: 31 })]
  drawerTableY.value = 520

  assert.deepEqual(overlayPageProps.value.groupOptions, [{ label: 'VIP', value: 'vip' }])
  assert.equal(overlayPageProps.value.logs[0].id, 30)
  assert.equal(overlayPageProps.value.routeLogs[0].id, 31)
  assert.equal(overlayPageProps.value.drawerTableY, 520)

  overlayPageHandlers['priority-move']()
  overlayPageHandlers['priority-preset']('balance' as GatewayPriorityPresetMode)
  overlayPageHandlers['balance-submit']()
  overlayPageHandlers['settings-save']({ gateway_api_key: 'key-from-dialog' } as GatewaySettingsData)
  overlayPageHandlers['add-upstream-submit']({ name: '上游 C' } as AddUpstreamForm, ['默认'])
  overlayPageHandlers['add-upstream-reset']()
  overlayPageHandlers['route-models-save']()

  assert.deepEqual(events, [
    'priority-move',
    'priority-preset:balance',
    'balance-submit',
    'settings-save:key-from-dialog',
    'add-upstream-submit:上游 C:默认',
    'add-upstream-reset',
    'route-models-save',
  ])
})

test('GatewayView delegates overlay page bindings through the overlay page controller', async () => {
  const viewSource = await readFile(gatewayViewPath, 'utf8')
  const pageControllerSource = await readFile(gatewayPageControllerPath, 'utf8')
  const shellBindingsControllerSource = await readFile(shellBindingsControllerPath, 'utf8')

  assert.match(viewSource, /useGatewayPageController\(\{/)
  assert.doesNotMatch(viewSource, /useGatewayPageState\(\)/)
  assert.doesNotMatch(viewSource, /useGatewayRuntimeOperationsPageActions\(\{/)
  assert.doesNotMatch(viewSource, /useGatewayRouteManagementOperationsPageActions\(\{/)
  assert.doesNotMatch(viewSource, /useGatewayAdminOperationsPageActions\(\{/)
  assert.ok(shellBindingsControllerSource.includes("useGatewayPageBindings"), "GatewayView delegates overlay page bindings through the overlay page controller should keep useGatewayPageBindings in gateway page controller")
  assert.ok(pageControllerSource.includes("overlayPageHandlers"), "GatewayView delegates overlay page bindings through the overlay page controller should keep overlayPageHandlers in gateway page controller")
})
