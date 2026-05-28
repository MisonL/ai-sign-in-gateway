import test from 'node:test'
import assert from 'node:assert/strict'
import { readFile } from 'node:fs/promises'

const componentPath = new URL('../src/components/settings/SettingsAccountTab.vue', import.meta.url)

test('settings account tab validates password confirmation before submit', async () => {
  const source = await readFile(componentPath, 'utf8')

  assert.match(source, /import \{ computed \} from 'vue'/)
  assert.match(source, /const isPasswordMismatch = computed\(\(\) =>/)
  assert.match(source, /view\.accountForm\.new_password\.length > 0/)
  assert.match(source, /view\.accountForm\.new_password !== props\.view\.accountForm\.confirm_password/)
  assert.match(source, /:validate-status="isPasswordMismatch \? 'error' : undefined"/)
  assert.match(source, /:help="isPasswordMismatch \? '两次输入的新密码不一致。' : undefined"/)
  assert.match(source, /:disabled="isPasswordMismatch"/)
})
