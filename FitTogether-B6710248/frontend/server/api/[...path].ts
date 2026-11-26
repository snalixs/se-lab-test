import { uploadBackend } from '../utils/uploadBackend'

export default defineEventHandler(async (event) => {
  const path = getRouterParam(event, 'path') || ''
  const config = useRuntimeConfig(event)
  const method = getMethod(event)
  const headers: Record<string, string> = {
    authorization: getHeader(event, 'authorization') || '',
  }
  const contentType = getHeader(event, 'content-type')
  if (contentType) headers['content-type'] = contentType
  let base = config.apiBase
  if (method === 'POST' && path === 'posts' && contentType?.startsWith('multipart/form-data')) {
    const updated = await uploadBackend(base)
    if (!updated) {
      throw createError({ statusCode: 503, statusMessage: 'Backend ยังไม่รองรับการแนบรูป กรุณาหยุดเซิร์ฟเวอร์เดิมแล้วรัน .\\start-dev.ps1 ใหม่' })
    }
    base = updated
  }
  const url = new URL('/api/' + path, base)
  url.search = getRequestURL(event).search
  const body = method === 'GET' || method === 'HEAD' ? undefined : await readRawBody(event, false)
  try {
    const response = await fetch(url, { method, headers, body })
    setResponseStatus(event, response.status)
    const payload = await response.text()
    if (payload) setHeader(event, 'content-type', response.headers.get('content-type') || 'application/json')
    return payload || undefined
  } catch {
    throw createError({ statusCode: 502, statusMessage: 'เชื่อมต่อ Backend ไม่ได้ ตรวจว่า Go API ทำงานที่พอร์ต 8080' })
  }
})
