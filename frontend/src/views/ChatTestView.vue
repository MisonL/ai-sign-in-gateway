<script setup lang="ts">
import {
  DeleteOutlined,
  ClearOutlined,
  DownOutlined,
  LockOutlined,
  PlusOutlined,
  PaperClipOutlined,
  ReloadOutlined,
  SendOutlined,
  UnlockOutlined,
} from '@ant-design/icons-vue'
import { computed, nextTick, onBeforeUnmount, onMounted, reactive, ref, type ComponentPublicInstance } from 'vue'
import {
  ApiError,
  appendChatSessionMessages,
  createChatSession,
  deleteChatSession,
  getChatSession,
  getSites,
  listChatSessions,
  listToolModels,
  testChat,
  updateChatSession,
} from '../api'
import ShellLayout from '../components/ShellLayout.vue'
import { useToast } from '../toast'
import {
  chatSessionMessageToView,
  chatSessionPreview,
  normalizeChatSessionTitle,
  viewMessageToChatSessionPayload,
} from '../chatSessionState'
import type { ChatImageReference, ChatRequestMessage, ChatResult, ChatSession, ModelListItem, Site } from '../types'
import '../styles/workspace-surfaces.css'

type MessageRole = 'system' | 'user' | 'assistant'
type MessageStatus = 'idle' | 'sending' | 'done' | 'error'
type ChatMode = 'chat' | 'image'

interface ChatActivity {
  label: string
  active: boolean
}

interface ChatMessage {
  id: string
  role: MessageRole
  content: string
  createdAt: string
  status: MessageStatus
  latencyMs?: number | null
  statusCode?: number | null
  error?: string
  references?: ChatImageReference[]
  images?: ChatImageReference[]
  mode?: ChatMode
  activity?: ChatActivity
}

const form = reactive({
  input: '',
  model_key: undefined as string | undefined,
  image_size: '1024x1024',
  image_width: 1024,
  image_height: 1024,
})

const toast = useToast()
const sites = ref<Site[]>([])
const selectedSiteId = ref<string>()
const loading = ref(false)
const modelsLoading = ref(false)
const sessionsLoading = ref(false)
const restoringSession = ref(false)
const deletingSessionIds = ref<number[]>([])
const modelItems = ref<ModelListItem[]>([])
const modelLoadMessage = ref('')
const modelLoadError = ref(false)
const messages = ref<ChatMessage[]>([])
const chatSessions = ref<ChatSession[]>([])
const activeSessionId = ref<number | null>(null)
const referenceImages = ref<ChatImageReference[]>([])
const fileInput = ref<HTMLInputElement | null>(null)
const scrollBody = ref<HTMLElement | null>(null)
const imageRatioLocked = ref(false)
const imageAspectRatio = ref(1)
const detectedImageRatio = ref('1:1')
const lockedImageRatio = ref('')
const activityTimers = new Map<string, number>()
let siteModelLoadRequest = 0
const maxReferenceImages = 5

const selectedSite = computed(() =>
  sites.value.find((item) => String(item.id) === selectedSiteId.value) ?? null,
)

const siteOptions = computed(() =>
  sites.value.map((site) => ({
    label: `${site.name} / ${site.plugin_key}`,
    value: String(site.id),
  })),
)

const visibleMessages = computed(() => messages.value.filter((item) => item.role !== 'system'))
const modelOptions = computed(() =>
  modelItems.value.map((model) => ({
    label: modelOptionLabel(model),
    value: modelOptionValue(model),
  })),
)
const selectedModel = computed(() => modelItems.value.find((model) => modelOptionValue(model) === form.model_key) ?? null)
const activeMode = computed(() => selectedModel.value?.mode === 'image' ? 'image' : 'chat')
const sendPlaceholder = computed(() =>
  activeMode.value === 'image' ? '描述你想生成的图片，或结合参考图说明要保留和改变的部分。' : '输入消息，Enter 发送，Shift + Enter 换行。',
)
const selectedModelMeta = computed(() => {
  const model = selectedModel.value
  if (!model) {
    return ''
  }
  return [routeTypeLabel(model.route_type), model.key_name || shortFingerprint(model.key_fingerprint), model.base_url]
    .filter(Boolean)
    .join(' / ')
})
const modelLoadAlertType = computed(() => (modelLoadError.value ? 'error' : 'info'))

const imageRatioPresets = [
  { label: '1:1', width: 1, height: 1 },
  { label: '3:4', width: 3, height: 4 },
  { label: '4:3', width: 4, height: 3 },
  { label: '16:9', width: 16, height: 9 },
  { label: '9:16', width: 9, height: 16 },
]
const imageRatioTooltip = computed(() => (imageRatioLocked.value ? `已锁定 ${form.image_width}:${form.image_height}` : '锁定当前宽高比'))
const activeImageRatio = computed(() => (imageRatioLocked.value ? lockedImageRatio.value : detectedImageRatio.value))
const activeSession = computed(() => chatSessions.value.find((item) => item.id === activeSessionId.value) ?? null)

function newID(prefix = 'msg') {
  return `${prefix}-${Date.now()}-${Math.random().toString(36).slice(2, 8)}`
}

function modelOptionValue(model: ModelListItem) {
  return `${model.id}\u0000${model.route_type}\u0000${model.key_fingerprint}\u0000${model.base_url}`
}

function modelOptionLabel(model: ModelListItem) {
  const mode = model.mode === 'image' ? '图片' : '对话'
  const key = model.key_name || shortFingerprint(model.key_fingerprint)
  return [model.id, mode, routeTypeLabel(model.route_type), key].filter(Boolean).join(' / ')
}

function routeTypeLabel(routeType: string) {
  if (routeType === 'claude') {
    return 'Claude'
  }
  if (routeType === 'gpt') {
    return 'GptChat'
  }
  if (routeType === 'codex') {
    return 'Codex Responses'
  }
  if (routeType === 'gemini') {
    return 'Gemini'
  }
  return 'OpenAI'
}

function shortFingerprint(value: string) {
  return value ? `Key ${value.slice(0, 8)}` : ''
}

function formatSessionTime(value: string) {
  if (!value) return ''
  return new Date(value).toLocaleString('zh-CN', {
    month: '2-digit',
    day: '2-digit',
    hour: '2-digit',
    minute: '2-digit',
  })
}

function clampImageDimension(value: unknown) {
  const parsed = Number(value)
  if (!Number.isFinite(parsed)) {
    return 100
  }
  return Math.min(4096, Math.max(100, Math.round(parsed)))
}

function syncImageSizeFromDimensions() {
  form.image_width = clampImageDimension(form.image_width)
  form.image_height = clampImageDimension(form.image_height)
  form.image_size = `${form.image_width}x${form.image_height}`
}

function gcd(a: number, b: number): number {
  a = Math.abs(Math.round(a))
  b = Math.abs(Math.round(b))
  while (b) {
    const next = a % b
    a = b
    b = next
  }
  return a || 1
}

function normalizedRatioLabel(width: number, height: number) {
  width = clampImageDimension(width)
  height = clampImageDimension(height)
  const divisor = gcd(width, height)
  return `${Math.round(width / divisor)}:${Math.round(height / divisor)}`
}

function detectImageRatio() {
  const label = normalizedRatioLabel(form.image_width, form.image_height)
  const preset = imageRatioPresets.find((item) => item.label === label)
  detectedImageRatio.value = preset ? preset.label : ''
}

function applyImageRatioPreset(preset: (typeof imageRatioPresets)[number]) {
  const ratioUnit = 100
  form.image_width = clampImageDimension(preset.width * ratioUnit)
  form.image_height = clampImageDimension(preset.height * ratioUnit)
  imageAspectRatio.value = preset.width / preset.height
  imageRatioLocked.value = true
  lockedImageRatio.value = preset.label
  detectedImageRatio.value = preset.label
  syncImageSizeFromDimensions()
}

function toggleImageRatioLock() {
  if (!imageRatioLocked.value) {
    syncImageSizeFromDimensions()
    imageAspectRatio.value = form.image_width / form.image_height
    lockedImageRatio.value = detectedImageRatio.value
  } else {
    lockedImageRatio.value = ''
  }
  imageRatioLocked.value = !imageRatioLocked.value
  detectImageRatio()
}

function handleImageWidthChange(value: unknown) {
  form.image_width = clampImageDimension(value)
  if (imageRatioLocked.value) {
    form.image_height = clampImageDimension(form.image_width / imageAspectRatio.value)
  }
  syncImageSizeFromDimensions()
  detectImageRatio()
  if (imageRatioLocked.value) {
    lockedImageRatio.value = detectedImageRatio.value
  }
}

function handleImageHeightChange(value: unknown) {
  form.image_height = clampImageDimension(value)
  if (imageRatioLocked.value) {
    form.image_width = clampImageDimension(form.image_height * imageAspectRatio.value)
  }
  syncImageSizeFromDimensions()
  detectImageRatio()
  if (imageRatioLocked.value) {
    lockedImageRatio.value = detectedImageRatio.value
  }
}

function chooseDefaultModel(items: ModelListItem[]) {
  return (
    items.find((item) => item.id === 'gpt-4o-mini') ??
    items.find((item) => item.id === 'gpt-image-2') ??
    items.find((item) => item.mode !== 'image') ??
    items[0] ??
    null
  )
}

async function applySelectedSite(preferredModel?: Pick<ModelListItem, 'id' | 'route_type' | 'key_fingerprint'>) {
  const requestID = ++siteModelLoadRequest
  const site = selectedSite.value
  modelItems.value = []
  form.model_key = undefined
  modelLoadMessage.value = ''
  modelLoadError.value = false
  if (!site) {
    modelsLoading.value = false
    return
  }
  modelsLoading.value = true
  try {
    const result = await listToolModels(Number(site.id))
    if (requestID !== siteModelLoadRequest || String(site.id) !== selectedSiteId.value) {
      return
    }
    modelItems.value = result.items ?? []
    modelLoadMessage.value = modelListMessage(result.message, result.status_code)
    const restored = preferredModel
      ? modelItems.value.find((item) =>
        item.id === preferredModel.id &&
        item.route_type === preferredModel.route_type &&
        item.key_fingerprint === preferredModel.key_fingerprint)
      : null
    const preferred = chooseDefaultModel(modelItems.value)
    form.model_key = restored
      ? modelOptionValue(restored)
      : (preferred ? modelOptionValue(preferred) : undefined)
    if (!result.ok) {
      modelLoadError.value = true
      toast.error(modelLoadMessage.value || '模型列表加载失败')
    }
  } catch (err) {
    if (requestID !== siteModelLoadRequest) {
      return
    }
    modelLoadError.value = true
    modelLoadMessage.value = modelListExceptionMessage(err)
    toast.error(modelLoadMessage.value)
  } finally {
    if (requestID === siteModelLoadRequest) {
      modelsLoading.value = false
    }
  }
}

function handleSiteChange() {
  void applySelectedSite()
}

function modelListMessage(message: string, statusCode?: number | null) {
  const text = message || '模型列表加载失败'
  if (statusCode === 404) {
    return `${text}。上游模型列表接口返回 404，请检查站点的 API 请求 URL 是否是模型请求根地址，例如 https://example.com/v1 或 https://example.com。`
  }
  return text
}

function modelListExceptionMessage(err: unknown) {
  if (err instanceof ApiError && err.status === 404) {
    return '模型列表接口 404：当前运行的后端尚未包含 /api/tools/models，或前端 API_BASE 指向了旧实例。请重启最新后端/二进制后再试。'
  }
  return err instanceof Error ? err.message : '模型列表加载失败'
}

function setScrollBody(element: Element | ComponentPublicInstance | null) {
  scrollBody.value = element instanceof HTMLElement ? element : null
}

async function scrollToBottom() {
  await nextTick()
  if (scrollBody.value) {
    scrollBody.value.scrollTop = scrollBody.value.scrollHeight
  }
}

function readableLatency(value: number | null | undefined) {
  return value === null || value === undefined ? '' : `${Math.round(value)} ms`
}

function imageSource(image: ChatImageReference) {
  return image.url
}

function triggerImagePicker() {
  if (activeMode.value !== 'image') {
    toast.error('当前模型不支持图片输入，请选择图片生成模型后再添加参考图。')
    return
  }
  fileInput.value?.click()
}

function fileToDataURL(file: File) {
  return new Promise<string>((resolve, reject) => {
    const reader = new FileReader()
    reader.onload = () => resolve(String(reader.result || ''))
    reader.onerror = () => reject(reader.error ?? new Error('图片读取失败'))
    reader.readAsDataURL(file)
  })
}

function addReferenceImages(event: Event) {
  const input = event.target as HTMLInputElement
  if (activeMode.value !== 'image') {
    input.value = ''
    toast.error('当前模型不支持图片输入，请选择图片生成模型后再添加参考图。')
    return
  }
  const files = Array.from(input.files ?? [])
  input.value = ''
  const imageFiles = files.filter((file) => file.type.startsWith('image/'))
  if (imageFiles.length !== files.length) {
    toast.error('只能添加图片文件。')
  }
  const remaining = Math.max(0, maxReferenceImages - referenceImages.value.length)
  imageFiles.slice(0, remaining).forEach((file) => {
    void fileToDataURL(file)
      .then((url) => {
        referenceImages.value.push({ name: file.name, url })
      })
      .catch((err) => toast.error(err instanceof Error ? err.message : '图片读取失败'))
  })
  if (imageFiles.length > remaining) {
    toast.info(`最多保留 ${maxReferenceImages} 张参考图。`)
  }
}

function removeReferenceImage(index: number) {
  referenceImages.value.splice(index, 1)
}

function clearConversation() {
  stopAllActivityTimers()
  messages.value = []
  referenceImages.value = []
}

function currentSessionPayload(title?: string) {
  const model = selectedModel.value
  const site = selectedSite.value
  return {
    title: normalizeChatSessionTitle(title ?? messages.value.find((item) => item.role === 'user' && item.content.trim())?.content ?? form.input),
    site_id: selectedSiteId.value ? Number(selectedSiteId.value) : null,
    site_name: site?.name ?? '',
    model: model?.id ?? '',
    mode: activeMode.value,
    route_type: model?.route_type ?? '',
    key_fingerprint: model?.key_fingerprint ?? '',
    key_name: model?.key_name ?? '',
    image_size: form.image_size,
    image_width: form.image_width,
    image_height: form.image_height,
  }
}

async function loadChatSessions() {
  sessionsLoading.value = true
  try {
    const result = await listChatSessions(80)
    chatSessions.value = result.items
    if (activeSessionId.value && !chatSessions.value.some((item) => item.id === activeSessionId.value)) {
      activeSessionId.value = null
    }
  } catch (err) {
    toast.error(err instanceof Error ? err.message : '会话历史加载失败')
  } finally {
    sessionsLoading.value = false
  }
}

async function ensureActiveSession(titleSeed: string) {
  if (activeSessionId.value) {
    return activeSessionId.value
  }
  const session = await createChatSession(currentSessionPayload(titleSeed))
  activeSessionId.value = session.id
  chatSessions.value = [session, ...chatSessions.value.filter((item) => item.id !== session.id)]
  return session.id
}

async function refreshActiveSessionMeta() {
  if (!activeSessionId.value) return
  try {
    const session = await updateChatSession(activeSessionId.value, currentSessionPayload(activeSession.value?.title))
    chatSessions.value = [session, ...chatSessions.value.filter((item) => item.id !== session.id)]
  } catch {
    await loadChatSessions()
  }
}

function startNewSession() {
  stopAllActivityTimers()
  activeSessionId.value = null
  messages.value = []
  referenceImages.value = []
  form.input = ''
}

async function restoreChatSession(id: number) {
  if (loading.value) {
    toast.error('请求发送中，稍后再切换会话。')
    return
  }
  restoringSession.value = true
  try {
    const detail = await getChatSession(id)
    stopAllActivityTimers()
    activeSessionId.value = detail.id
    messages.value = detail.messages.map((item) => chatSessionMessageToView(item))
    referenceImages.value = []
    if (detail.image_width > 0) form.image_width = detail.image_width
    if (detail.image_height > 0) form.image_height = detail.image_height
    syncImageSizeFromDimensions()
    detectImageRatio()
    if (detail.site_id) {
      selectedSiteId.value = String(detail.site_id)
      const preferredModel = detail.model
        ? {
          id: detail.model,
          mode: detail.mode || 'chat',
          route_type: detail.route_type,
          key_fingerprint: detail.key_fingerprint,
          key_name: detail.key_name,
          base_url: '',
        }
        : undefined
      await applySelectedSite(preferredModel)
    }
    await scrollToBottom()
  } catch (err) {
    toast.error(err instanceof Error ? err.message : '会话恢复失败')
  } finally {
    restoringSession.value = false
  }
}

async function removeChatSession(session: ChatSession) {
  if (loading.value && activeSessionId.value === session.id) {
    toast.error('当前会话请求发送中，稍后再删除。')
    return
  }
  deletingSessionIds.value = [...deletingSessionIds.value, session.id]
  try {
    await deleteChatSession(session.id)
    chatSessions.value = chatSessions.value.filter((item) => item.id !== session.id)
    if (activeSessionId.value === session.id) {
      startNewSession()
    }
    toast.success('会话已删除。')
  } catch (err) {
    toast.error(err instanceof Error ? err.message : '会话删除失败')
  } finally {
    deletingSessionIds.value = deletingSessionIds.value.filter((id) => id !== session.id)
  }
}

function toRequestMessages(): ChatRequestMessage[] {
  return messages.value
    .filter((item) => item.role === 'user' || item.role === 'assistant')
    .filter((item) => item.status === 'done' && (item.content.trim() || item.references?.length))
    .map((item) => ({
      role: item.role,
      content: item.content,
      reference_images: item.references ?? [],
    }))
}

function resultImages(result: ChatResult): ChatImageReference[] {
  return (result.images ?? [])
    .map((item, index) => ({
      name: `生成图 ${index + 1}`,
      url: item.url || (item.b64_json ? `data:image/png;base64,${item.b64_json}` : ''),
    }))
    .filter((item) => item.url)
}

function activitySteps(mode: ChatMode, refs: ChatImageReference[]) {
  if (mode === 'image') {
    return refs.length ? ['Read', 'Edit', 'Generate'] : ['Thinking', 'Generate']
  }
  return refs.length ? ['Read', 'Thinking', 'Respond'] : ['Thinking', 'Respond']
}

function stopActivityTimer(messageID: string) {
  const timer = activityTimers.get(messageID)
  if (timer !== undefined) {
    window.clearInterval(timer)
    activityTimers.delete(messageID)
  }
}

function stopAllActivityTimers() {
  activityTimers.forEach((timer) => window.clearInterval(timer))
  activityTimers.clear()
}

function startMessageActivity(message: ChatMessage, steps: string[]) {
  stopActivityTimer(message.id)
  const labels = steps.length ? steps : ['Thinking']
  let index = 0
  message.activity = { label: labels[index], active: true }
  if (labels.length <= 1) {
    return
  }
  activityTimers.set(message.id, window.setInterval(() => {
    index = (index + 1) % labels.length
    message.activity = { label: labels[index], active: true }
  }, 1800))
}

function finishMessageActivity(message: ChatMessage, label: string) {
  stopActivityTimer(message.id)
  message.activity = { label, active: false }
}

async function sendMessage() {
  const content = form.input.trim()
  if (!content && !referenceImages.value.length) {
    toast.error('请输入消息或添加参考图。')
    return
  }
  const model = selectedModel.value
  if (!selectedSiteId.value || !model) {
    toast.error('请先选择站点和模型。')
    return
  }
  const siteID = Number(selectedSiteId.value)
  const requestMode: ChatMode = model.mode === 'image' ? 'image' : 'chat'
  if (requestMode !== 'image' && referenceImages.value.length) {
    toast.error('当前模型不支持图片输入，请移除参考图或切换到图片生成模型。')
    return
  }
  const imageSize = form.image_size
  const refs = referenceImages.value.map((item) => ({ ...item }))
  let sessionID: number
  try {
    sessionID = await ensureActiveSession(content || '图片会话')
    await refreshActiveSessionMeta()
  } catch (err) {
    toast.error(err instanceof Error ? err.message : '会话创建失败')
    return
  }
  const userMessage: ChatMessage = {
    id: newID('user'),
    role: 'user',
    content,
    references: refs,
    createdAt: new Date().toISOString(),
    status: 'done',
  }
  const assistantMessage = reactive<ChatMessage>({
    id: newID('assistant'),
    role: 'assistant',
    content: requestMode === 'image' ? '正在生成图片...' : '正在思考...',
    createdAt: new Date().toISOString(),
    status: 'sending',
    mode: requestMode,
  })

  messages.value.push(userMessage, assistantMessage)
  const requestMessages = requestMode === 'chat' ? toRequestMessages() : undefined
  startMessageActivity(assistantMessage, activitySteps(requestMode, refs))
  form.input = ''
  referenceImages.value = []
  await scrollToBottom()
  loading.value = true

  try {
    const result = await testChat({
      site_id: siteID,
      route_type: model.route_type,
      key_fingerprint: model.key_fingerprint,
      model: model.id,
      mode: 'auto',
      prompt: content,
      messages: requestMessages,
      reference_images: refs,
      image_size: requestMode === 'image' ? imageSize : undefined,
      image_generation_path: requestMode === 'image' ? model.image_generation_path : undefined,
      image_edit_path: requestMode === 'image' ? model.image_edit_path : undefined,
    })
    assistantMessage.status = result.ok ? 'done' : 'error'
    assistantMessage.statusCode = result.status_code
    assistantMessage.latencyMs = result.latency_ms
    assistantMessage.error = result.ok ? undefined : result.message
    if (requestMode === 'image') {
      assistantMessage.images = resultImages(result)
      assistantMessage.content = assistantMessage.images.length
        ? (result.revised_prompt ? `已生成图片。优化提示词：${result.revised_prompt}` : '已生成图片。')
        : result.message
    } else {
      assistantMessage.content = result.output || result.message || '接口未返回可读文本。'
    }
    finishMessageActivity(assistantMessage, result.ok ? 'Done' : 'Error')
    toast.success(result.ok ? '请求完成。' : result.message)
  } catch (err) {
    assistantMessage.status = 'error'
    assistantMessage.content = err instanceof Error ? err.message : '请求失败'
    assistantMessage.error = assistantMessage.content
    finishMessageActivity(assistantMessage, 'Error')
    toast.error(assistantMessage.content)
  } finally {
    try {
      const persisted = await appendChatSessionMessages(sessionID, {
        messages: [
          viewMessageToChatSessionPayload(userMessage),
          viewMessageToChatSessionPayload(assistantMessage),
        ],
      })
      const { messages: _messages, ...session } = persisted
      chatSessions.value = [session, ...chatSessions.value.filter((item) => item.id !== session.id)]
    } catch (err) {
      toast.error(err instanceof Error ? err.message : '会话保存失败')
    }
    loading.value = false
    await scrollToBottom()
  }
}

function handleEditorKeydown(event: KeyboardEvent) {
  if (event.key === 'Enter' && !event.shiftKey) {
    event.preventDefault()
    if (!loading.value) {
      void sendMessage()
    }
  }
}

async function loadSites() {
  try {
    sites.value = await getSites()
  } catch (err) {
    toast.error(err instanceof Error ? err.message : '站点加载失败')
  }
}

onMounted(() => {
  void loadSites()
  void loadChatSessions()
})

onBeforeUnmount(() => {
  stopAllActivityTimers()
})
</script>

<template>
  <ShellLayout class="session-layout-shell">
    <div class="chat-workbench">
      <main class="session-main">
        <div :ref="setScrollBody" class="session-history">
          <div v-if="visibleMessages.length" class="session-message-list">
            <article
              v-for="message in visibleMessages"
              :key="message.id"
              class="session-message"
              :class="[`session-message--${message.role}`, `session-message--${message.status}`]"
            >
              <div v-if="message.role === 'assistant' && message.activity" class="session-message__thought">
                处理耗时 {{ readableLatency(message.latencyMs) || '...' }}
              </div>

              <div class="session-message__bubble">
                <p v-if="message.content" class="session-message__text">{{ message.content }}</p>

                <div v-if="message.references?.length" class="session-reference-strip">
                  <div v-for="image in message.references" :key="image.url" class="session-reference-card">
                    <img :src="imageSource(image)" :alt="image.name" />
                  </div>
                </div>

                <div v-if="message.images?.length" class="session-generated-grid">
                  <a v-for="image in message.images" :key="image.url" :href="imageSource(image)" target="_blank" rel="noreferrer">
                    <img :src="imageSource(image)" :alt="image.name" />
                  </a>
                </div>

                <div v-if="message.error" class="session-message__error">{{ message.error }}</div>
              </div>
            </article>
          </div>

          <div v-else class="session-empty">
            <div class="session-empty__copy">
              <h1>选择站点与模型后开始使用</h1>
              <p>文本对话与图片生成能力将根据所选模型自动启用。</p>
            </div>
          </div>
        </div>

        <footer class="session-composer">
          <a-alert v-if="modelLoadMessage && (!selectedModelMeta || modelLoadError)" class="session-composer__alert" :type="modelLoadAlertType" show-icon :message="modelLoadMessage" />

          <div class="session-composer__controls">
            <a-select
              v-model:value="selectedSiteId"
              class="session-toolbar__select"
              :options="siteOptions"
              allow-clear
              show-search
              option-filter-prop="label"
              placeholder="选择站点"
              @change="handleSiteChange"
            >
              <template #suffixIcon><DownOutlined /></template>
            </a-select>

            <a-select
              v-model:value="form.model_key"
              class="session-toolbar__select session-toolbar__select--model"
              :options="modelOptions"
              :loading="modelsLoading"
              :disabled="!selectedSite || modelsLoading"
              allow-clear
              show-search
              option-filter-prop="label"
              placeholder="选择模型"
            >
              <template #suffixIcon><DownOutlined /></template>
            </a-select>

            <div v-if="activeMode === 'image'" class="image-size-editor session-composer__image-size">
              <div class="image-ratio-presets" aria-label="图片比例快捷设置">
                <button
                  v-for="preset in imageRatioPresets"
                  :key="preset.label"
                  type="button"
                  class="image-ratio-preset"
                  :class="{ 'is-active': activeImageRatio === preset.label, 'is-detected': !imageRatioLocked && detectedImageRatio === preset.label, 'is-locked': imageRatioLocked && lockedImageRatio === preset.label }"
                  @click="applyImageRatioPreset(preset)"
                >
                  <span class="image-ratio-preset__box" :style="{ aspectRatio: `${preset.width} / ${preset.height}` }"></span>
                  <span>{{ preset.label }}</span>
                </button>
              </div>
              <a-input-number :value="form.image_width" :min="100" :max="4096" :step="1" addon-before="宽" @change="handleImageWidthChange" />
              <a-input-number :value="form.image_height" :min="100" :max="4096" :step="1" addon-before="高" @change="handleImageHeightChange" />
              <a-tooltip :title="imageRatioTooltip">
                <a-button class="image-size-editor__lock" :type="imageRatioLocked ? 'primary' : 'default'" @click="toggleImageRatioLock">
                  <template #icon>
                    <LockOutlined v-if="imageRatioLocked" />
                    <UnlockOutlined v-else />
                  </template>
                </a-button>
              </a-tooltip>
            </div>

            <button type="button" class="session-clear-button" :disabled="!visibleMessages.length && !referenceImages.length" @click="clearConversation">
              <ClearOutlined />
              清空会话
            </button>
          </div>

          <div v-if="referenceImages.length" class="session-attachments">
            <div v-for="(image, index) in referenceImages" :key="image.url" class="session-attachment">
              <img :src="imageSource(image)" :alt="image.name" />
              <span>{{ image.name }}</span>
              <button type="button" title="移除参考图" @click="removeReferenceImage(index)">
                <DeleteOutlined />
              </button>
            </div>
          </div>

          <input id="chat-reference-images" ref="fileInput" class="chat-file-input" type="file" name="chat_reference_images" accept="image/*" multiple hidden tabindex="-1" @change="addReferenceImages" />

          <div class="session-composer__frame">
            <button type="button" class="session-tool-button" :disabled="loading" title="添加参考图" aria-label="添加参考图" @click="triggerImagePicker">
              <PaperClipOutlined />
            </button>

            <a-textarea id="chat-message-input" v-model:value="form.input" class="session-composer__input" name="chat_message_input" :rows="3" :placeholder="sendPlaceholder" @keydown="handleEditorKeydown" />

            <div class="session-composer__side">
              <button type="button" class="session-send-button" :disabled="loading" @click="sendMessage">
                <SendOutlined />
                <span>{{ loading ? '发送中' : '发送请求' }}</span>
              </button>
              <span class="session-footnote">{{ referenceImages.length }}/{{ maxReferenceImages }} 张参考图</span>
            </div>
          </div>
        </footer>
      </main>

      <aside class="session-sidebar">
        <div class="session-sidebar__header">
          <div>
            <strong>会话历史</strong>
            <span>{{ chatSessions.length }} 条记录</span>
          </div>
          <div class="session-sidebar__actions">
            <button type="button" title="新建会话" @click="startNewSession">
              <PlusOutlined />
            </button>
            <button type="button" title="刷新历史" :disabled="sessionsLoading" @click="loadChatSessions">
              <ReloadOutlined />
            </button>
          </div>
        </div>

        <div class="session-sidebar__list" :class="{ 'is-loading': sessionsLoading || restoringSession }">
          <div
            v-for="session in chatSessions"
            :key="session.id"
            class="session-history-item"
            :class="{ 'is-active': session.id === activeSessionId }"
          >
            <button type="button" class="session-history-item__main" @click="restoreChatSession(session.id)">
              <span class="session-history-item__title">{{ session.title }}</span>
              <span class="session-history-item__preview">{{ chatSessionPreview(session.last_message_text) }}</span>
              <span class="session-history-item__meta">
                <span>{{ session.model || '未选模型' }}</span>
                <span>{{ formatSessionTime(session.updated_at) }}</span>
              </span>
            </button>
            <a-popconfirm
              title="确认删除该会话？"
              ok-text="删除"
              cancel-text="保留"
              @confirm="removeChatSession(session)"
            >
              <button
                type="button"
                class="session-history-delete"
                :disabled="deletingSessionIds.includes(session.id)"
                title="删除会话"
              >
                <DeleteOutlined />
              </button>
            </a-popconfirm>
          </div>

          <a-empty v-if="!chatSessions.length && !sessionsLoading" description="暂无会话" />
        </div>
      </aside>
    </div>
  </ShellLayout>
</template>

<style scoped>
.session-layout-shell :deep(.app-header) {
  display: none !important;
}

.session-layout-shell :deep(.app-content) {
  padding: 0 !important;
  overflow: hidden !important;
}

.chat-workbench {
  display: grid;
  grid-template-columns: minmax(0, 1fr) 292px;
  height: 100vh;
  min-height: 0;
  overflow: hidden;
  color: #0a3472;
  background: var(--bg-page);
}

.session-sidebar {
  display: grid;
  grid-template-rows: auto minmax(0, 1fr);
  min-width: 0;
  min-height: 0;
  border-left: 1px solid rgba(137, 174, 232, 0.28);
  background: rgba(247, 251, 255, 0.82);
  backdrop-filter: blur(18px);
}

.session-sidebar__header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 10px;
  padding: 16px 14px 12px;
  border-bottom: 1px solid rgba(137, 174, 232, 0.2);
}

.session-sidebar__header strong {
  display: block;
  color: #0d3679;
  font-size: 15px;
  line-height: 1.3;
}

.session-sidebar__header span {
  display: block;
  margin-top: 2px;
  color: #6b7fa5;
  font-size: 12px;
}

.session-sidebar__actions {
  display: inline-flex;
  gap: 6px;
}

.session-sidebar__actions button,
.session-history-delete {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  border: 1px solid rgba(137, 174, 232, 0.32);
  background: rgba(255, 255, 255, 0.78);
  color: #2f5288;
  cursor: pointer;
}

.session-sidebar__actions button {
  width: var(--control-height);
  height: var(--control-height);
  border-radius: 8px;
}

.session-sidebar__actions button:disabled,
.session-history-delete:disabled {
  cursor: not-allowed;
  opacity: 0.55;
}

.session-sidebar__list {
  display: grid;
  align-content: start;
  gap: 6px;
  min-height: 0;
  overflow-y: auto;
  padding: 10px;
}

.session-sidebar__list.is-loading {
  opacity: 0.7;
  pointer-events: none;
}

.session-history-item {
  display: grid;
  grid-template-columns: minmax(0, 1fr) 30px;
  align-items: center;
  gap: 6px;
  width: 100%;
  padding: 4px;
  border: 1px solid transparent;
  border-radius: 8px;
  background: transparent;
}

.session-history-item:hover,
.session-history-item.is-active {
  border-color: rgba(37, 99, 235, 0.18);
  background: rgba(232, 241, 255, 0.86);
}

.session-history-item__main {
  display: grid;
  gap: 5px;
  min-width: 0;
  padding: 6px;
  border: 0;
  background: transparent;
  text-align: left;
  cursor: pointer;
}

.session-history-item__title,
.session-history-item__preview {
  min-width: 0;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.session-history-item__title {
  color: #183b73;
  font-size: 13px;
  font-weight: 700;
  line-height: 1.35;
}

.session-history-item__preview {
  color: #536b93;
  font-size: 12px;
  line-height: 1.4;
}

.session-history-item__meta {
  display: flex;
  justify-content: space-between;
  gap: 8px;
  color: #8a9bb7;
  font-size: 11px;
  line-height: 1.3;
}

.session-history-item__meta span {
  min-width: 0;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.session-history-delete {
  width: 30px;
  height: 30px;
  padding: 0;
  border-radius: 8px;
  color: #9f3a3a;
  font-size: 12px;
}

.session-toolbar__select {
  flex: 1 1 240px;
  width: clamp(220px, 24vw, 420px);
  min-width: 0;
}

.session-toolbar__select--model {
  flex-basis: 320px;
  width: clamp(240px, 28vw, 480px);
}

.session-clear-button {
  flex: 0 0 auto;
}

.session-clear-button {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  gap: 8px;
  height: var(--control-height);
  padding: 0 14px;
  border: 1px solid rgba(134, 172, 230, 0.32);
  border-radius: var(--radius-control);
  background: var(--bg-panel);
  color: #2e528f;
  font-size: 13px;
  font-weight: 600;
  box-shadow: none;
  cursor: pointer;
}

.session-clear-button:disabled,
.session-send-button:disabled,
.session-tool-button:disabled {
  cursor: not-allowed;
  opacity: 0.58;
}

.session-main {
  position: relative;
  display: grid;
  grid-template-rows: minmax(0, 1fr) auto;
  min-width: 0;
  min-height: 0;
  overflow: hidden;
  background: var(--bg-page);
}

.session-history {
  min-height: 0;
  overflow-y: auto;
  padding: 40px 32px 24px;
  scrollbar-gutter: stable;
}

.session-empty {
  display: grid;
  place-items: center;
  align-content: center;
  min-height: 100%;
  padding-bottom: 118px;
  text-align: center;
}

.session-empty__art {
  display: block;
  width: min(496px, 48vw);
  height: auto;
  margin: 0 auto 26px;
  filter: drop-shadow(0 22px 34px rgba(73, 129, 216, 0.14));
}

.session-empty__copy h1 {
  margin: 0 0 14px;
  color: #0d3679;
  font-size: clamp(26px, 2.2vw, 34px);
  font-weight: 850;
  line-height: 1.18;
  letter-spacing: -0.03em;
}

.session-empty__copy p {
  margin: 0;
  color: #4b6697;
  font-size: 18px;
  font-weight: 500;
  line-height: 1.5;
}

.session-message-list {
  display: grid;
  gap: 38px;
  width: min(920px, 100%);
  margin: 0 auto;
}

.session-message {
  display: grid;
  gap: 14px;
  justify-items: start;
}

.session-message--user {
  justify-items: end;
}

.session-message__thought {
  color: #8b95a8;
  font-size: 15px;
  line-height: 1.4;
}

.session-message__bubble {
  max-width: min(720px, 78%);
  padding: 14px 18px;
  border-radius: var(--radius-container);
  background: rgba(255, 255, 255, 0.84);
  color: #12244a;
  box-shadow: 0 10px 30px rgba(80, 116, 170, 0.08);
}

.session-message--user .session-message__bubble {
  border-radius: var(--radius-container);
  background: rgba(242, 242, 242, 0.96);
  box-shadow: none;
}

.session-message__text {
  margin: 0;
  color: #0f172a;
  font-size: 16px;
  line-height: 1.65;
  white-space: pre-wrap;
  overflow-wrap: anywhere;
}

.session-message__error {
  margin-top: 8px;
  color: #dc2626;
  font-size: 13px;
  line-height: 1.5;
}

.session-reference-strip,
.session-attachments {
  display: flex;
  flex-wrap: wrap;
  gap: 10px;
}

.session-reference-card img {
  display: block;
  width: 180px;
  height: 132px;
  object-fit: cover;
  border-radius: var(--radius-control);
}

.session-generated-grid {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(220px, 1fr));
  gap: 12px;
  width: min(640px, 100%);
}

.session-generated-grid img {
  display: block;
  width: 100%;
  aspect-ratio: 1 / 1;
  object-fit: cover;
  border-radius: var(--radius-control);
}

.session-composer {
  position: relative;
  z-index: 3;
  padding: 0 24px 24px;
}

.session-composer__controls {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 8px;
  width: min(1180px, calc(100vw - 48px));
  margin: 0 auto 8px;
}

.session-composer__alert {
  width: min(1180px, calc(100vw - 48px));
  margin: 0 auto 8px;
}

.session-composer__frame {
  display: grid;
  grid-template-columns: var(--control-height) minmax(0, 1fr) auto;
  align-items: stretch;
  gap: 12px;
  width: min(1180px, calc(100vw - 48px));
  min-height: 86px;
  margin: 0 auto;
  padding: 12px 14px;
  border: 1px solid rgba(159, 185, 226, 0.34);
  border-radius: var(--radius-container);
  background: var(--bg-panel);
  box-shadow: var(--shadow-card);
  backdrop-filter: none;
}

.session-composer__side {
  display: grid;
  align-content: space-between;
  justify-items: end;
  gap: 10px;
}

.session-tool-button,
.session-send-button {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  border: 0;
  cursor: pointer;
}

.session-tool-button {
  width: var(--control-height);
  height: var(--control-height);
  border: 1px solid rgba(177, 199, 233, 0.48);
  border-radius: var(--radius-control);
  background: rgba(247, 251, 255, 0.9);
  color: #2077ff;
  font-size: 17px;
  box-shadow: none;
}

.session-send-button {
  gap: 8px;
  min-width: 112px;
  height: var(--control-height);
  padding: 0 14px;
  border-radius: var(--radius-control);
  background: var(--accent);
  color: #ffffff;
  font-size: 13px;
  font-weight: 600;
  box-shadow: none;
}

.session-footnote {
  color: #7d91b4;
  font-size: 12px;
  font-weight: 500;
  line-height: 1.4;
}

.session-attachments {
  width: min(1180px, calc(100vw - 48px));
  margin: 0 auto 8px;
}

.session-composer__image-size {
  flex: 1 0 592px;
  min-width: min(100%, 592px);
}

.session-attachment {
  position: relative;
  display: grid;
  grid-template-columns: 44px minmax(0, 1fr);
  gap: 8px;
  align-items: center;
  width: 190px;
  padding: 8px;
  border: 1px solid rgba(159, 185, 226, 0.34);
  border-radius: var(--radius-container);
  background: var(--bg-panel);
}

.session-attachment img {
  width: 44px;
  height: 44px;
  object-fit: cover;
  border-radius: var(--radius-control);
}

.session-attachment span {
  min-width: 0;
  overflow: hidden;
  color: #3b527b;
  font-size: 12px;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.session-attachment button {
  position: absolute;
  top: -7px;
  right: -7px;
  display: grid;
  place-items: center;
  width: 22px;
  height: 22px;
  padding: 0;
  border: 1px solid rgba(190, 74, 74, 0.16);
  border-radius: 999px;
  background: #ffffff;
  color: #dc2626;
  cursor: pointer;
}

.image-size-editor {
  display: grid;
  grid-template-columns: minmax(260px, 1fr) 132px 132px 34px;
  gap: 8px;
  align-items: center;
}

.image-ratio-presets {
  display: flex;
  align-items: stretch;
  gap: 6px;
  min-width: 0;
}

.image-ratio-preset {
  display: grid;
  grid-template-rows: 24px auto;
  place-items: center;
  gap: 3px;
  min-width: var(--control-height);
  padding: 4px 6px;
  border: 1px solid rgba(137, 174, 232, 0.34);
  border-radius: var(--radius-control);
  background: rgba(246, 251, 255, 0.78);
  color: #3b527b;
  font-size: 11px;
  line-height: 1;
  cursor: pointer;
}

.image-ratio-preset.is-active {
  border-color: rgba(37, 99, 235, 0.46);
  background: rgba(232, 241, 255, 0.92);
  color: #1d4ed8;
}

.image-ratio-preset.is-detected:not(.is-locked) {
  border-style: dashed;
  background: rgba(246, 251, 255, 0.92);
}

.image-ratio-preset.is-locked {
  box-shadow: inset 0 0 0 1px rgba(37, 99, 235, 0.12);
}

.image-ratio-preset__box {
  display: block;
  max-width: 28px;
  max-height: 24px;
  min-width: 10px;
  min-height: 10px;
  width: 24px;
  border: 1px solid currentColor;
  border-radius: 4px;
  background: var(--bg-panel);
}

.image-size-editor__lock {
  width: 34px;
  height: var(--control-height);
}

.chat-file-input {
  position: absolute;
  width: 1px;
  height: 1px;
  overflow: hidden;
  clip: rect(0 0 0 0);
  clip-path: inset(50%);
  white-space: nowrap;
}

.chat-workbench :deep(.ant-select-selector) {
  height: var(--control-height) !important;
  padding: 0 11px !important;
  border: 1px solid var(--border-soft) !important;
  border-radius: var(--radius-control) !important;
  background: var(--bg-panel) !important;
  color: #25477f !important;
  box-shadow: none !important;
}

.chat-workbench :deep(.ant-select-selection-search-input),
.chat-workbench :deep(.ant-select-selection-item),
.chat-workbench :deep(.ant-select-selection-placeholder) {
  height: 30px !important;
  line-height: 30px !important;
  color: #284b84 !important;
  font-size: 13px;
  font-weight: 500;
}

.chat-workbench :deep(.ant-select-selection-item) {
  padding-inline-end: 22px !important;
}

.chat-workbench :deep(.ant-select-selection-placeholder) {
  color: #8ca0c0 !important;
}

.chat-workbench :deep(.ant-select-arrow),
.chat-workbench :deep(.ant-select-clear),
.chat-workbench :deep(.ant-select-prefix) {
  color: #355892 !important;
  font-size: 13px;
}

.chat-workbench :deep(.ant-input-number),
.chat-workbench :deep(.ant-input-number-group-addon) {
  border-color: var(--border-soft) !important;
  background: var(--bg-panel) !important;
  color: #284b84 !important;
}

.chat-workbench :deep(.ant-input-number),
.chat-workbench :deep(.ant-input-number-input),
.chat-workbench :deep(.ant-input-number-group-addon) {
  height: var(--control-height) !important;
  font-size: 13px;
}

.chat-workbench :deep(.ant-input-number-input) {
  line-height: 30px !important;
}

.session-composer__input :deep(textarea) {
  min-height: 62px !important;
  padding: 0 !important;
  border: 0 !important;
  background: transparent !important;
  color: #203a67 !important;
  box-shadow: none !important;
  resize: none;
  font-size: 13px;
  font-weight: 400;
  line-height: 1.55;
}

.session-composer__input :deep(textarea::placeholder) {
  color: #8ea0bf !important;
}

.chat-workbench :deep(.ant-select-focused .ant-select-selector),
.chat-workbench :deep(.ant-input-focused),
.chat-workbench :deep(.ant-input-number-focused) {
  border-color: rgba(39, 119, 255, 0.48) !important;
  box-shadow: 0 0 0 3px rgba(39, 119, 255, 0.1) !important;
}

.chat-workbench :deep(.ant-alert) {
  border-color: rgba(137, 174, 232, 0.3);
  background: rgba(255, 255, 255, 0.78);
}

@media (max-width: 1180px) {
  .chat-workbench {
    grid-template-columns: minmax(0, 1fr) 260px;
  }

  .session-toolbar__select {
    width: clamp(200px, 24vw, 340px);
  }

  .session-toolbar__select--model {
    width: clamp(220px, 28vw, 380px);
  }

  .session-composer__controls,
  .session-composer__frame,
  .session-attachments {
    width: min(100%, calc(100vw - 40px));
  }

  .session-composer__alert {
    width: min(100%, calc(100vw - 40px));
  }

  .session-composer__image-size {
    min-width: 560px;
  }
}

@media (max-width: 900px) {
  .chat-workbench {
    grid-template-columns: 1fr;
    height: auto;
    min-height: 100vh;
    overflow: visible;
  }

  .session-sidebar {
    order: 2;
    max-height: 320px;
    border-left: 0;
    border-top: 1px solid rgba(137, 174, 232, 0.28);
    border-bottom: 1px solid rgba(137, 174, 232, 0.28);
  }

  .session-toolbar__select,
  .session-toolbar__select--model,
  .session-clear-button {
    width: 100%;
  }

  .session-main {
    order: 1;
    grid-template-rows: auto auto;
    min-height: auto;
    overflow: visible;
  }

  .session-history {
    min-height: auto;
    overflow: visible;
    padding: 16px 14px 12px;
  }

  .session-empty {
    margin: 0 auto;
    padding: 20px;
  }

  .session-empty__art {
    width: min(360px, 86vw);
  }

  .session-composer {
    padding: 0 14px 18px;
  }

  .session-composer__frame {
    grid-template-columns: var(--control-height) minmax(0, 1fr);
    width: 100%;
    min-height: 118px;
    padding: 14px;
  }

  .session-composer__side {
    grid-column: 1 / -1;
    grid-template-columns: minmax(0, 1fr) auto;
    align-items: center;
  }

  .session-tool-button {
    width: var(--control-height);
    height: var(--control-height);
  }

  .session-send-button {
    min-width: 112px;
  }

  .session-attachments {
    width: 100%;
  }

  .session-composer__controls {
    width: 100%;
    flex-direction: column;
    align-items: stretch;
    gap: 8px;
    margin-bottom: 10px;
  }

  .session-toolbar__select,
  .session-toolbar__select--model {
    flex-basis: auto;
    min-height: var(--control-height);
  }

  .chat-workbench :deep(.ant-select-selector) {
    height: var(--control-height) !important;
  }

  .chat-workbench :deep(.ant-select-selection-search-input),
  .chat-workbench :deep(.ant-select-selection-item),
  .chat-workbench :deep(.ant-select-selection-placeholder) {
    height: 34px !important;
    line-height: 34px !important;
  }

  .session-clear-button {
    height: var(--control-height);
  }

  .session-composer__image-size {
    grid-template-columns: 1fr;
    width: 100%;
    min-width: 0;
  }

  .image-ratio-presets {
    flex-wrap: wrap;
  }
}
</style>
