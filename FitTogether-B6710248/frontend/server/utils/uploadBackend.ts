async function supportsImageUpload(base: string): Promise<boolean> {
  try {
    const response = await fetch(new URL('/healthz', base), { signal: AbortSignal.timeout(2000) })
    if (!response.ok) return false
    const health = await response.json() as { image_upload?: string }
    return health.image_upload === 'multipart-10'
  } catch {
    return false
  }
}

export async function uploadBackend(base: string): Promise<string | null> {
  if (await supportsImageUpload(base)) return base

  const url = new URL(base)
  if ((url.hostname === '127.0.0.1' || url.hostname === 'localhost') && url.port === '8080') {
    url.port = '8081'
    const alternate = url.toString()
    if (await supportsImageUpload(alternate)) return alternate
  }
  return null
}
