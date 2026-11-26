export function useBackend() {
  const account = useState<'tom' | 'mint' | 'beam'>('account', () => 'tom')
  const token = computed(() => 'dev-' + account.value)
  async function api<T = any>(path: string, options: { method?: string; body?: unknown } = {}): Promise<T> {
    return await $fetch<T>('/api/' + path, {
      method: (options.method || 'GET') as any,
      headers: { Authorization: 'Bearer ' + token.value },
      body: options.body,
    })
  }
  return { account, api }
}
