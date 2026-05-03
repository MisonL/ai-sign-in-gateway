<script setup lang="ts">
import {
  ApiOutlined,
  ClearOutlined,
  DeleteOutlined,
  FileImageOutlined,
  LockOutlined,
  PaperClipOutlined,
  PictureOutlined,
  SendOutlined,
  UnlockOutlined,
  UserOutlined,
} from '@ant-design/icons-vue'
import { computed, nextTick, onMounted, reactive, ref, type ComponentPublicInstance } from 'vue'
import { ApiError, getSites, listToolModels, testChat } from '../api'
import ShellLayout from '../components/ShellLayout.vue'
import { formatGroupNames } from '../format'
import { useToast } from '../toast'
import type { ChatImageReference, ChatRequestMessage, ChatResult, ModelListItem, Site } from '../types'

type MessageRole = 'system' | 'user' | 'assistant'
type MessageStatus = 'idle' | 'sending' | 'done' | 'error'

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
const modelItems = ref<ModelListItem[]>([])
const modelLoadMessage = ref('')
const modelLoadError = ref(false)
const messages = ref<ChatMessage[]>([])
const referenceImages = ref<ChatImageReference[]>([])
const fileInput = ref<HTMLInputElement | null>(null)
const scrollBody = ref<HTMLElement | null>(null)
const imageRatioLocked = ref(false)
const imageAspectRatio = ref(1)

const selectedSite = computed(() =>
  sites.value.find((item) => String(item.id) === selectedSiteId.value) ?? null,
)

const siteOptions = computed(() =>
  sites.value.map((site) => ({
    label: `${site.name} · ${site.plugin_key}`,
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
    .join(' · ')
})
const modelLoadAlertType = computed(() => (modelLoadError.value ? 'error' : 'info'))

const imageSizeOptions = [
  { label: '1024 x 1024', value: '1024x1024' },
  { label: '1024 x 1536', value: '1024x1536' },
  { label: '1536 x 1024', value: '1536x1024' },
]
const imagePresetValue = computed(() =>
  imageSizeOptions.some((option) => option.value === form.image_size) ? form.image_size : undefined,
)
const imageRatioTooltip = computed(() => (imageRatioLocked.value ? `已锁定 ${form.image_width}:${form.image_height}` : '锁定当前宽高比'))

function newID(prefix = 'msg') {
  return `${prefix}-${Date.now()}-${Math.random().toString(36).slice(2, 8)}`
}

function modelOptionValue(model: ModelListItem) {
  return `${model.id}\u0000${model.route_type}\u0000${model.key_fingerprint}\u0000${model.base_url}`
}

function modelOptionLabel(model: ModelListItem) {
  const mode = model.mode === 'image' ? '图片' : '对话'
  const key = model.key_name || shortFingerprint(model.key_fingerprint)
  return [model.id, mode, routeTypeLabel(model.route_type), key].filter(Boolean).join(' · ')
}

function routeTypeLabel(routeType: string) {
  if (routeType === 'claude') {
    return 'Claude'
  }
  if (routeType === 'gemini') {
    return 'Gemini'
  }
  return 'OpenAI'
}

function shortFingerprint(value: string) {
  return value ? `Key ${value.slice(0, 8)}` : ''
}

function clampImageDimension(value: unknown) {
  const parsed = Number(value)
  if (!Number.isFinite(parsed)) {
    return 1024
  }
  return Math.min(4096, Math.max(64, Math.round(parsed)))
}

function syncImageSizeFromDimensions() {
  form.image_width = clampImageDimension(form.image_width)
  form.image_height = clampImageDimension(form.image_height)
  form.image_size = `${form.image_width}x${form.image_height}`
}

function applyImagePreset(value: unknown) {
  if (typeof value !== 'string') {
    return
  }
  const match = /^(\d+)x(\d+)$/.exec(value)
  if (!match) {
    return
  }
  form.image_width = clampImageDimension(match[1])
  form.image_height = clampImageDimension(match[2])
  imageAspectRatio.value = form.image_width / form.image_height
  syncImageSizeFromDimensions()
}

function toggleImageRatioLock() {
  if (!imageRatioLocked.value) {
    syncImageSizeFromDimensions()
    imageAspectRatio.value = form.image_width / form.image_height
  }
  imageRatioLocked.value = !imageRatioLocked.value
}

function handleImageWidthChange(value: unknown) {
  form.image_width = clampImageDimension(value)
  if (imageRatioLocked.value) {
    form.image_height = clampImageDimension(form.image_width / imageAspectRatio.value)
  }
  syncImageSizeFromDimensions()
}

function handleImageHeightChange(value: unknown) {
  form.image_height = clampImageDimension(value)
  if (imageRatioLocked.value) {
    form.image_width = clampImageDimension(form.image_height * imageAspectRatio.value)
  }
  syncImageSizeFromDimensions()
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

async function applySelectedSite() {
  modelItems.value = []
  form.model_key = undefined
  modelLoadMessage.value = ''
  modelLoadError.value = false
  if (!selectedSite.value) {
    return
  }
  modelsLoading.value = true
  try {
    const result = await listToolModels(Number(selectedSite.value.id))
    modelItems.value = result.items ?? []
    modelLoadMessage.value = modelListMessage(result.message, result.status_code)
    const preferred = chooseDefaultModel(modelItems.value)
    form.model_key = preferred ? modelOptionValue(preferred) : undefined
    if (!result.ok) {
      modelLoadError.value = true
      toast.error(modelLoadMessage.value || '模型列表加载失败')
    }
  } catch (err) {
    modelLoadError.value = true
    modelLoadMessage.value = modelListExceptionMessage(err)
    toast.error(modelLoadMessage.value)
  } finally {
    modelsLoading.value = false
  }
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

function handleModelChange() {
  form.input = ''
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

function messageTime(value: string) {
  return new Intl.DateTimeFormat('zh-CN', { hour: '2-digit', minute: '2-digit', second: '2-digit' }).format(new Date(value))
}

function readableLatency(value: number | null | undefined) {
  return value === null || value === undefined ? '' : `${Math.round(value)} ms`
}

function imageSource(image: ChatImageReference) {
  return image.url
}

function triggerImagePicker() {
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
  const files = Array.from(input.files ?? [])
  input.value = ''
  const imageFiles = files.filter((file) => file.type.startsWith('image/'))
  if (imageFiles.length !== files.length) {
    toast.error('只能添加图片文件。')
  }
  const remaining = Math.max(0, 6 - referenceImages.value.length)
  imageFiles.slice(0, remaining).forEach((file) => {
    void fileToDataURL(file)
      .then((url) => {
        referenceImages.value.push({ name: file.name, url })
      })
      .catch((err) => toast.error(err instanceof Error ? err.message : '图片读取失败'))
  })
  if (imageFiles.length > remaining) {
    toast.info('最多保留 6 张参考图。')
  }
}

function removeReferenceImage(index: number) {
  referenceImages.value.splice(index, 1)
}

function clearConversation() {
  messages.value = []
  referenceImages.value = []
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
  const refs = referenceImages.value.map((item) => ({ ...item }))
  const userMessage: ChatMessage = {
    id: newID('user'),
    role: 'user',
    content,
    references: refs,
    createdAt: new Date().toISOString(),
    status: 'done',
  }
  const assistantMessage: ChatMessage = {
    id: newID('assistant'),
    role: 'assistant',
    content: activeMode.value === 'image' ? '正在生成图片...' : '正在思考...',
    createdAt: new Date().toISOString(),
    status: 'sending',
  }

  messages.value.push(userMessage, assistantMessage)
  form.input = ''
  referenceImages.value = []
  await scrollToBottom()
  loading.value = true

  try {
    const requestMessages = toRequestMessages()
    const result = await testChat({
      site_id: Number(selectedSiteId.value),
      route_type: model.route_type,
      key_fingerprint: model.key_fingerprint,
      model: model.id,
      mode: 'auto',
      prompt: content,
      messages: activeMode.value === 'chat' ? requestMessages : undefined,
      reference_images: refs,
      image_size: activeMode.value === 'image' ? form.image_size : undefined,
    })
    assistantMessage.status = result.ok ? 'done' : 'error'
    assistantMessage.statusCode = result.status_code
    assistantMessage.latencyMs = result.latency_ms
    assistantMessage.error = result.ok ? undefined : result.message
    if (activeMode.value === 'image') {
      assistantMessage.images = resultImages(result)
      assistantMessage.content = assistantMessage.images.length
        ? (result.revised_prompt ? `已生成图片。优化提示词：${result.revised_prompt}` : '已生成图片。')
        : result.message
    } else {
      assistantMessage.content = result.output || result.message || '接口未返回可读文本。'
    }
    toast.success(result.ok ? '请求完成。' : result.message)
  } catch (err) {
    assistantMessage.status = 'error'
    assistantMessage.content = err instanceof Error ? err.message : '请求失败'
    assistantMessage.error = assistantMessage.content
    toast.error(assistantMessage.content)
  } finally {
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

onMounted(loadSites)
</script>

<template>
  <ShellLayout>
    <div class="page-stack page-stack--fit chat-workbench">
      <a-card :bordered="false" class="admin-card admin-card--fill chat-card">
        <template #title>
          <div class="chat-card-title">
            <span>AI 对话</span>
            <a-tag :color="activeMode === 'image' ? 'purple' : 'blue'">{{ activeMode === 'image' ? '图片生成' : '多轮对话' }}</a-tag>
          </div>
        </template>
        <template #extra>
          <a-space wrap>
            <a-button size="small" :disabled="loading" @click="clearConversation">
              <template #icon><ClearOutlined /></template>
              清空
            </a-button>
          </a-space>
        </template>

        <div class="chat-layout">
          <aside class="chat-sidebar">
            <div class="chat-sidebar__section">
              <div class="chat-sidebar__label">路由与模型</div>
              <a-select
                v-model:value="selectedSiteId"
                :options="siteOptions"
                allow-clear
                show-search
                option-filter-prop="label"
                placeholder="选择已保存站点"
                @change="applySelectedSite"
              />
              <div v-if="selectedSite" class="chat-site-meta">
                <ApiOutlined />
                <span>{{ formatGroupNames(selectedSite.group_name) || '未分组' }}</span>
              </div>
              <a-select
                v-model:value="form.model_key"
                :options="modelOptions"
                :loading="modelsLoading"
                :disabled="!selectedSite || modelsLoading"
                allow-clear
                show-search
                option-filter-prop="label"
                placeholder="选择模型"
                @change="handleModelChange"
              />
              <div v-if="selectedModelMeta" class="chat-site-meta">
                <ApiOutlined />
                <span>{{ selectedModelMeta }}</span>
              </div>
              <a-alert
                v-if="modelLoadMessage && (!selectedModelMeta || modelLoadError)"
                :type="modelLoadAlertType"
                show-icon
                :message="modelLoadMessage"
              />
              <template v-if="activeMode === 'image'">
                <a-select
                  :value="imagePresetValue"
                  :options="imageSizeOptions"
                  placeholder="预设尺寸"
                  @change="applyImagePreset"
                />
                <div class="image-size-editor">
                  <a-input-number
                    :value="form.image_width"
                    :min="64"
                    :max="4096"
                    :step="1"
                    addon-before="宽"
                    @change="handleImageWidthChange"
                  />
                  <span class="image-size-editor__separator">x</span>
                  <a-input-number
                    :value="form.image_height"
                    :min="64"
                    :max="4096"
                    :step="1"
                    addon-before="高"
                    @change="handleImageHeightChange"
                  />
                  <a-tooltip :title="imageRatioTooltip">
                    <a-button
                      class="image-size-editor__lock"
                      :type="imageRatioLocked ? 'primary' : 'default'"
                      @click="toggleImageRatioLock"
                    >
                      <template #icon>
                        <LockOutlined v-if="imageRatioLocked" />
                        <UnlockOutlined v-else />
                      </template>
                    </a-button>
                  </a-tooltip>
                </div>
              </template>
            </div>
          </aside>

          <section class="chat-main">
            <div :ref="setScrollBody" class="chat-history">
              <div v-if="visibleMessages.length" class="chat-message-list">
                <article
                  v-for="message in visibleMessages"
                  :key="message.id"
                  class="chat-message"
                  :class="[`chat-message--${message.role}`, `chat-message--${message.status}`]"
                >
                  <div class="chat-avatar">
                    <UserOutlined v-if="message.role === 'user'" />
                    <PictureOutlined v-else-if="message.images?.length" />
                    <ApiOutlined v-else />
                  </div>
                  <div class="chat-bubble">
                    <div class="chat-bubble__meta">
                      <strong>{{ message.role === 'user' ? '你' : 'AI' }}</strong>
                      <span>{{ messageTime(message.createdAt) }}</span>
                      <span v-if="readableLatency(message.latencyMs)">{{ readableLatency(message.latencyMs) }}</span>
                      <a-tag v-if="message.status === 'error'" color="error">失败</a-tag>
                    </div>
                    <p v-if="message.content" class="chat-bubble__text">{{ message.content }}</p>
                    <div v-if="message.references?.length" class="chat-reference-strip">
                      <div v-for="image in message.references" :key="image.url" class="chat-reference-thumb">
                        <img :src="imageSource(image)" :alt="image.name" />
                        <span>{{ image.name }}</span>
                      </div>
                    </div>
                    <div v-if="message.images?.length" class="chat-generated-grid">
                      <a :href="imageSource(image)" target="_blank" rel="noreferrer" v-for="image in message.images" :key="image.url">
                        <img :src="imageSource(image)" :alt="image.name" />
                      </a>
                    </div>
                    <div v-if="message.error" class="chat-error">{{ message.error }}</div>
                  </div>
                </article>
              </div>
              <a-empty v-else description="开始一段新的对话。" />
            </div>

            <div class="chat-composer">
              <div v-if="referenceImages.length" class="chat-attachments">
                <div v-for="(image, index) in referenceImages" :key="image.url" class="chat-attachment">
                  <img :src="imageSource(image)" :alt="image.name" />
                  <span>{{ image.name }}</span>
                  <button type="button" @click="removeReferenceImage(index)">
                    <DeleteOutlined />
                  </button>
                </div>
              </div>

              <input
                ref="fileInput"
                class="chat-file-input"
                type="file"
                accept="image/*"
                multiple
                hidden
                tabindex="-1"
                @change="addReferenceImages"
              />
              <div class="chat-input-row">
                <a-button class="chat-tool-button" :disabled="loading" @click="triggerImagePicker">
                  <template #icon><PaperClipOutlined /></template>
                </a-button>
                <a-textarea
                  v-model:value="form.input"
                  class="chat-input"
                  :auto-size="{ minRows: 1, maxRows: 6 }"
                  :placeholder="sendPlaceholder"
                  @keydown="handleEditorKeydown"
                />
                <a-button type="primary" class="chat-send-button" :loading="loading" @click="sendMessage">
                  <template #icon><SendOutlined /></template>
                  发送
                </a-button>
              </div>
              <div class="chat-composer__hint">
                <FileImageOutlined />
                <span>{{ activeMode === 'image' ? '当前模型将请求图片接口；参考图会作为编辑/生成依据传给支持的上游。' : '当前模型将请求对话接口；参考图会作为视觉输入传给支持多模态的模型。' }}</span>
              </div>
            </div>
          </section>
        </div>
      </a-card>
    </div>
  </ShellLayout>
</template>

<style scoped>
.chat-workbench {
  height: 100%;
  min-height: 0;
  grid-template-rows: minmax(0, 1fr);
}

.chat-card {
  display: flex;
  flex-direction: column;
  height: 100%;
  min-height: 0;
}

.chat-card :deep(.ant-card-body) {
  display: flex;
  flex-direction: column;
  flex: 1 1 auto;
  min-height: 0;
  padding: 0;
}

.chat-card-title {
  display: inline-flex;
  align-items: center;
  gap: 10px;
}

.chat-layout {
  display: grid;
  grid-template-columns: 320px minmax(0, 1fr);
  flex: 1 1 auto;
  min-height: 0;
  height: 100%;
  overflow: hidden;
}

.chat-sidebar {
  display: flex;
  flex-direction: column;
  gap: 16px;
  min-width: 0;
  min-height: 0;
  overflow-y: auto;
  padding: 16px;
  border-right: 1px solid var(--border-muted);
  background: var(--bg-subtle);
}

.chat-sidebar__section {
  display: grid;
  gap: 10px;
}

.chat-sidebar__label {
  color: #344054;
  font-size: 12px;
  font-weight: 800;
}

.chat-site-meta,
.chat-composer__hint {
  color: var(--text-muted);
  font-size: 12px;
  line-height: 1.45;
}

.chat-site-meta {
  display: inline-flex;
  align-items: center;
  gap: 6px;
}

.image-size-editor {
  display: grid;
  grid-template-columns: minmax(0, 1fr) auto minmax(0, 1fr) 38px;
  gap: 8px;
  align-items: center;
}

.image-size-editor :deep(.ant-input-number-group-wrapper) {
  width: 100%;
  min-width: 0;
}

.image-size-editor :deep(.ant-input-number) {
  width: 100%;
}

.image-size-editor :deep(.ant-input-number-group-addon) {
  padding-inline: 8px;
  color: var(--text-muted);
  font-size: 12px;
}

.image-size-editor__separator {
  color: var(--text-faint);
  font-size: 12px;
  font-weight: 700;
}

.image-size-editor__lock {
  width: 38px;
  height: 32px;
  padding: 0;
}

.chat-main {
  display: grid;
  grid-template-rows: minmax(0, 1fr) auto;
  min-width: 0;
  min-height: 0;
  height: 100%;
  overflow: hidden;
}

.chat-history {
  min-height: 0;
  overflow-y: auto;
  padding: 18px;
  background: #f7f9fd;
}

.chat-message-list {
  display: grid;
  gap: 16px;
}

.chat-message {
  display: grid;
  grid-template-columns: 34px minmax(0, 1fr);
  gap: 10px;
  align-items: flex-start;
}

.chat-message--user {
  grid-template-columns: minmax(0, 1fr) 34px;
}

.chat-message--user .chat-avatar {
  grid-column: 2;
  background: #14213d;
  color: #fff;
}

.chat-message--user .chat-bubble {
  grid-column: 1;
  grid-row: 1;
  justify-self: end;
  background: #eaf1ff;
}

.chat-avatar {
  display: grid;
  place-items: center;
  width: 34px;
  height: 34px;
  border-radius: var(--radius-container);
  background: #fff;
  color: var(--accent);
  border: 1px solid var(--border-soft);
}

.chat-bubble {
  max-width: min(760px, 100%);
  padding: 12px;
  border: 1px solid var(--border-soft);
  border-radius: var(--radius-container);
  background: #fff;
  box-shadow: var(--shadow-card);
}

.chat-bubble__meta {
  display: flex;
  flex-wrap: wrap;
  gap: 6px 10px;
  align-items: center;
  margin-bottom: 6px;
  color: var(--text-faint);
  font-size: 12px;
}

.chat-bubble__text {
  margin: 0;
  white-space: pre-wrap;
  overflow-wrap: anywhere;
  line-height: 1.65;
}

.chat-reference-strip,
.chat-attachments {
  display: flex;
  flex-wrap: wrap;
  gap: 8px;
  margin-top: 10px;
}

.chat-reference-thumb,
.chat-attachment {
  display: grid;
  grid-template-columns: 44px minmax(0, 1fr);
  align-items: center;
  gap: 8px;
  width: 180px;
  padding: 6px;
  border: 1px solid var(--border-muted);
  border-radius: var(--radius-control);
  background: rgba(255, 255, 255, 0.78);
}

.chat-reference-thumb img,
.chat-attachment img {
  width: 44px;
  height: 44px;
  object-fit: cover;
  border-radius: 8px;
}

.chat-reference-thumb span,
.chat-attachment span {
  min-width: 0;
  overflow: hidden;
  color: var(--text-muted);
  font-size: 12px;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.chat-generated-grid {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(180px, 260px));
  gap: 10px;
  margin-top: 10px;
}

.chat-generated-grid img {
  display: block;
  width: 100%;
  aspect-ratio: 1 / 1;
  object-fit: cover;
  border: 1px solid var(--border-soft);
  border-radius: var(--radius-container);
}

.chat-error {
  margin-top: 8px;
  color: var(--danger);
  font-size: 12px;
}

.chat-composer {
  display: grid;
  gap: 10px;
  padding: 12px;
  border-top: 1px solid var(--border-muted);
  background: #fff;
}

.chat-input-row {
  display: grid;
  grid-template-columns: auto minmax(0, 1fr) auto;
  gap: 8px;
  align-items: end;
}

.chat-tool-button,
.chat-send-button {
  height: 38px;
}

.chat-input :deep(textarea) {
  resize: none;
}

.chat-attachment {
  position: relative;
  margin-top: 0;
}

.chat-attachment button {
  position: absolute;
  top: -7px;
  right: -7px;
  display: grid;
  place-items: center;
  width: 22px;
  height: 22px;
  padding: 0;
  border: 1px solid var(--border-soft);
  border-radius: 999px;
  background: #fff;
  color: var(--danger);
  cursor: pointer;
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

.chat-composer__hint {
  display: inline-flex;
  align-items: center;
  gap: 6px;
}

@media (max-width: 980px) {
  .chat-layout {
    grid-template-columns: 1fr;
  }

  .chat-sidebar {
    border-right: 0;
    border-bottom: 1px solid var(--border-muted);
  }
}

@media (max-width: 640px) {
  .chat-input-row {
    grid-template-columns: auto minmax(0, 1fr);
  }

  .chat-send-button {
    grid-column: 1 / -1;
  }
}
</style>
