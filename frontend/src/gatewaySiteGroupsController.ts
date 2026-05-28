import type { SiteGroup } from './types.ts'

export type RefreshGatewaySiteGroupsOptions = {
  requestSiteGroups: () => Promise<SiteGroup[]>
  setSiteGroups: (groups: SiteGroup[]) => void
}

export function createRefreshGatewaySiteGroupsAction({
  requestSiteGroups,
  setSiteGroups,
}: RefreshGatewaySiteGroupsOptions) {
  return () =>
    refreshGatewaySiteGroups({
      requestSiteGroups,
      setSiteGroups,
    })
}

export async function refreshGatewaySiteGroups({
  requestSiteGroups,
  setSiteGroups,
}: RefreshGatewaySiteGroupsOptions) {
  try {
    setSiteGroups(await requestSiteGroups())
  } catch {
    // Header group changes are best-effort; keep current options on refresh failure.
  }
}
