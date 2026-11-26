<script setup lang="ts">
import { House, Search, UsersRound, Bell, Bookmark, Plus, Send, X, ChevronRight, ChevronLeft, ArrowLeft, CalendarDays, MapPin, LogOut, UserPlus, Check, ImagePlus, Heart, MessageCircle } from 'lucide-vue-next'

const { account, api } = useBackend()
const accountIds = { tom: 1, mint: 2, beam: 3 }
const names = { tom: 'Tom', mint: 'Mint', beam: 'Beam' }
const section = ref<'feed' | 'search' | 'groups' | 'saved' | 'notifications'>('feed')
const filter = ref('latest')
const query = ref('')
const posts = ref<any[]>([])
const groups = ref<any[]>([])
const notices = ref<any[]>([])
const invites = ref<any[]>([])
const selectedGroup = ref<any>(null)
const members = ref<any[]>([])
const requests = ref<any[]>([])
const messages = ref<any[]>([])
const events = ref<any[]>([])
const commentPost = ref<any>(null)
const comments = ref<any[]>([])
const commentText = ref('')
const editingCommentId = ref<number | null>(null)
const editingCommentText = ref('')
const commentLoading = ref(false)
const commentHasMore = ref(false)
const commentOffset = ref(0)
const detailImageIndex = ref(0)
const detailGallery = ref<HTMLElement | null>(null)
const detailClose = ref<HTMLButtonElement | null>(null)
let detailTouchStartX = 0
let detailTouchStartY = 0
const messageText = ref('')
const dialog = ref<'post' | 'group' | 'event' | 'invite' | null>(null)
const busy = ref(false)
const loaded = ref(false)
const error = ref('')
const notice = ref('')
const groupScope = ref('')
const postForm = reactive({ content: '', visibility: 'public', hashtags: '' })
const postFileInput = ref<HTMLInputElement | null>(null)
const postPhotos = ref<Array<{ file: File; preview: string }>>([])
const groupForm = reactive({ name: '', description: '', sport: '', visibility: 'open', max_members: 20, cover_url: '' })
const eventForm = reactive({ title: '', sport: '', location: '', starts_at: '' })
const inviteId = ref('')
const currentId = computed(() => accountIds[account.value])

function clearPostPhotos() {
  for (const photo of postPhotos.value) URL.revokeObjectURL(photo.preview)
  postPhotos.value = []
}
function addPostPhotos(files: File[]) {
  const accepted = new Set(['image/jpeg', 'image/png', 'image/webp'])
  for (const file of files) {
    if (postPhotos.value.length >= 10) {
      error.value = 'แนบรูปได้สูงสุด 10 รูป'
      break
    }
    if (!accepted.has(file.type) || file.size === 0 || file.size > 8 * 1024 * 1024) {
      error.value = 'รองรับรูป JPG, PNG, WebP ขนาดไม่เกิน 8 MB ต่อรูป'
      continue
    }
    postPhotos.value.push({ file, preview: URL.createObjectURL(file) })
  }
}
function choosePostPhotos(event: Event) {
  const input = event.target as HTMLInputElement
  addPostPhotos(Array.from(input.files || []))
  input.value = ''
}
function pastePostPhotos(event: ClipboardEvent) {
  if (dialog.value !== 'post') return
  const files = Array.from(event.clipboardData?.files || []).filter(file => file.type.startsWith('image/'))
  if (!files.length) return
  event.preventDefault()
  addPostPhotos(files)
}
function removePostPhoto(index: number) {
  const [removed] = postPhotos.value.splice(index, 1)
  if (removed) URL.revokeObjectURL(removed.preview)
}
watch(dialog, (next, previous) => {
  if (previous === 'post' && next !== 'post') clearPostPhotos()
})
onBeforeUnmount(clearPostPhotos)

function showError(e: any) {
  error.value = e?.data?.error || e?.data?.message || e?.statusMessage || 'เกิดข้อผิดพลาด กรุณาลองอีกครั้ง'
}
async function run(action: () => Promise<void>) {
  busy.value = true
  error.value = ''
  try { await action() } catch (e) { showError(e) } finally { busy.value = false }
}
async function loadPosts() {
  const mode = section.value === 'saved' ? 'bookmarked' : section.value === 'search' ? 'latest' : filter.value
  const params = new URLSearchParams({ mode })
  if (query.value.trim()) params.set('q', query.value.trim())
  posts.value = (await api<{ items: any[] }>('posts?' + params)).items
}
async function loadGroups() {
  const params = new URLSearchParams()
  if (query.value.trim()) params.set('q', query.value.trim())
  if (groupScope.value) params.set('scope', groupScope.value)
  groups.value = (await api<{ items: any[] }>('groups?' + params)).items
}
async function loadNotices() {
  notices.value = (await api<{ items: any[] }>('group-notifications')).items
  invites.value = (await api<{ items: any[] }>('group-invites')).items
}
async function refresh() {
  loaded.value = false
  try {
    if (section.value === 'groups') await loadGroups()
    else if (section.value === 'notifications') await loadNotices()
    else await loadPosts()
  } finally {
    loaded.value = true
  }
}
async function openGroup(group: any) {
  await run(async () => {
    selectedGroup.value = await api('groups/' + group.id)
    await loadGroupDetails()
  })
}
async function loadGroupDetails() {
  if (!selectedGroup.value) return
  const id = selectedGroup.value.id
  selectedGroup.value = await api('groups/' + id)
  if (!selectedGroup.value.is_member) return
  const [m, chat, ev] = await Promise.all([
    api<{ items: any[] }>('groups/' + id + '/members'),
    api<{ items: any[] }>('groups/' + id + '/messages'),
    api<{ items: any[] }>('groups/' + id + '/events')
  ])
  members.value = m.items
  messages.value = chat.items.slice().reverse()
  events.value = ev.items
  requests.value = selectedGroup.value.owner_id === currentId.value
    ? (await api<{ items: any[] }>('groups/' + id + '/join-requests')).items : []
}
async function togglePost(post: any, kind: 'like' | 'bookmark') {
  await run(async () => {
    await api('posts/' + post.id + '/' + kind, { method: post[kind === 'like' ? 'liked' : 'bookmarked'] ? 'DELETE' : 'PUT' })
    await loadPosts()
    if (commentPost.value?.id === post.id) commentPost.value = posts.value.find(item => item.id === post.id) || commentPost.value
  })
}
async function sharePost(post: any) {
  await run(async () => {
    await api('posts/' + post.id + '/share', { method: 'POST', body: { content: '' } })
    notice.value = 'แชร์โพสต์แล้ว'
    await loadPosts()
  })
}
async function savePost() {
  await run(async () => {
    const body = new FormData()
    body.set('content', postForm.content)
    body.set('visibility', postForm.visibility)
    for (const tag of postForm.hashtags.split(/[\s,]+/).map(s => s.replace(/^#/, '').toLowerCase()).filter(Boolean)) {
      body.append('hashtags', tag)
    }
    for (const photo of postPhotos.value) body.append('images', photo.file, photo.file.name)
    await api('posts', { method: 'POST', body })
    dialog.value = null
    Object.assign(postForm, { content: '', visibility: 'public', hashtags: '' })
    section.value = 'feed'
    filter.value = 'latest'
    await loadPosts()
  })
}
async function openComments(post: any, imageIndex = 0) {
  editingCommentId.value = null
  commentPost.value = post
  detailImageIndex.value = imageIndex
  comments.value = []
  commentOffset.value = 0
  commentText.value = ''
  commentLoading.value = true
  commentHasMore.value = false
  await nextTick()
  ;(detailGallery.value || detailClose.value)?.focus()
  try {
    const items = (await api<{ items: any[] }>('posts/' + post.id + '/comments?limit=25')).items
    if (commentPost.value?.id === post.id) {
      comments.value = items
      commentOffset.value = items.length
      commentHasMore.value = items.length === 25
    }
  } catch (e) { showError(e) } finally { commentLoading.value = false }
}
function editComment(comment: any) {
  editingCommentId.value = comment.id
  editingCommentText.value = comment.content
}
async function saveComment(comment: any) {
  if (!commentPost.value || busy.value || !editingCommentText.value.trim()) return
  const postId = commentPost.value.id
  const content = editingCommentText.value.trim()
  await run(async () => {
    await api(`posts/${postId}/comments/${comment.id}`, { method: 'PATCH', body: { content } })
    if (commentPost.value?.id === postId) {
      comment.content = content
      comment.updated_at = new Date().toISOString()
      editingCommentId.value = null
    }
  })
}
async function removeComment(comment: any) {
  if (!commentPost.value || busy.value || !window.confirm('ลบความคิดเห็นนี้? คำตอบของคนอื่นจะยังอยู่')) return
  const post = commentPost.value
  await run(async () => {
    await api(`posts/${post.id}/comments/${comment.id}`, { method: 'DELETE' })
    await loadPosts()
    if (commentPost.value?.id === post.id) {
      await openComments(posts.value.find(item => item.id === post.id) || { ...post, comment_count: Math.max(0, post.comment_count - 1) })
    }
  })
}
async function removePost(post: any) {
  if (busy.value || !window.confirm('ลบโพสต์นี้? โพสต์จะถูกนำออกและเก็บสำรองไว้ โพสต์ที่คนอื่นแชร์จะยังอยู่')) return
  await run(async () => {
    await api(`posts/${post.id}`, { method: 'DELETE' })
    if (commentPost.value?.id === post.id) closePostDetail()
    await loadPosts()
  })
}
function closePostDetail() {
  commentPost.value = null
  commentText.value = ''
}
function moveDetailImage(direction: number) {
  const count = commentPost.value?.images?.length || 0
  if (count > 1) detailImageIndex.value = (detailImageIndex.value + direction + count) % count
}
function startDetailTouch(event: TouchEvent) {
  detailTouchStartX = event.changedTouches[0]?.clientX || 0
  detailTouchStartY = event.changedTouches[0]?.clientY || 0
}
function endDetailTouch(event: TouchEvent) {
  const touch = event.changedTouches[0]
  if (!touch) return
  const dx = touch.clientX - detailTouchStartX
  const dy = touch.clientY - detailTouchStartY
  if (Math.abs(dx) > 45 && Math.abs(dx) > Math.abs(dy)) moveDetailImage(dx < 0 ? 1 : -1)
}
async function loadMoreComments() {
  if (!commentPost.value || commentLoading.value || !commentHasMore.value) return
  const postId = commentPost.value.id
  commentLoading.value = true
  try {
    const items = (await api<{ items: any[] }>(`posts/${postId}/comments?limit=25&offset=${commentOffset.value}`)).items
    if (commentPost.value?.id === postId) {
      const known = new Set(comments.value.map(item => item.id))
      comments.value.push(...items.filter(item => !known.has(item.id)))
      commentOffset.value += items.length
      commentHasMore.value = items.length === 25
    }
  } catch (e) { showError(e) } finally { commentLoading.value = false }
}
async function sendComment() {
  if (!commentPost.value || !commentText.value.trim()) return
  const postId = commentPost.value.id
  await run(async () => {
    const created: any = await api('posts/' + postId + '/comments', { method: 'POST', body: { content: commentText.value.trim() } })
    commentText.value = ''
    if (commentPost.value?.id === postId) {
      comments.value.push({ ...created, display_name: names[account.value] })
    }
    await loadPosts()
    if (commentPost.value?.id === postId) commentPost.value = posts.value.find(item => item.id === postId) || commentPost.value
  })
}
async function createGroup() {
  await run(async () => {
    const created: any = await api('groups', { method: 'POST', body: {
      ...groupForm, cover_url: groupForm.cover_url || null
    } })
    dialog.value = null
    Object.assign(groupForm, { name: '', description: '', sport: '', visibility: 'open', max_members: 20, cover_url: '' })
    section.value = 'groups'
    await loadGroups()
    await openGroup(created)
  })
}
async function joinGroup() {
  if (!selectedGroup.value) return
  await run(async () => {
    await api('groups/' + selectedGroup.value.id + '/join', { method: 'POST' })
    await loadGroupDetails()
    notice.value = selectedGroup.value.is_member ? 'เข้าร่วมกลุ่มแล้ว' : 'ส่งคำขอเข้าร่วมแล้ว'
  })
}
async function leaveGroup() {
  if (!selectedGroup.value) return
  await run(async () => {
    await api('groups/' + selectedGroup.value.id + '/membership', { method: 'DELETE' })
    selectedGroup.value = null
    await loadGroups()
  })
}
async function reviewRequest(request: any, status: 'approved' | 'rejected') {
  await run(async () => {
    await api('groups/' + selectedGroup.value.id + '/join-requests/' + request.id, { method: 'PUT', body: { status } })
    await loadGroupDetails()
  })
}
async function sendInvite() {
  await run(async () => {
    await api('groups/' + selectedGroup.value.id + '/invites', { method: 'POST', body: { user_id: Number(inviteId.value) } })
    dialog.value = null
    inviteId.value = ''
    notice.value = 'ส่งคำเชิญแล้ว'
  })
}
async function replyInvite(invite: any, status: 'accepted' | 'declined') {
  await run(async () => {
    await api('groups/' + invite.group_id + '/invites/' + invite.id, { method: 'PUT', body: { status } })
    await loadNotices()
    await loadGroups()
  })
}
async function sendMessage() {
  if (!messageText.value.trim()) return
  await run(async () => {
    await api('groups/' + selectedGroup.value.id + '/messages', { method: 'POST', body: { content: messageText.value.trim() } })
    messageText.value = ''
    await loadGroupDetails()
  })
}
async function createEvent() {
  await run(async () => {
    await api('groups/' + selectedGroup.value.id + '/events', { method: 'POST', body: {
      ...eventForm, starts_at: new Date(eventForm.starts_at).toISOString()
    } })
    dialog.value = null
    Object.assign(eventForm, { title: '', sport: '', location: '', starts_at: '' })
    await loadGroupDetails()
  })
}
async function rsvp(event: any, response: 'going' | 'not_going') {
  await run(async () => {
    await api('groups/' + selectedGroup.value.id + '/events/' + event.id + '/rsvp', { method: 'PUT', body: { response } })
    await loadGroupDetails()
  })
}
async function markRead(item: any) {
  await run(async () => {
    await api('group-notifications/' + item.id + '/read', { method: 'PUT' })
    await loadNotices()
  })
}
function go(to: typeof section.value) {
  section.value = to
  selectedGroup.value = null
  query.value = ''
  error.value = ''
  refresh().catch(showError)
}
watch(account, () => { selectedGroup.value = null; refresh().catch(showError) })
watch(filter, () => { if (section.value === 'feed') loadPosts().catch(showError) })
onMounted(() => refresh().catch(showError))
function date(value: string) { return new Intl.DateTimeFormat('th-TH', { dateStyle: 'medium', timeStyle: 'short' }).format(new Date(value)) }
</script>

<template>
  <div class="app-shell">
    <aside class="sidebar">
      <div class="brand"><span class="brand-symbol">↗</span><span class="nav-label brand-name">FitTogether</span></div>
      <nav class="nav">
        <button :class="{ active: section === 'feed' }" @click="go('feed')"><House :size="23"/><span class="nav-label">หน้าฟีด</span></button>
        <button :class="{ active: section === 'search' }" @click="go('search')"><Search :size="23"/><span class="nav-label">ค้นหาโพสต์</span></button>
        <button :class="{ active: section === 'groups' }" @click="go('groups')"><UsersRound :size="23"/><span class="nav-label">กลุ่ม</span></button>
        <button :class="{ active: section === 'notifications' }" @click="go('notifications')"><Bell :size="23"/><span class="nav-label">แจ้งเตือน</span></button>
        <button :class="{ active: section === 'saved' }" @click="go('saved')"><Bookmark :size="23"/><span class="nav-label">ที่บันทึกไว้</span></button>
        <button @click="dialog = section === 'groups' ? 'group' : 'post'"><Plus :size="23"/><span class="nav-label">สร้างใหม่</span></button>
      </nav>
      <div class="sidebar-bottom">
        <div class="account-icon">{{ names[account][0] }}</div>
        <label class="nav-label account-switch"><span>{{ names[account] }}<small>บัญชีทดลอง</small></span><select v-model="account" aria-label="บัญชีทดลอง"><option value="tom">Tom</option><option value="mint">Mint</option><option value="beam">Beam</option></select></label>
      </div>
    </aside>

    <main class="workspace">
      <div v-if="error" class="alert" role="alert">{{ error }} <button @click="error = ''"><X :size="16"/></button></div>
      <div v-if="notice" class="toast" @click="notice = ''">{{ notice }}</div>

      <template v-if="selectedGroup">
        <header class="page-top"><button class="back" @click="selectedGroup = null; loadGroups()"><ArrowLeft :size="20"/> กลุ่มทั้งหมด</button><button v-if="selectedGroup.owner_id === currentId" class="text-action" @click="dialog = 'invite'"><UserPlus :size="18"/> เชิญสมาชิก</button></header>
        <div class="group-hero" :style="selectedGroup.cover_url ? { backgroundImage: `linear-gradient(90deg,rgba(22,20,19,.95),rgba(22,20,19,.35)),url(${selectedGroup.cover_url})` } : {}">
          <div class="eyebrow">{{ selectedGroup.sport }} · {{ selectedGroup.visibility === 'open' ? 'กลุ่มเปิด' : selectedGroup.visibility === 'closed' ? 'ขออนุมัติก่อนเข้าร่วม' : 'กลุ่มส่วนตัว' }}</div>
          <h1>{{ selectedGroup.name }}</h1><p>{{ selectedGroup.description }}</p>
          <div class="group-hero-bottom"><span><UsersRound :size="16"/> {{ selectedGroup.member_count }} / {{ selectedGroup.max_members }} คน</span><button v-if="!selectedGroup.is_member && selectedGroup.visibility !== 'private'" class="primary" @click="joinGroup">เข้าร่วมกลุ่ม</button><button v-else-if="selectedGroup.is_member && selectedGroup.owner_id !== currentId" class="ghost" @click="leaveGroup"><LogOut :size="16"/> ออกจากกลุ่ม</button></div>
        </div>
        <div v-if="selectedGroup.is_member" class="group-layout">
          <div class="group-main">
            <section class="section"><div class="section-head"><h2>กิจกรรมที่นัดไว้</h2><button v-if="selectedGroup.owner_id === currentId" class="text-action" @click="dialog = 'event'"><Plus :size="18"/> สร้างกิจกรรม</button></div>
              <p v-if="!events.length" class="empty">ยังไม่มีกิจกรรมในกลุ่ม</p>
              <article v-for="event in events" :key="event.id" class="event-row"><div class="event-date"><b>{{ new Date(event.starts_at).getDate() }}</b><span>{{ new Intl.DateTimeFormat('th-TH', { month: 'short' }).format(new Date(event.starts_at)) }}</span></div><div class="event-info"><h3>{{ event.title }}</h3><p><CalendarDays :size="15"/> {{ date(event.starts_at) }}</p><p><MapPin :size="15"/> {{ event.location }}</p><div class="event-actions"><button :class="{ selected: event.my_response === 'going' }" @click="rsvp(event,'going')">เข้าร่วม</button><button :class="{ selected: event.my_response === 'not_going' }" @click="rsvp(event,'not_going')">ไม่เข้าร่วม</button><span>{{ event.going_count }} คนจะไป</span></div></div></article>
            </section>
            <section class="section"><div class="section-head"><h2>พูดคุยในกลุ่ม</h2><span class="subtle">{{ messages.length }} ข้อความล่าสุด</span></div><p v-if="!messages.length" class="empty">เริ่มบทสนทนาแรกของกลุ่ม</p><div class="chat-list"><div v-for="message in messages" :key="message.id" class="chat-message"><span class="avatar">{{ (message.display_name || 'M')[0] }}</span><div><b>{{ message.display_name || 'สมาชิก' }}</b><time>{{ date(message.created_at) }}</time><p>{{ message.content }}</p></div></div></div><form class="send-box" @submit.prevent="sendMessage"><input v-model="messageText" maxlength="500" placeholder="ส่งข้อความถึงสมาชิกในกลุ่ม" aria-label="ข้อความ"/><button :disabled="busy || !messageText.trim()" aria-label="ส่งข้อความ"><Send :size="18"/></button></form></section>
          </div>
          <aside class="context-panel"><h3>สมาชิก <span>{{ members.length }}</span></h3><div v-for="member in members" :key="member.user_id" class="member-row"><span class="avatar">{{ (member.display_name || 'M')[0] }}</span><span>{{ member.display_name || 'สมาชิก' }}<small v-if="member.user_id === selectedGroup.owner_id">ผู้ดูแลกลุ่ม</small></span></div><template v-if="selectedGroup.owner_id === currentId"><h3 class="requests-title">คำขอเข้าร่วม</h3><p v-if="!requests.length" class="subtle">ไม่มีคำขอที่รออนุมัติ</p><div v-for="request in requests.filter((r:any) => r.status === 'pending')" :key="request.id" class="request-row"><span>สมาชิก #{{ request.requester_id }}</span><div><button aria-label="อนุมัติ" @click="reviewRequest(request,'approved')"><Check :size="16"/></button><button aria-label="ปฏิเสธ" @click="reviewRequest(request,'rejected')"><X :size="16"/></button></div></div></template></aside>
        </div><div v-else class="locked-note">เข้าร่วมกลุ่มเพื่อดูกิจกรรม สมาชิก และบทสนทนา</div>
      </template>

      <template v-else-if="section === 'groups'">
        <section class="community-hero">
          <div class="community-hero-copy">
            <div class="eyebrow">FITTOGETHER COMMUNITY</div>
            <h1>ไปด้วยกัน<br>ได้ไกลกว่า</h1>
            <p>เจอเพื่อนที่ชอบกีฬาเดียวกัน แล้วชวนกันออกไปขยับ</p>
            <button class="primary" @click="dialog = 'group'"><Plus :size="18"/> สร้างกลุ่มของคุณ</button>
          </div>
          <div class="community-hero-image" role="img" aria-label="นักวิ่งกำลังวิ่งบนลู่"></div>
        </section>
        <div class="community-toolbar">
          <div class="community-toolbar-heading"><div class="eyebrow">EXPLORE TOGETHER</div><h2>ค้นหากลุ่ม</h2></div>
          <div class="search-line"><Search :size="19"/><input v-model="query" placeholder="ชื่อกลุ่มหรือประเภทกีฬา" @keyup.enter="loadGroups" aria-label="ค้นหากลุ่ม"/><button @click="loadGroups">ค้นหา</button></div>
        </div>
        <div class="filters community-filters"><button :class="{ chosen: !groupScope }" @click="groupScope='';loadGroups()">ทั้งหมด</button><button :class="{ chosen: groupScope === 'mine' }" @click="groupScope='mine';loadGroups()">กลุ่มของฉัน</button><span class="group-count" v-if="loaded">{{ groups.length }} กลุ่ม</span></div>
        <div v-if="!loaded" class="loading-stage" role="status">กำลังโหลดกลุ่ม...</div>
        <div v-if="loaded && groups.length" class="community-list">
          <button v-for="group in groups" :key="group.id" class="community-row" @click="openGroup(group)">
            <div class="community-cover" :style="group.cover_url ? { backgroundImage: `url(${group.cover_url})` } : {}"><span v-if="!group.cover_url">{{ group.sport.slice(0, 1).toUpperCase() }}</span></div>
            <div class="community-row-copy"><span class="community-meta">{{ group.sport }} <span>·</span> {{ group.visibility === 'open' ? 'เข้าร่วมได้เลย' : group.visibility === 'closed' ? 'ต้องขอเข้าร่วม' : 'กลุ่มส่วนตัว' }}</span><h3>{{ group.name }}</h3><p>{{ group.description || 'ร่วมทำกิจกรรมและพูดคุยกับสมาชิกในกลุ่ม' }}</p></div>
            <div class="community-row-end"><span><UsersRound :size="15"/> {{ group.member_count }} / {{ group.max_members }}</span><ChevronRight :size="20"/></div>
          </button>
        </div>
        <div v-else-if="loaded" class="community-empty"><div class="eyebrow">START SOMETHING</div><h2>{{ query || groupScope ? 'ยังไม่พบกลุ่มที่ค้นหา' : 'เริ่มกลุ่มแรกของคุณ' }}</h2><p>{{ query || groupScope ? 'ลองใช้คำค้นหาอื่น หรือสำรวจกลุ่มทั้งหมด' : 'สร้างพื้นที่สำหรับคนที่อยากออกกำลังกายไปด้วยกัน' }}</p><button class="primary" @click="query='';groupScope='';dialog='group'"><Plus :size="18"/> สร้างกลุ่ม</button></div>
      </template>

      <template v-else-if="section === 'notifications'">
        <header class="page-top"><div><div class="eyebrow">YOUR ACTIVITY</div><h1>แจ้งเตือน</h1></div></header>
        <div class="narrow"><h2 class="list-heading">คำเชิญเข้ากลุ่ม</h2><div v-for="invite in invites.filter((i:any) => i.status === 'pending')" :key="invite.id" class="notice-row"><div class="notice-icon"><UsersRound :size="19"/></div><div><b>มีคำเชิญเข้ากลุ่ม #{{ invite.group_id }}</b><p>ตอบรับเพื่อเข้าร่วมบทสนทนาและกิจกรรม</p></div><div class="notice-actions"><button @click="replyInvite(invite,'accepted')">ตอบรับ</button><button @click="replyInvite(invite,'declined')">ปฏิเสธ</button></div></div><h2 class="list-heading">กิจกรรมล่าสุด</h2><button v-for="item in notices" :key="item.id" class="notice-row" :class="{ unread: !item.read_at }" @click="markRead(item)"><div class="notice-icon"><Bell :size="19"/></div><div><b>{{ item.kind.replaceAll('_',' ') }}</b><p>กลุ่ม #{{ item.group_id }} · {{ date(item.created_at) }}</p></div><span v-if="!item.read_at" class="unread-dot"></span></button><p v-if="!notices.length && !invites.length" class="empty">ยังไม่มีการแจ้งเตือน</p></div>
      </template>

      <template v-else>
        <div class="feed-layout"><div class="feed-column"><header class="feed-header"><div v-if="section === 'search' || section === 'saved'" class="eyebrow">{{ section === 'search' ? 'DISCOVER' : 'COLLECTION' }}</div><h1 v-if="section === 'search'">ค้นหาโพสต์</h1><h1 v-else-if="section === 'saved'">ที่บันทึกไว้</h1><div v-else class="filters feed-filters"><button v-for="item in [['latest','ล่าสุด'],['following','กำลังติดตาม'],['popular','ยอดนิยม']]" :key="item[0]" :class="{ chosen: filter === item[0] }" @click="filter = item[0]">{{ item[1] }}</button></div><button class="create-round" aria-label="สร้างโพสต์" @click="dialog = 'post'"><Plus :size="22"/></button></header>
          <div v-if="section === 'search'" class="search-line"><Search :size="19"/><input v-model="query" placeholder="ค้นหาโพสต์หรือ #แฮชแท็ก" @keyup.enter="loadPosts" aria-label="ค้นหาโพสต์"/><button @click="loadPosts">ค้นหา</button></div>
          <div v-if="!loaded" class="loading-stage" role="status">กำลังโหลดโพสต์...</div>
          <div v-if="loaded && posts.length" class="stories">
            <PostStory v-for="post in posts" :key="post.id" :post="post"
              @like="togglePost(post,'like')" @bookmark="togglePost(post,'bookmark')"
              @comments="openComments(post)" @open="index => openComments(post, index)" @share="sharePost(post)" />
          </div>
          <div v-else-if="loaded" class="feed-empty">
            <img src="/runner.png" alt="นักวิ่งบนลู่" />
            <div class="feed-empty-copy">
              <div class="eyebrow">YOUR FIRST MOMENT</div>
              <h2>{{ section === 'saved' ? 'ยังไม่มีโพสต์ที่บันทึก' : section === 'search' ? 'ยังไม่พบโพสต์' : 'เริ่มเรื่องราวของคุณ' }}</h2>
              <p>{{ section === 'saved' ? 'โพสต์ที่คุณบันทึกไว้จะปรากฏที่นี่' : section === 'search' ? 'ลองค้นหาคำอื่นหรือแฮชแท็กใหม่' : 'แชร์ภาพหรือเรื่องราวจากการออกกำลังกายครั้งล่าสุด' }}</p>
              <button v-if="section === 'feed'" class="primary" @click="dialog='post'"><Plus :size="17" /> สร้างโพสต์</button>
            </div>
          </div>
        </div><aside class="feed-aside"><div class="aside-account"><span class="avatar">{{ names[account][0] }}</span><div><b>{{ names[account] }}</b><small>พร้อมไปด้วยกัน</small></div></div><div class="aside-feature"><div class="eyebrow">FITTOGETHER</div><h2>ออกไปขยับ<br/>ด้วยกัน</h2><p>ค้นหาคนที่ชอบกีฬาเดียวกัน แล้วนัดออกกำลังกายครั้งต่อไป</p><button @click="go('groups')">สำรวจกลุ่ม <ChevronRight :size="17"/></button></div></aside></div>
      </template>
    </main>

    <div v-if="commentPost" class="overlay post-detail-overlay" role="presentation" @click.self="closePostDetail" @keydown.esc="closePostDetail">
      <div class="post-detail" :class="{ 'post-detail-text-only': !commentPost.images?.length }" role="dialog" aria-modal="true" aria-label="รายละเอียดโพสต์">
        <button ref="detailClose" type="button" class="post-detail-close" aria-label="ปิดรายละเอียดโพสต์" @click="closePostDetail"><X :size="21" /></button>
        <div v-if="commentPost.images?.length" ref="detailGallery" class="post-detail-media" tabindex="0" :aria-label="`รูปที่ ${detailImageIndex + 1} จาก ${commentPost.images.length}`" @keydown.left.prevent="moveDetailImage(-1)" @keydown.right.prevent="moveDetailImage(1)" @touchstart.passive="startDetailTouch" @touchend.passive="endDetailTouch">
          <img :src="commentPost.images[detailImageIndex]" :alt="`รูปโพสต์ที่ ${detailImageIndex + 1} จาก ${commentPost.images.length}`" />
          <template v-if="commentPost.images.length > 1">
            <span class="post-detail-count" aria-live="polite">{{ detailImageIndex + 1 }} / {{ commentPost.images.length }}</span>
            <button type="button" class="post-detail-arrow previous" aria-label="รูปก่อนหน้า" @click="moveDetailImage(-1)"><ChevronLeft :size="24" /></button>
            <button type="button" class="post-detail-arrow next" aria-label="รูปถัดไป" @click="moveDetailImage(1)"><ChevronRight :size="24" /></button>
            <div class="post-detail-dots" aria-label="เลือกรูปภาพ"><button v-for="(_, index) in commentPost.images" :key="index" type="button" :class="{ active: detailImageIndex === index }" :aria-label="`ดูรูปที่ ${index + 1}`" @click="detailImageIndex = index" /></div>
          </template>
        </div>
        <div class="post-detail-panel">
          <div class="post-detail-author"><span class="avatar">{{ (commentPost.display_name || 'F')[0] }}</span><div><b>{{ commentPost.display_name || 'สมาชิก FitTogether' }}</b><small>{{ date(commentPost.created_at) }}</small></div></div>
          <div class="post-detail-thread">
            <div class="post-detail-caption"><p v-if="commentPost.shared_post_deleted">โพสต์ต้นฉบับถูกลบแล้ว</p><button v-if="commentPost.author_id === currentId" type="button" :disabled="busy" @click="removePost(commentPost)">ลบโพสต์</button><p>{{ commentPost.content }}</p><div class="story-tags"><span v-for="tag in commentPost.hashtags" :key="tag">#{{ tag }}</span></div></div>
            <div class="post-detail-comments-head">ความคิดเห็น <span>{{ commentPost.comment_count || 0 }}</span></div>
            <div v-for="comment in comments" :key="comment.id" class="post-detail-comment">
              <span class="avatar">{{ (comment.display_name || 'M')[0] }}</span>
              <div><div><b>{{ comment.display_name || 'สมาชิก' }}</b><time>{{ date(comment.created_at) }}{{ comment.updated_at ? ' · แก้ไขแล้ว' : '' }}</time></div>
                <form v-if="editingCommentId === comment.id" @submit.prevent="saveComment(comment)">
                  <textarea v-model="editingCommentText" aria-label="แก้ไขความคิดเห็น" maxlength="350" required />
                  <button type="submit" :disabled="busy || !editingCommentText.trim()">บันทึก</button>
                  <button type="button" :disabled="busy" @click="editingCommentId = null">ยกเลิก</button>
                </form>
                <p v-else>{{ comment.content }}</p>
                <div v-if="comment.author_id === currentId && editingCommentId !== comment.id">
                  <button type="button" :disabled="busy" @click="editComment(comment)">แก้ไข</button>
                  <button type="button" :disabled="busy" @click="removeComment(comment)">ลบ</button>
                </div>
              </div>
            </div>
            <p v-if="commentLoading && !comments.length" class="post-detail-empty">กำลังโหลดความคิดเห็น...</p>
            <p v-else-if="!comments.length" class="post-detail-empty">ยังไม่มีความคิดเห็น</p>
            <button v-if="commentHasMore" type="button" class="post-detail-more" :disabled="commentLoading" @click="loadMoreComments">{{ commentLoading ? 'กำลังโหลด...' : 'ดูความคิดเห็นเพิ่มเติม' }}</button>
          </div>
          <div class="post-detail-bottom">
            <div class="post-detail-actions"><button type="button" :class="{ engaged: commentPost.liked }" :aria-label="commentPost.liked ? 'ยกเลิกถูกใจ' : 'ถูกใจ'" :disabled="busy" @click="togglePost(commentPost,'like')"><Heart :size="22" :fill="commentPost.liked ? 'currentColor' : 'none'" /> {{ commentPost.like_count }}</button><span><MessageCircle :size="21" /> {{ commentPost.comment_count }}</span><button type="button" class="post-detail-bookmark" :class="{ engaged: commentPost.bookmarked }" :aria-label="commentPost.bookmarked ? 'ยกเลิกบันทึก' : 'บันทึกโพสต์'" :disabled="busy" @click="togglePost(commentPost,'bookmark')"><Bookmark :size="21" :fill="commentPost.bookmarked ? 'currentColor' : 'none'" /></button></div>
            <form class="post-detail-compose" @submit.prevent="sendComment"><input v-model="commentText" aria-label="เขียนความคิดเห็น" placeholder="เขียนความคิดเห็น..." maxlength="350"/><button type="submit" :disabled="busy || !commentText.trim()" aria-label="ส่งความคิดเห็น"><Send :size="19" /></button></form>
          </div>
        </div>
      </div>
    </div>
    <div v-if="dialog" class="overlay" @click.self="dialog = null"><div class="modal" @paste="pastePostPhotos"><button class="close" @click="dialog = null"><X :size="20"/></button>
      <template v-if="dialog === 'post'">
        <div class="eyebrow">NEW POST</div><h2>แบ่งปันเรื่องราว</h2>
        <form @submit.prevent="savePost">
          <label>ข้อความ<textarea v-model="postForm.content" maxlength="1500" placeholder="วันนี้คุณอยากแบ่งปันอะไร?" rows="5"></textarea></label>
          <div class="post-photo-field">
            <div class="post-photo-heading"><span>รูปภาพ</span><span>{{ postPhotos.length }}/10 รูป</span></div>
            <input ref="postFileInput" class="file-input" type="file" accept="image/jpeg,image/png,image/webp" multiple @change="choosePostPhotos" />
            <button type="button" class="add-photo-button" :disabled="busy || postPhotos.length >= 10" @click="postFileInput?.click()"><ImagePlus :size="20"/> เพิ่มรูปภาพ</button>
            <p class="upload-hint">เลือกจากเครื่องหรือมือถือ และวางรูปด้วย Ctrl+V ได้ · JPG, PNG, WebP ไม่เกิน 8 MB ต่อรูป</p>
            <div v-if="postPhotos.length" class="photo-previews">
              <div v-for="(photo, index) in postPhotos" :key="photo.preview" class="photo-preview">
                <img :src="photo.preview" :alt="'รูปที่ ' + (index + 1)" />
                <button type="button" :disabled="busy" :aria-label="'ลบรูปที่ ' + (index + 1)" @click="removePostPhoto(index)"><X :size="15"/></button>
              </div>
            </div>
          </div>
          <label>แฮชแท็ก<input v-model="postForm.hashtags" placeholder="running, friends"/></label>
          <label>การมองเห็น<select v-model="postForm.visibility"><option value="public">สาธารณะ</option><option value="followers">ผู้ติดตาม</option></select></label>
          <button class="primary form-submit" :disabled="busy || !postForm.content.trim()">{{ busy ? 'กำลังเผยแพร่...' : 'เผยแพร่โพสต์' }}</button>
        </form>
      </template>
      <template v-else-if="dialog === 'group'"><div class="eyebrow">NEW COMMUNITY</div><h2>สร้างกลุ่มใหม่</h2><form @submit.prevent="createGroup"><label>ชื่อกลุ่ม<input v-model="groupForm.name" maxlength="100" required placeholder="เช่น วิ่งเช้า มทส."/></label><label>กีฬา<input v-model="groupForm.sport" maxlength="50" required placeholder="เช่น วิ่ง"/></label><label>เกี่ยวกับกลุ่ม<textarea v-model="groupForm.description" maxlength="1000" rows="3" placeholder="ชวนเพื่อนมาออกกำลังกายด้วยกัน"></textarea></label><label>ลิงก์รูปหน้าปก HTTPS<input v-model="groupForm.cover_url" placeholder="https://..."/></label><div class="form-row"><label>ประเภท<select v-model="groupForm.visibility"><option value="open">เปิด</option><option value="closed">ต้องอนุมัติ</option><option value="private">ส่วนตัว</option></select></label><label>สมาชิกสูงสุด<input v-model.number="groupForm.max_members" type="number" min="2" max="500"/></label></div><button class="primary form-submit" :disabled="busy">สร้างกลุ่ม</button></form></template>
      <template v-else-if="dialog === 'event'"><div class="eyebrow">GROUP EVENT</div><h2>นัดกิจกรรมใหม่</h2><form @submit.prevent="createEvent"><label>ชื่อกิจกรรม<input v-model="eventForm.title" required maxlength="150"/></label><label>กีฬา<input v-model="eventForm.sport" required maxlength="50"/></label><label>สถานที่<input v-model="eventForm.location" required maxlength="255"/></label><label>วันและเวลา<input v-model="eventForm.starts_at" required type="datetime-local"/></label><button class="primary form-submit" :disabled="busy">สร้างกิจกรรม</button></form></template>
      <template v-else><div class="eyebrow">INVITE</div><h2>เชิญสมาชิก</h2><form @submit.prevent="sendInvite"><label>รหัสสมาชิก<input v-model="inviteId" type="number" min="1" required placeholder="เช่น 2 สำหรับ Mint"/></label><button class="primary form-submit" :disabled="busy">ส่งคำเชิญ</button></form></template>
    </div></div>
  </div>
</template>
