import test from 'node:test'
import assert from 'node:assert/strict'
import { readFile } from 'node:fs/promises'

const sitesViewControllerPath = new URL('../src/composables/useSitesViewController.ts', import.meta.url)
const sitesApiKeyDialogPath = new URL('../src/composables/useSitesApiKeyDialog.ts', import.meta.url)
const sitesRuntimeChecksPath = new URL('../src/composables/useSitesRuntimeChecks.ts', import.meta.url)
const sitesInvitesPath = new URL('../src/composables/useSitesInvites.ts', import.meta.url)
const sitesCheckinConfigModalPath = new URL('../src/components/sites/SitesCheckinConfigModal.vue', import.meta.url)
const sitesApiKeyDialogComponentPath = new URL('../src/components/sites/SitesApiKeyDialog.vue', import.meta.url)
const sitesEditorCredentialsCardPath = new URL('../src/components/sites/SitesEditorCredentialsCard.vue', import.meta.url)

test('sites view controller reports site group reload errors from the event listener', async () => {
  const source = await readFile(sitesViewControllerPath, 'utf8')

  assert.match(source, /async function handleSiteGroupsChanged\(\)/)
  assert.match(source, /await loadData\(editingId\.value \?\? selectedId\.value, \{ preserveEditor: true \}\)/)
  assert.match(source, /catch \(err\)/)
  assert.match(source, /toast\.error\(err instanceof Error \? err\.message : '站点分组刷新失败'\)/)
})

test('sites api key dialog saves with object fallbacks for nullable config payloads', async () => {
  const source = await readFile(sitesApiKeyDialogPath, 'utf8')

  assert.match(source, /credentials: JSON\.parse\(JSON\.stringify\(site\.credentials \?\? \{\}\)\)/)
  assert.match(source, /plugin_config: JSON\.parse\(JSON\.stringify\(site\.plugin_config \?\? \{\}\)\)/)
  assert.match(source, /payload\.plugin_config\.api_request_urls = normalizeStringList/)
})

test('sites runtime checks prevent duplicate balance probes per site', async () => {
  const source = await readFile(sitesRuntimeChecksPath, 'utf8')

  assert.match(source, /if \(balanceProbeIds\.value\.includes\(site\.id\)\) \{\s+return\s+\}/)
  assert.match(source, /const nextProbeIds = \[\.\.\.balanceProbeIds\.value\]/)
  assert.match(source, /nextProbeIds\.splice\(probeIndex, 1\)/)
})

test('sites invites prevent duplicate invite loads per site', async () => {
  const source = await readFile(sitesInvitesPath, 'utf8')

  assert.match(source, /if \(inviteLoadingSiteIds\.value\.includes\(targetSite\.id\)\) \{\s+return\s+\}/)
  assert.match(source, /inviteLoadingSiteIds\.value = \[\.\.\.inviteLoadingSiteIds\.value, targetSite\.id\]/)
})

test('sites checkin config modal edits a local form and emits an explicit save payload', async () => {
  const source = await readFile(sitesCheckinConfigModalPath, 'utf8')

  assert.match(source, /const localForm = reactive<SettingsData>\(\{ \.\.\.props\.form \}\)/)
  assert.match(source, /Object\.assign\(localForm, value\)/)
  assert.match(source, /emit\('save', \{ \.\.\.localForm \}\)/)
  assert.match(source, /@ok="handleSave"/)
  assert.match(source, /v-model:value="localForm\.timezone"/)
  assert.doesNotMatch(source, /v-model:[^=]+="form\./)
})

test('sites api key dialog passes Ant Design password visibility prop in camelCase', async () => {
  const source = await readFile(sitesApiKeyDialogComponentPath, 'utf8')

  assert.match(source, /visibilityToggle/)
  assert.doesNotMatch(source, /visibility-toggle/)
})

test('sites editor totp textarea uses centralized autocomplete helper', async () => {
  const source = await readFile(sitesEditorCredentialsCardPath, 'utf8')

  assert.match(source, /:autocomplete="credentialAutocomplete\(field\.name, 'textarea'\)"/)
  assert.doesNotMatch(source, /autocomplete="off"/)
})
