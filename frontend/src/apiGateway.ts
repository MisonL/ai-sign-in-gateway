import { request, type GatewayLogStatusFilter, type RequestOptions } from './apiCore'
import type {
  BalanceProbeResult,
  GatewayActiveRequest,
  GatewayLog,
  GatewayOverview,
  GatewayRoute,
  GatewayRouteDiagnosis,
  GatewayRouteProbeResult,
  GatewayRouteUpdatePayload,
  GatewaySettingsData,
  GatewayUsage,
} from './types'

export function getGatewayOverview(options: RequestOptions = {}): Promise<GatewayOverview> {
  return request('/gateway-admin/overview', { signal: options.signal })
}

export function getGatewayUsage(options?: { start?: string; end?: string; signal?: AbortSignal }): Promise<GatewayUsage> {
  const params = new URLSearchParams()
  if (options?.start) {
    params.set('start', options.start)
  }
  if (options?.end) {
    params.set('end', options.end)
  }
  const suffix = params.toString() ? `?${params.toString()}` : ''
  return request(`/gateway-admin/usage${suffix}`, { signal: options?.signal })
}

export function getGatewaySettings(options: RequestOptions = {}): Promise<GatewaySettingsData> {
  return request('/gateway-admin/settings', { signal: options.signal })
}

export function updateGatewaySettings(payload: GatewaySettingsData): Promise<GatewaySettingsData> {
  return request('/gateway-admin/settings', {
    method: 'PUT',
    body: JSON.stringify(payload),
  })
}

export function syncGatewayRoutes(): Promise<{ status: string; route_count: number }> {
  return request('/gateway-admin/sync', {
    method: 'POST',
  })
}

export function getGatewayRoutes(options?: { group?: string; includeDisabled?: boolean; signal?: AbortSignal }): Promise<GatewayRoute[]> {
  const params = new URLSearchParams()
  if (options?.group) {
    params.set('group', options.group)
  }
  if (options?.includeDisabled !== undefined) {
    params.set('include_disabled', String(options.includeDisabled))
  }
  const suffix = params.toString() ? `?${params.toString()}` : ''
  return request(`/gateway-admin/routes${suffix}`, { signal: options?.signal })
}

export function toggleGatewayRoute(id: number): Promise<{ id: number; is_enabled: boolean; is_enabled_manual?: boolean; circuit_state: string }> {
  return request(`/gateway-admin/routes/${id}/toggle`, {
    method: 'POST',
  })
}

export function disableAllGatewayRoutes(): Promise<{ status: string; disabled_count: number }> {
  return request('/gateway-admin/routes/disable-all', {
    method: 'POST',
  })
}

export function enableOnlyGatewayRoute(id: number): Promise<{ status: string; enabled_route_id: number }> {
  return request(`/gateway-admin/routes/${id}/enable-only`, {
    method: 'POST',
  })
}

export function resetGatewayRouteCircuit(id: number): Promise<{ id: number; is_enabled: boolean; circuit_state: string }> {
  return request(`/gateway-admin/routes/${id}/reset-circuit`, {
    method: 'POST',
  })
}

export function updateGatewayRouteType(
  id: number,
  payload: GatewayRouteUpdatePayload,
): Promise<GatewayRoute> {
  return request(`/gateway-admin/routes/${id}/type`, {
    method: 'PATCH',
    body: JSON.stringify(payload),
  })
}

export function reorderGatewayRoutePriorities(payload: {
  route_id?: number
  mode: 'move' | 'package' | 'balance'
  index?: number
}): Promise<GatewayRoute[]> {
  return request('/gateway-admin/routes/priorities/reorder', {
    method: 'POST',
    body: JSON.stringify(payload),
  })
}

export function probeGatewayRoute(id: number): Promise<GatewayRouteProbeResult> {
  return request(`/gateway-admin/routes/${id}/probe`, {
    method: 'POST',
  })
}

export function diagnoseGatewayRoute(id: number): Promise<GatewayRouteDiagnosis> {
  return request(`/gateway-admin/routes/${id}/diagnose`)
}

export function probeGatewayRouteBalance(id: number, payload?: { balance_probe_url?: string }): Promise<BalanceProbeResult> {
  return request(`/gateway-admin/routes/${id}/balance-probe`, {
    method: 'POST',
    body: payload ? JSON.stringify(payload) : undefined,
  })
}

export function probeGatewayRoutes(routeIds: number[]): Promise<GatewayRouteProbeResult[]> {
  return request('/gateway-admin/routes/probe', {
    method: 'POST',
    body: JSON.stringify({ route_ids: routeIds }),
  })
}

export function getGatewayLogs(limit = 80, options: RequestOptions & { status?: GatewayLogStatusFilter } = {}): Promise<GatewayLog[]> {
  const params = new URLSearchParams({ limit: String(limit) })
  if (options.status && options.status !== 'all') {
    params.set('status', options.status)
  }
  return request(`/gateway-admin/logs?${params.toString()}`, { signal: options.signal })
}

export function getGatewayActiveRequests(options: RequestOptions & { includeRecent?: boolean } = {}): Promise<GatewayActiveRequest[]> {
  const query = options.includeRecent ? '?include_recent=true' : ''
  return request(`/gateway-admin/active-requests${query}`, { signal: options.signal })
}

export function getGatewayRouteLogs(routeId: number, limit = 80, options: RequestOptions & { status?: GatewayLogStatusFilter } = {}): Promise<GatewayLog[]> {
  const params = new URLSearchParams({ limit: String(limit) })
  if (options.status && options.status !== 'all') {
    params.set('status', options.status)
  }
  return request(`/gateway-admin/routes/${routeId}/logs?${params.toString()}`, { signal: options.signal })
}
