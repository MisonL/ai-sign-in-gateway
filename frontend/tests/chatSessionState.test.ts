import test from 'node:test'
import assert from 'node:assert/strict'

import {
  chatSessionMessageToView,
  chatSessionPreview,
  normalizeChatSessionTitle,
  viewMessageToChatSessionPayload,
} from '../src/chatSessionState.ts'
import type { ChatSessionMessage } from '../src/types.ts'

test('normalizes chat session title and preview text', () => {
  assert.equal(normalizeChatSessionTitle('  hello   world  '), 'hello world')
  assert.equal(chatSessionPreview(''), '暂无消息')
  assert.equal(normalizeChatSessionTitle(''), '新会话')
})

test('converts persisted chat messages to view state and append payload', () => {
  const stored: ChatSessionMessage = {
    id: 12,
    session_id: 3,
    seq: 2,
    role: 'assistant',
    content: 'Answer',
    status: 'done',
    mode: 'chat',
    latency_ms: 42,
    status_code: 200,
    error: '',
    reference_images: [{ name: 'ref.png', url: 'data:image/png;base64,abc' }],
    images: [],
    created_at: '2026-05-10T00:00:00Z',
    updated_at: '2026-05-10T00:00:01Z',
  }

  const view = chatSessionMessageToView(stored)
  assert.equal(view.id, 'stored-12')
  assert.equal(view.role, 'assistant')
  assert.equal(view.latencyMs, 42)
  assert.deepEqual(view.references, stored.reference_images)

  const payload = viewMessageToChatSessionPayload(view)
  assert.equal(payload.role, 'assistant')
  assert.equal(payload.status_code, 200)
  assert.deepEqual(payload.reference_images, stored.reference_images)
})
