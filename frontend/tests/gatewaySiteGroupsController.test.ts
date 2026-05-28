import test from 'node:test'
import assert from 'node:assert/strict'
import { readFile } from 'node:fs/promises'

import {
  createRefreshGatewaySiteGroupsAction,
  refreshGatewaySiteGroups,
} from '../src/gatewaySiteGroupsController.ts'
import type { SiteGroup } from '../src/types.ts'

const gatewayViewPath = new URL('../src/views/GatewayView.vue', import.meta.url)
const gatewayPageControllerPath = new URL('../src/gatewayPageController.ts', import.meta.url)
const gatewayPageOperationsControllerPath = new URL('../src/gatewayPageOperationsController.ts', import.meta.url)
const pageRefreshActionsControllerPath = new URL('../src/gatewayPageRefreshActionsController.ts', import.meta.url)
const gatewaySiteGroupsPageControllerPath = new URL(
  '../src/gatewaySiteGroupsPageController.ts',
  import.meta.url,
)
const catalogRefreshPageControllerPath = new URL('../src/gatewayCatalogRefreshPageController.ts', import.meta.url)
const refreshOperationsPageControllerPath = new URL(
  '../src/gatewayRefreshOperationsPageController.ts',
  import.meta.url,
)

function group(name: string): SiteGroup {
  return {
    name,
    site_count: 1,
    in_catalog: true,
    in_use: true,
  }
}

test('refreshGatewaySiteGroups loads site groups and replaces the current options', async () => {
  const events: string[] = []
  let currentGroups = [group('旧分组')]

  await refreshGatewaySiteGroups({
    requestSiteGroups: async () => {
      events.push('request')
      return [group('新分组')]
    },
    setSiteGroups: (groups) => {
      events.push(`set:${groups.map((item) => item.name).join(',')}`)
      currentGroups = groups
    },
  })

  assert.deepEqual(events, [
    'request',
    'set:新分组',
  ])
  assert.deepEqual(currentGroups.map((item) => item.name), ['新分组'])
})

test('refreshGatewaySiteGroups keeps existing options when loading fails', async () => {
  const events: string[] = []
  const currentGroups = [group('旧分组')]

  await refreshGatewaySiteGroups({
    requestSiteGroups: async () => {
      events.push('request')
      throw new Error('groups failed')
    },
    setSiteGroups: () => {
      events.push('set')
    },
  })

  assert.deepEqual(events, ['request'])
  assert.deepEqual(currentGroups.map((item) => item.name), ['旧分组'])
})

test('createRefreshGatewaySiteGroupsAction refreshes groups through injected dependencies', async () => {
  const events: string[] = []
  let requestCount = 0
  let currentGroups: SiteGroup[] = []

  const action = createRefreshGatewaySiteGroupsAction({
    requestSiteGroups: async () => {
      requestCount += 1
      events.push(`request:${requestCount}`)
      return [group(`分组 ${requestCount}`)]
    },
    setSiteGroups: (groups) => {
      events.push(`set:${groups.map((item) => item.name).join(',')}`)
      currentGroups = groups
    },
  })

  await action()
  await action()

  assert.deepEqual(events, [
    'request:1',
    'set:分组 1',
    'request:2',
    'set:分组 2',
  ])
  assert.deepEqual(currentGroups.map((item) => item.name), ['分组 2'])
})

test('GatewayView delegates site group changes through the site groups controller', async () => {
  const source = await readFile(gatewayPageControllerPath, 'utf8')
  const operationsControllerSource = await readFile(gatewayPageOperationsControllerPath, 'utf8')
  const pageRefreshActionsController = await readFile(pageRefreshActionsControllerPath, 'utf8')
  const pageController = await readFile(gatewaySiteGroupsPageControllerPath, 'utf8')
  const catalogController = await readFile(catalogRefreshPageControllerPath, 'utf8')
  const operationsController = await readFile(refreshOperationsPageControllerPath, 'utf8')
  const pageActionStart = operationsControllerSource.indexOf('useGatewayPageRefreshActions({')
  const pageAction = operationsControllerSource.slice(
    pageActionStart,
    operationsControllerSource.indexOf('  routeActions = useGatewayPageRouteActions({', pageActionStart),
  )

  assert.match(operationsControllerSource, /import \{ useGatewayPageRefreshActions \} from '\.\/gatewayPageRefreshActionsController\.ts'/)
  assert.doesNotMatch(source, /import \{ useGatewayRefreshOperationsPageActions \} from '(?:\.\.\/|\.\/)gatewayRefreshOperationsPageController(?:\.ts)?'/)
  assert.doesNotMatch(source, /import \{ useGatewayCatalogRefreshPageActions \} from '(?:\.\.\/|\.\/)gatewayCatalogRefreshPageController(?:\.ts)?'/)
  assert.doesNotMatch(source, /import \{ useGatewaySiteGroupsPageActions \} from '(?:\.\.\/|\.\/)gatewaySiteGroupsPageController(?:\.ts)?'/)
  assert.doesNotMatch(source, /useGatewayRefreshOperationsPageActions\(\{/)
  assert.doesNotMatch(source, /useGatewayCatalogRefreshPageActions\(\{/)
  assert.doesNotMatch(source, /useGatewaySiteGroupsPageActions\(\{/)
  assert.match(pageAction, /state/)
  assert.match(pageRefreshActionsController, /siteGroups: state\.siteGroups/)
  assert.match(pageRefreshActionsController, /requestSiteGroups: gatewayPageRequests\.getSiteGroups/)
  assert.match(operationsController, /useGatewayCatalogRefreshPageActions/)
  assert.match(catalogController, /useGatewaySiteGroupsPageActions/)
  assert.match(pageController, /createRefreshGatewaySiteGroupsAction/)
  assert.match(pageController, /requestSiteGroups/)
  assert.match(pageController, /setSiteGroups: \(groups\) => \{[\s\S]*siteGroups\.value = groups[\s\S]*\}/)
  assert.doesNotMatch(source, /createRefreshGatewaySiteGroupsAction/)
  assert.doesNotMatch(source, /siteGroups\.value = groups/)
  assert.doesNotMatch(source, /async function handleSiteGroupsChanged\(\)/)
  assert.doesNotMatch(pageAction, /refreshGatewaySiteGroups\(\{/)
  assert.doesNotMatch(pageAction, /try \{/)
  assert.doesNotMatch(pageAction, /catch/)
  assert.doesNotMatch(pageAction, /await getSiteGroups\(\)/)
})
