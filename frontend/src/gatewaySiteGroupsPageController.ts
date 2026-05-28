import type { Ref } from 'vue'

import { createRefreshGatewaySiteGroupsAction } from './gatewaySiteGroupsController.ts'
import type { SiteGroup } from './types.ts'

type GatewaySiteGroupsPageOptions = {
  siteGroups: Ref<SiteGroup[]>
  requestSiteGroups: () => Promise<SiteGroup[]>
}

export function useGatewaySiteGroupsPageActions({
  siteGroups,
  requestSiteGroups,
}: GatewaySiteGroupsPageOptions) {
  const handleSiteGroupsChanged = createRefreshGatewaySiteGroupsAction({
    requestSiteGroups,
    setSiteGroups: (groups) => {
      siteGroups.value = groups
    },
  })

  return {
    handleSiteGroupsChanged,
  }
}
