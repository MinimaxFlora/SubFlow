export interface Subscription {
  id: string
  name: string
  displayName: string
  source: 'remote' | 'local'
  url: string
  enabled: boolean
  sort: number
  process: unknown
  createdAt: string
  updatedAt: string
}

export interface SubscriptionInput {
  name: string
  displayName: string
  source: 'remote' | 'local'
  url: string
  content?: string
  enabled: boolean
  sort: number
}

export interface HealthInfo {
  status: string
  time: string
}

export interface VersionInfo {
  version: string
  commit: string
  buildTime: string
}

export interface Platform {
  id: string
  name: string
  extension: string
  group: string
}

export type OperationType =
  | 'Filter'
  | 'Rename'
  | 'Deduplicate'
  | 'Sort'
  | 'UselessFilter'
  | 'SetProperty'

export interface Operation {
  type: OperationType
  args: Record<string, unknown>
}

export interface ConvertResult {
  output: string
  platform: string
  total: number
  emitted: number
  source?: string
  warnings?: string[]
}

export interface ConvertInput {
  content: string
  url?: string
  userAgent?: string
  platform: string
  operations: Operation[]
  includeUnsupported?: boolean
}

export interface FetchResult {
  finalUrl: string
  contentType: string
  bytes: number
  nodes: number
  preview: string
  parseError: string
}

export interface FetchInput {
  url: string
  userAgent?: string
}

export interface SubscriptionContent {
  content: string
  nodes: number
  source: string
  parseError: string
}

interface Envelope<T> {
  data: T | null
  error: { code: string; message: string } | null
}

async function request<T>(path: string, init?: RequestInit): Promise<T> {
  const response = await fetch(path, {
    headers: { 'Content-Type': 'application/json' },
    ...init,
  })
  let body: Envelope<T> | null = null
  try {
    body = (await response.json()) as Envelope<T>
  } catch {
    body = null
  }
  if (!response.ok || (body && body.error)) {
    const message = body?.error?.message ?? `request failed with status ${response.status}`
    throw new Error(message)
  }
  return body?.data as T
}

export const api = {
  health: () => request<HealthInfo>('/api/health'),
  version: () => request<VersionInfo>('/api/version'),
  listSubscriptions: () => request<Subscription[]>('/api/subscriptions'),
  createSubscription: (input: SubscriptionInput) =>
    request<Subscription>('/api/subscriptions', {
      method: 'POST',
      body: JSON.stringify(input),
    }),
  updateSubscription: (id: string, input: SubscriptionInput) =>
    request<Subscription>(`/api/subscriptions/${id}`, {
      method: 'PUT',
      body: JSON.stringify(input),
    }),
  deleteSubscription: (id: string) =>
    request<{ id: string }>(`/api/subscriptions/${id}`, { method: 'DELETE' }),
  subscriptionContent: (id: string) =>
    request<SubscriptionContent>(`/api/subscriptions/${id}/content`),
  listPlatforms: () => request<Platform[]>('/api/platforms'),
  convert: (input: ConvertInput) =>
    request<ConvertResult>('/api/convert', {
      method: 'POST',
      body: JSON.stringify(input),
    }),
  fetchSubscription: (input: FetchInput) =>
    request<FetchResult>('/api/fetch', {
      method: 'POST',
      body: JSON.stringify(input),
    }),
  listSettings: () => request<Record<string, string>>('/api/settings'),
  updateSettings: (input: Record<string, string>) =>
    request<Record<string, string>>('/api/settings', {
      method: 'PUT',
      body: JSON.stringify(input),
    }),
}
