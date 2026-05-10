export type SiteApiKeyRecord = Record<string, unknown>

export function apiKeyEntryValue(item: SiteApiKeyRecord | undefined, key: string): string {
  const value = String(item?.[key] ?? '').trim()
  return value === '<nil>' ? '' : value
}

export function normalizeStringList(values: unknown): string[] {
  if (Array.isArray(values)) {
    return values
      .flatMap((item) => normalizeStringList(item))
      .filter((item, index, source) => item && source.indexOf(item) === index)
  }
  return String(values ?? '')
    .split(/[\n\r,，\t]+/)
    .map((item) => item.trim())
    .filter((item, index, source) => item && source.indexOf(item) === index)
}

export function apiKeyRequestBaseURLs(item: SiteApiKeyRecord | undefined): string[] {
  return normalizeStringList([
    item?.request_base_urls,
    item?.request_base_url,
    item?.api_request_urls,
    item?.api_request_url,
    item?.gateway_request_urls,
    item?.gateway_request_url,
    item?.endpoint_url,
    item?.base_url,
    item?.baseURL,
    item?.api_base_url,
    item?.apiBaseUrl,
    item?.api_url,
    item?.apiUrl,
  ])
}

export function apiKeyImageGenerationPath(item: SiteApiKeyRecord | undefined): string {
  return apiKeyEntryValue(item, 'image_generation_path')
}

export function apiKeyImageEditPath(item: SiteApiKeyRecord | undefined): string {
  return apiKeyEntryValue(item, 'image_edit_path')
}

export function apiKeyEntryRouteType(item: SiteApiKeyRecord | undefined): string {
  const normalized = apiKeyEntryValue(item, 'route_type')
    || apiKeyEntryValue(item, 'api_type')
    || apiKeyEntryValue(item, 'api_format')
    || apiKeyEntryValue(item, 'type')
  return normalized.toLowerCase()
}

export function equivalentApiKeyEntryExists(entries: SiteApiKeyRecord[], candidate: SiteApiKeyRecord): boolean {
  const signature = apiKeyEntryConfigSignature(candidate)
  return entries.some((entry) => apiKeyEntryConfigSignature(entry) === signature)
}

function apiKeyEntryConfigSignature(item: SiteApiKeyRecord): string {
  return [
    apiKeyEntryValue(item, 'key'),
    apiKeyEntryRouteType(item),
    apiKeyRequestBaseURLs(item).join('\n'),
    apiKeyImageGenerationPath(item),
    apiKeyImageEditPath(item),
  ].join('\x00')
}

export function setApiKeyRequestBaseURLs(
  credentials: Record<string, unknown>,
  key: string,
  urls: unknown,
  entryIndex?: number,
): Record<string, unknown> {
  const normalizedKey = key.trim()
  if (!normalizedKey) {
    return credentials
  }
  const normalizedURLs = normalizeStringList(urls)
  const entries = storedApiKeyEntries(credentials).map((item, index) => {
    if (!apiKeyEntryMatches(item, index, normalizedKey, entryIndex)) {
      return item
    }
    const next: SiteApiKeyRecord = {
      ...item,
      request_base_urls: normalizedURLs,
    }
    for (const alias of [
      'request_base_url',
      'api_request_urls',
      'api_request_url',
      'gateway_request_urls',
      'gateway_request_url',
      'endpoint_url',
      'base_url',
      'baseURL',
      'api_base_url',
      'apiBaseUrl',
      'api_url',
      'apiUrl',
    ]) {
      delete next[alias]
    }
    if (normalizedURLs.length === 0) {
      delete next.request_base_urls
    }
    return next
  })
  return {
    ...credentials,
    api_keys: entries,
  }
}

export function setApiKeyImagePaths(
  credentials: Record<string, unknown>,
  key: string,
  imageGenerationPath: unknown,
  imageEditPath: unknown,
  entryIndex?: number,
): Record<string, unknown> {
  const normalizedKey = key.trim()
  if (!normalizedKey) {
    return credentials
  }
  const generationPath = String(imageGenerationPath ?? '').trim()
  const editPath = String(imageEditPath ?? '').trim()
  const entries = storedApiKeyEntries(credentials).map((item, index) => {
    if (!apiKeyEntryMatches(item, index, normalizedKey, entryIndex)) {
      return item
    }
    const next: SiteApiKeyRecord = { ...item }
    if (generationPath) {
      next.image_generation_path = generationPath
    } else {
      delete next.image_generation_path
    }
    if (editPath) {
      next.image_edit_path = editPath
    } else {
      delete next.image_edit_path
    }
    return next
  })
  return {
    ...credentials,
    api_keys: entries,
  }
}

export function isManualApiKeyEntry(item: SiteApiKeyRecord): boolean {
  const source = apiKeyEntryValue(item, 'source').toLowerCase()
  return source === 'manual' || source === 'custom' || source === 'user' || item?.is_custom === true || item?.manual === true
}

export function storedApiKeyEntries(credentials: Record<string, unknown> | undefined): SiteApiKeyRecord[] {
  const raw = credentials?.api_keys
  if (!Array.isArray(raw)) {
    return []
  }
  return raw
    .map((item) => (item && typeof item === 'object' ? { ...(item as SiteApiKeyRecord) } : null))
    .filter((item): item is SiteApiKeyRecord => Boolean(item && apiKeyEntryValue(item, 'key')))
}

function apiKeyEntryIdentity(item: SiteApiKeyRecord, index: number): string {
  const id = apiKeyEntryValue(item, 'id')
  if (id) {
    return `id:${id}`
  }
  const signature = apiKeyEntryConfigSignature(item)
  return signature ? `config:${signature}` : `key:${apiKeyEntryValue(item, 'key')}:index:${index}`
}

function apiKeyEntryMatches(item: SiteApiKeyRecord, index: number, key: string, entryIndex?: number): boolean {
  if (apiKeyEntryValue(item, 'key') !== key) {
    return false
  }
  return entryIndex === undefined || index === entryIndex
}

export function mergeApiKeyEntries(entries: SiteApiKeyRecord[]): SiteApiKeyRecord[] {
  const seen = new Set<string>()
  const out: SiteApiKeyRecord[] = []
  for (const [index, entry] of entries.entries()) {
    const key = apiKeyEntryValue(entry, 'key')
    const identity = apiKeyEntryIdentity(entry, index)
    if (!key || seen.has(identity)) {
      continue
    }
    seen.add(identity)
    out.push({ ...entry, key })
  }
  return out
}

export function apiKeyValue(credentials: Record<string, unknown> | undefined): string {
  const value = credentials?.api_key
  return typeof value === 'string' ? value.trim() : String(value ?? '').trim()
}

export function removeSiteApiKeyCredential(
  credentials: Record<string, unknown>,
  key: string,
  entryIndex?: number,
): Record<string, unknown> {
  const normalizedKey = key.trim()
  if (!normalizedKey) {
    return credentials
  }
  const next = storedApiKeyEntries(credentials).filter((item, index) => !apiKeyEntryMatches(item, index, normalizedKey, entryIndex))
  const updated: Record<string, unknown> = {
    ...credentials,
    api_keys: next,
  }
  if (apiKeyValue(credentials) === normalizedKey) {
    updated.api_key = apiKeyEntryValue(next[0], 'key')
  }
  return updated
}
