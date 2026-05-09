import type { GatewayActiveRequest, GatewayRoute } from './types'

type RouteConcurrencyTarget = Pick<GatewayRoute, 'id' | 'active_concurrency'>
type ActiveRouteSnapshot = Pick<GatewayActiveRequest, 'route_id'>

function countActiveRequestsByRoute(activeRequests: ActiveRouteSnapshot[]) {
  const counts = new Map<number, number>()
  for (const item of activeRequests) {
    if (!Number.isFinite(item.route_id) || item.route_id <= 0) {
      continue
    }
    counts.set(item.route_id, (counts.get(item.route_id) ?? 0) + 1)
  }
  return counts
}

export function applyGatewayActiveConcurrency<T extends RouteConcurrencyTarget>(
  routes: T[],
  activeRequests: ActiveRouteSnapshot[],
): T[] {
  const counts = countActiveRequestsByRoute(activeRequests)
  let changed = false
  const next = routes.map((route) => {
    const activeConcurrency = counts.get(route.id) ?? 0
    if (route.active_concurrency === activeConcurrency) {
      return route
    }
    changed = true
    return {
      ...route,
      active_concurrency: activeConcurrency,
    }
  })
  return changed ? next : routes
}
