import { request, type RequestOptions } from './apiCore'
import type { FeatureMeta, OverviewData, PluginMeta } from './types'

export function getOverview(options: RequestOptions = {}): Promise<OverviewData> {
  return request('/overview', { signal: options.signal })
}

export function getPlugins(): Promise<PluginMeta[]> {
  return request('/plugins')
}

export function getFeatures(options: RequestOptions = {}): Promise<FeatureMeta[]> {
  return request('/features', { signal: options.signal })
}
