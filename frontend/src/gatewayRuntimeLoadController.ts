import {
  buildGatewayActiveRequestsLoadErrorPlan,
  buildGatewayActiveRequestsLoadResultPlan,
} from './gatewayActiveRequestsLoadModel.ts'
import {
  buildGatewayRealtimeRefreshApplyPlan,
  buildGatewayRealtimeRefreshPlan,
} from './gatewayRealtimeRefreshModel.ts'
import {
  buildGatewayUsageLoadErrorPlan,
  buildGatewayUsageLoadPlan,
  buildGatewayUsageLoadResultPlan,
} from './gatewayUsageLoadModel.ts'

type RefreshStartOptions = {
  now: number
  visible: boolean
}

type AbortControllerLike = {
  signal: {
    aborted: boolean
  }
  abort: () => void
}

type RuntimeControllerSlot<T extends AbortControllerLike> = {
  replace: () => T
  clearIfCurrent: (controller: T) => boolean
}

type GatewayUsageRequestRange = {
  start: string
  end: string
}

type LoadGatewayActiveRequestsOptions<TSnapshot, TController extends AbortControllerLike = AbortController> = {
  silent: boolean
  mounted: () => boolean
  controllerSlot: RuntimeControllerSlot<TController>
  requestActiveRequests: (options: { signal: TController['signal'] }) => Promise<TSnapshot[]>
  setActiveRequests: (snapshot: TSnapshot[]) => void
  applyActiveRequestSnapshot: (snapshot: TSnapshot[]) => void
  showPlanNotice: (plan: Extract<ReturnType<typeof buildGatewayActiveRequestsLoadErrorPlan>, { showError: true }>) => void
}

type LoadGatewayUsageOptions<TUsage, TController extends AbortControllerLike = AbortController> = {
  silent: boolean
  isMonitor: boolean
  requestRange: GatewayUsageRequestRange
  mounted: () => boolean
  controllerSlot: RuntimeControllerSlot<TController>
  requestUsage: (options: GatewayUsageRequestRange & { signal: TController['signal'] }) => Promise<TUsage>
  setUsage: (usage: TUsage | null) => void
  setUsageLoading: (loading: boolean) => void
  showNotice: (notice: NonNullable<ReturnType<typeof buildGatewayUsageLoadPlan>['invalidRangeNotice']>) => void
  showPlanNotice: (plan: Extract<ReturnType<typeof buildGatewayUsageLoadErrorPlan>, { showError: true }>) => void
  isAbortError: (error: unknown) => boolean
}

type RefreshGatewayRealtimeDataOptions<
  TOverview,
  TInputRoute,
  TOutputRoute,
  TLog,
  TController extends AbortControllerLike = AbortController,
> = {
  now: number
  visible: boolean
  isMonitor: boolean
  logsDrawerOpen: boolean
  includeDisabled: boolean
  mounted: () => boolean
  priorityDialogOpen: () => boolean
  startAutoRefresh: (options: RefreshStartOptions) => boolean
  finishAutoRefresh: () => void
  controllerSlot: RuntimeControllerSlot<TController>
  requestOverview: (options: { signal: TController['signal'] }) => Promise<TOverview>
  requestRoutes: (options: { includeDisabled: boolean; signal: TController['signal'] }) => Promise<TInputRoute[]>
  requestLogs: (limit: number, options: { signal: TController['signal'] }) => Promise<TLog[]>
  currentLogs: () => TLog[]
  normalizeRoute: (route: TInputRoute) => TOutputRoute
  setOverview: (overview: TOverview) => void
  setRoutes: (routes: TOutputRoute[]) => void
  setPriorityRoutes: (routes: TOutputRoute[]) => void
  setLogs: (logs: TLog[]) => void
  refreshActiveRequests: (silent: true) => Promise<void>
  isAbortError: (error: unknown) => boolean
}

export function createReloadGatewayDataAfterAction({
  loadData,
}: {
  loadData: () => Promise<void>
}) {
  return () => loadData()
}

export function createLoadGatewayUsageRuntimeAction<TUsage, TController extends AbortControllerLike = AbortController>(
  options: Omit<LoadGatewayUsageOptions<TUsage, TController>, 'silent' | 'isMonitor' | 'requestRange'> & {
  loadUsage: (options: LoadGatewayUsageOptions<TUsage, TController>) => Promise<void>
  isMonitor: () => boolean
  getRequestRange: () => GatewayUsageRequestRange
  },
) {
  return (silent = false) =>
    options.loadUsage({
      ...options,
      silent,
      isMonitor: options.isMonitor(),
      requestRange: options.getRequestRange(),
    })
}

export async function loadGatewayActiveRequests<TSnapshot, TController extends AbortControllerLike = AbortController>({
  silent,
  mounted,
  controllerSlot,
  requestActiveRequests,
  setActiveRequests,
  applyActiveRequestSnapshot,
  showPlanNotice,
}: LoadGatewayActiveRequestsOptions<TSnapshot, TController>) {
  const controller = controllerSlot.replace()
  try {
    const snapshot = await requestActiveRequests({ signal: controller.signal })
    const resultPlan = buildGatewayActiveRequestsLoadResultPlan({
      mounted: mounted(),
      aborted: controller.signal.aborted,
    })
    if (!resultPlan.applySnapshot) {
      return
    }
    setActiveRequests(snapshot)
    applyActiveRequestSnapshot(snapshot)
  } catch (err) {
    const errorPlan = buildGatewayActiveRequestsLoadErrorPlan({
      aborted: controller.signal.aborted,
      mounted: mounted(),
      silent,
      errorMessage: err instanceof Error ? err.message : null,
    })
    if (errorPlan.showError) {
      showPlanNotice(errorPlan)
    }
  } finally {
    controllerSlot.clearIfCurrent(controller)
  }
}

export async function loadGatewayUsage<TUsage, TController extends AbortControllerLike = AbortController>({
  silent,
  isMonitor,
  requestRange,
  mounted,
  controllerSlot,
  requestUsage,
  setUsage,
  setUsageLoading,
  showNotice,
  showPlanNotice,
  isAbortError,
}: LoadGatewayUsageOptions<TUsage, TController>) {
  const usagePlan = buildGatewayUsageLoadPlan({
    isMonitor,
    start: requestRange.start,
    end: requestRange.end,
    silent,
  })
  if (usagePlan.clearUsage) {
    setUsage(null)
    return
  }
  if (!usagePlan.shouldLoad) {
    const notice = usagePlan.invalidRangeNotice
    if (usagePlan.showInvalidRangeError && notice) {
      showNotice(notice)
    }
    return
  }

  const controller = controllerSlot.replace()
  setUsageLoading(true)
  try {
    const usage = await requestUsage({
      ...usagePlan.requestRange,
      signal: controller.signal,
    })
    const resultPlan = buildGatewayUsageLoadResultPlan({
      mounted: mounted(),
      aborted: controller.signal.aborted,
    })
    if (resultPlan.applyUsage) {
      setUsage(usage)
    }
  } catch (err) {
    const errorPlan = buildGatewayUsageLoadErrorPlan({
      aborted: isAbortError(err),
      mounted: mounted(),
      silent,
      errorMessage: err instanceof Error ? err.message : null,
    })
    if (errorPlan.showError) {
      showPlanNotice(errorPlan)
    }
  } finally {
    controllerSlot.clearIfCurrent(controller)
    if (mounted()) {
      setUsageLoading(false)
    }
  }
}

export async function refreshGatewayRealtimeData<
  TOverview,
  TInputRoute,
  TOutputRoute,
  TLog,
  TController extends AbortControllerLike = AbortController,
>({
  now,
  visible,
  isMonitor,
  logsDrawerOpen,
  includeDisabled,
  mounted,
  priorityDialogOpen,
  startAutoRefresh,
  finishAutoRefresh,
  controllerSlot,
  requestOverview,
  requestRoutes,
  requestLogs,
  currentLogs,
  normalizeRoute,
  setOverview,
  setRoutes,
  setPriorityRoutes,
  setLogs,
  refreshActiveRequests,
  isAbortError,
}: RefreshGatewayRealtimeDataOptions<TOverview, TInputRoute, TOutputRoute, TLog, TController>) {
  if (!startAutoRefresh({ now, visible })) {
    return
  }
  const controller = controllerSlot.replace()
  try {
    const refreshPlan = buildGatewayRealtimeRefreshPlan({
      isMonitor,
      logsDrawerOpen,
    })
    const [overviewData, routeData, logData] = await Promise.all([
      requestOverview({ signal: controller.signal }),
      requestRoutes({ includeDisabled, signal: controller.signal }),
      refreshPlan.loadLogs ? requestLogs(80, { signal: controller.signal }) : Promise.resolve(currentLogs()),
    ])
    const applyPlan = buildGatewayRealtimeRefreshApplyPlan({
      mounted: mounted(),
      aborted: controller.signal.aborted,
      priorityDialogOpen: priorityDialogOpen(),
      routes: routeData,
      normalizeRoute,
      refreshActiveRequests: refreshPlan.refreshActiveRequests,
    })
    if (!applyPlan.shouldApply) {
      return
    }
    setOverview(overviewData)
    setRoutes(applyPlan.normalizedRoutes)
    if (applyPlan.updatePriorityRoutes) {
      setPriorityRoutes(applyPlan.normalizedRoutes)
    }
    setLogs(logData)
    if (applyPlan.refreshActiveRequests) {
      await refreshActiveRequests(true)
    }
  } catch (err) {
    if (!isAbortError(err)) {
      // 自动刷新静默失败，避免请求波动时持续打扰。
    }
  } finally {
    controllerSlot.clearIfCurrent(controller)
    if (mounted()) {
      finishAutoRefresh()
    }
  }
}
