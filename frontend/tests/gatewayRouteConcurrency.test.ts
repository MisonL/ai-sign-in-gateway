import test from 'node:test'
import assert from 'node:assert/strict'

import { applyGatewayActiveConcurrency } from '../src/gatewayRouteConcurrency.ts'
import type { GatewayActiveRequest, GatewayRoute } from '../src/types'

function route(id: number, activeConcurrency: number): GatewayRoute {
  return {
    id,
    site_id: id,
    site_name: `route-${id}`,
    base_url: '',
    request_base_url: '',
    group_name: '',
    supported_models: [],
    key_name: '',
    key_fingerprint: '',
    key_source: '',
    route_type: 'codex',
    route_priority: id,
    weight: 1,
    is_enabled: true,
    circuit_state: 'closed',
    consecutive_failures: 0,
    active_concurrency: activeConcurrency,
    request_count: 0,
    success_count: 0,
    failure_count: 0,
    avg_latency_ms: null,
    ewma_latency_ms: null,
    last_latency_ms: null,
    success_rate: 0,
    last_status_code: null,
    last_error: null,
    last_used_at: null,
    last_success_at: null,
    last_failure_at: null,
    circuit_open_until: null,
  }
}

function active(routeId: number): GatewayActiveRequest {
  return {
    id: `active-${routeId}-${Math.random()}`,
    request_id: 'request',
    route_id: routeId,
    site_id: routeId,
    route_label: `route-${routeId}`,
    site_name: `route-${routeId}`,
    key_name: '',
    key_fingerprint: '',
    group_name: '',
    target_path: 'chat/completions',
    method: 'POST',
    route_strategy: 'round_robin',
    attempt_index: 1,
    is_stream: false,
    route_type: 'codex',
    request_base_url: '',
    active_concurrency: 1,
    started_at: new Date().toISOString(),
    elapsed_ms: 0,
  }
}

test('syncs route concurrency from active request snapshots and clears stale counts', () => {
  const routes = [route(1, 2), route(2, 1)]

  const busy = applyGatewayActiveConcurrency(routes, [active(1), active(1), active(2)])
  assert.deepEqual(busy.map((item) => item.active_concurrency), [2, 1])

  const idle = applyGatewayActiveConcurrency(busy, [])
  assert.deepEqual(idle.map((item) => item.active_concurrency), [0, 0])
})
