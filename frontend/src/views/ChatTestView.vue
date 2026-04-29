<script setup lang="ts">
import { computed, onMounted, reactive, ref } from 'vue'
import { getSites, testChat, testMcp } from '../api'
import ShellLayout from '../components/ShellLayout.vue'
import { formatGroupNames } from '../format'
import { useToast } from '../toast'
import type { ChatResult, McpTestResult, Site } from '../types'

type TestMode = 'chat' | 'mcp'

const chatPrompt = '请返回一句简短的 API 健康检查结果。'
const mcpPrompt = '请调用 chrome-devtool-mcp，读取当前页面标题，并用一句话说明你实际执行了什么。'

const form = reactive({
  base_url: 'https://api.openai.com/v1',
  api_key: '',
  model: 'gpt-4o-mini',
  prompt: chatPrompt,
  server_label: 'chrome-devtool-mcp',
  server_url: '',
  allowed_tools_text: '',
  require_approval: 'never' as 'never' | 'always',
})

const toast = useToast()
const sites = ref<Site[]>([])
const selectedSiteId = ref<string>()
const loading = ref(false)
const mode = ref<TestMode>('chat')
const result = ref<ChatResult | McpTestResult | null>(null)

const selectedSite = computed(() =>
  sites.value.find((item) => String(item.id) === selectedSiteId.value) ?? null,
)

const siteOptions = computed(() =>
  sites.value.map((site) => ({
    label: `${site.name} · ${site.plugin_key}`,
    value: String(site.id),
  })),
)

const modeOptions = [
  { label: '普通对话', value: 'chat' },
  { label: 'MCP 工具调用', value: 'mcp' },
]

const requireApprovalOptions = [
  { label: 'never', value: 'never' },
  { label: 'always', value: 'always' },
]

const modeTitle = computed(() => (mode.value === 'chat' ? '普通对话测试' : 'MCP 工具调用测试'))
const mcpResult = computed(() => (mode.value === 'mcp' ? (result.value as McpTestResult | null) : null))

function parseAllowedTools(input: string): string[] {
  return input
    .split(/[\n,]/)
    .map((item) => item.trim())
    .filter(Boolean)
}

function applySelectedSite() {
  if (!selectedSite.value) {
    return
  }
  form.base_url = selectedSite.value.base_url
  form.api_key = selectedSite.value.credentials.api_key ?? ''
}

function switchMode(nextMode: TestMode) {
  if (mode.value === nextMode) {
    return
  }
  mode.value = nextMode
  result.value = null
  form.prompt = nextMode === 'chat' ? chatPrompt : mcpPrompt
}

async function loadSites() {
  try {
    sites.value = await getSites()
  } catch (err) {
    toast.error(err instanceof Error ? err.message : '站点加载失败')
  }
}

async function submit() {
  loading.value = true
  try {
    if (mode.value === 'chat') {
      result.value = await testChat({
        base_url: form.base_url,
        api_key: form.api_key,
        model: form.model,
        prompt: form.prompt,
      })
    } else {
      if (!form.server_url.trim()) {
        throw new Error('MCP 模式必须填写可访问的 MCP Server URL。')
      }
      result.value = await testMcp({
        base_url: form.base_url,
        api_key: form.api_key,
        model: form.model,
        prompt: form.prompt,
        server_label: form.server_label,
        server_url: form.server_url,
        allowed_tools: parseAllowedTools(form.allowed_tools_text),
        require_approval: form.require_approval,
      })
    }
    toast.success(result.value.ok ? '测试完成。' : result.value.message)
  } catch (err) {
    toast.error(err instanceof Error ? err.message : '测试失败')
  } finally {
    loading.value = false
  }
}

onMounted(loadSites)
</script>

<template>
  <ShellLayout>
    <div class="page-stack page-stack--fit">
      <a-row :gutter="[16, 16]" class="page-grid-fill">
        <a-col :xs="24" :xl="12">
          <a-card :bordered="false" class="admin-card admin-card--fill" title="测试请求">
            <template #extra>{{ modeTitle }}</template>
            <div class="card-form">
              <div class="card-scroll card-scroll--padded">
                <a-form layout="vertical">
                  <a-form-item label="测试模式">
                    <a-space wrap>
                      <a-button
                        v-for="item in modeOptions"
                        :key="item.value"
                        :type="mode === item.value ? 'primary' : 'default'"
                        @click="switchMode(item.value as TestMode)"
                      >
                        {{ item.label }}
                      </a-button>
                    </a-space>
                  </a-form-item>

                  <a-form-item label="快捷选择站点">
                    <a-select
                      v-model:value="selectedSiteId"
                      :options="siteOptions"
                      allow-clear
                      placeholder="手动填写或选择已保存站点"
                      @change="applySelectedSite"
                    />
                  </a-form-item>

                  <a-alert
                    v-if="selectedSite"
                    type="info"
                    show-icon
                    style="margin-bottom: 16px"
                    :message="`${formatGroupNames(selectedSite.group_name) || '未分组'} / ${selectedSite.base_url}`"
                  />

                  <a-form-item label="Base URL">
                    <a-input v-model:value="form.base_url" />
                  </a-form-item>

                  <a-form-item label="API Key">
                    <a-input-password v-model:value="form.api_key" />
                  </a-form-item>

                  <a-form-item label="模型名">
                    <a-input v-model:value="form.model" />
                  </a-form-item>

                  <a-form-item label="Prompt">
                    <a-textarea v-model:value="form.prompt" :rows="5" />
                  </a-form-item>

                  <template v-if="mode === 'mcp'">
                    <a-alert
                      type="warning"
                      show-icon
                      style="margin-bottom: 16px"
                      message="MCP 模式要求目标接口支持 /responses 和 remote MCP tool。"
                      description="如果你接的是只兼容 /chat/completions 的中转站，这里通常会直接返回 404 或 400。"
                    />

                    <a-form-item label="MCP Server Label">
                      <a-input v-model:value="form.server_label" />
                    </a-form-item>

                    <a-form-item label="MCP Server URL">
                      <a-input v-model:value="form.server_url" placeholder="https://your-mcp-server.example.com/mcp" />
                    </a-form-item>

                    <a-form-item label="Allowed Tools">
                      <a-textarea
                        v-model:value="form.allowed_tools_text"
                        :rows="3"
                        placeholder="一行一个，或逗号分隔。例如：list_pages, take_snapshot"
                      />
                      <small class="field-help">留空表示不额外限制，直接让服务端按默认策略暴露工具。</small>
                    </a-form-item>

                    <a-form-item label="Require Approval">
                      <a-select v-model:value="form.require_approval" :options="requireApprovalOptions" />
                    </a-form-item>
                  </template>
                </a-form>
              </div>

              <div class="card-actions card-actions--left">
                <a-button type="primary" :loading="loading" @click="submit">
                  {{ mode === 'chat' ? '发送对话请求' : '发送 MCP 测试请求' }}
                </a-button>
              </div>
            </div>
          </a-card>
        </a-col>

        <a-col :xs="24" :xl="12">
          <a-card :bordered="false" class="admin-card admin-card--fill" title="响应">
            <div class="card-scroll card-scroll--padded">
              <div v-if="result" class="result-block">
                <p><strong>状态：</strong>{{ result.message }}</p>
                <p><strong>状态码：</strong>{{ result.status_code ?? '无' }}</p>
                <p><strong>耗时：</strong>{{ result.latency_ms ?? '无' }} ms</p>

                <template v-if="mcpResult">
                  <div>
                    <strong>工具事件：</strong>
                    <div class="tag-list">
                      <a-tag v-for="item in mcpResult.tool_events" :key="item" color="geekblue">{{ item }}</a-tag>
                      <a-tag v-if="!mcpResult.tool_events.length">未识别到工具调用事件</a-tag>
                    </div>
                  </div>

                  <div>
                    <strong>模型输出：</strong>
                    <pre class="response-preview">{{ mcpResult.output || '未提取到可读输出。' }}</pre>
                  </div>

                  <div>
                    <strong>原始响应摘要：</strong>
                    <pre class="response-preview response-preview--light">{{ mcpResult.raw_excerpt || '无原始响应内容。' }}</pre>
                  </div>
                </template>

                <template v-else>
                  <pre class="response-preview">{{ result.output || '接口未返回可读文本。' }}</pre>
                </template>
              </div>
              <a-empty v-else :description="mode === 'chat' ? '等待发送对话测试。' : '等待发送 MCP 测试请求。'" />
            </div>
          </a-card>
        </a-col>
      </a-row>
    </div>
  </ShellLayout>
</template>
