<script setup lang="ts">
import { computed, onMounted, reactive, ref } from 'vue'
import { getSites, testConnectivity } from '../api'
import ShellLayout from '../components/ShellLayout.vue'
import { formatGroupNames } from '../format'
import { useToast } from '../toast'
import type { ConnectivityResult, Site } from '../types'

const form = reactive({
  base_url: 'https://api.openai.com/v1',
  api_key: '',
  model: 'gpt-4o-mini',
})

const toast = useToast()
const sites = ref<Site[]>([])
const selectedSiteId = ref<string>()
const loading = ref(false)
const result = ref<ConnectivityResult | null>(null)

const selectedSite = computed(() =>
  sites.value.find((item) => String(item.id) === selectedSiteId.value) ?? null,
)

const siteOptions = computed(() =>
  sites.value.map((site) => ({
    label: `${site.name} · ${site.plugin_key}`,
    value: String(site.id),
  })),
)

function applySelectedSite() {
  if (!selectedSite.value) {
    return
  }
  form.base_url = selectedSite.value.base_url
  form.api_key = selectedSite.value.credentials.api_key ?? ''
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
    result.value = await testConnectivity(form)
    toast.success(result.value.ok ? '连接测试完成。' : result.value.message)
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
          <a-card :bordered="false" class="admin-card admin-card--fill" title="接口检测">
            <div class="card-form">
              <div class="card-scroll card-scroll--padded">
                <a-form layout="vertical">
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
                  <a-form-item label="参考模型名">
                    <a-input v-model:value="form.model" />
                  </a-form-item>
                </a-form>
              </div>
              <div class="card-actions card-actions--left">
                <a-button type="primary" :loading="loading" @click="submit">开始检测</a-button>
              </div>
            </div>
          </a-card>
        </a-col>

        <a-col :xs="24" :xl="12">
          <a-card :bordered="false" class="admin-card admin-card--fill" title="检测结果">
            <div class="card-scroll card-scroll--padded">
              <div v-if="result" class="result-block">
                <p><strong>结果：</strong>{{ result.message }}</p>
                <p><strong>状态码：</strong>{{ result.status_code ?? '无' }}</p>
                <p><strong>耗时：</strong>{{ result.latency_ms ?? '无' }} ms</p>
                <div>
                  <strong>模型列表：</strong>
                  <div class="tag-list">
                    <a-tag v-for="model in result.models" :key="model" color="blue">{{ model }}</a-tag>
                    <a-tag v-if="!result.models.length">未返回模型列表</a-tag>
                  </div>
                </div>
              </div>
              <a-empty v-else description="等待执行连接测试。" />
            </div>
          </a-card>
        </a-col>
      </a-row>
    </div>
  </ShellLayout>
</template>
