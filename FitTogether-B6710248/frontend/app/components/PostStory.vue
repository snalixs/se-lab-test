<script setup lang="ts">
import { Bookmark, ChevronLeft, ChevronRight, Heart, MessageCircle, Send, MoreHorizontal } from 'lucide-vue-next'

const props = defineProps<{ post: any }>()
const imageIndex = ref(0)
let touchStartX = 0
let touchStartY = 0

function moveImage(direction: number) {
  const count = props.post.images?.length || 0
  if (count < 2) return
  imageIndex.value = (imageIndex.value + direction + count) % count
}

function onTouchStart(event: TouchEvent) {
  touchStartX = event.changedTouches[0]?.clientX || 0
  touchStartY = event.changedTouches[0]?.clientY || 0
}

function onTouchEnd(event: TouchEvent) {
  const touch = event.changedTouches[0]
  if (!touch) return
  const dx = touch.clientX - touchStartX
  const dy = touch.clientY - touchStartY
  if (Math.abs(dx) > 45 && Math.abs(dx) > Math.abs(dy)) moveImage(dx < 0 ? 1 : -1)
}
const emit = defineEmits<{
  open: [imageIndex: number]
  like: []
  bookmark: []
  comments: []
  share: []
}>()

function date(value: string) {
  return new Intl.DateTimeFormat('th-TH', { dateStyle: 'medium' }).format(new Date(value))
}
</script>

<template>
  <article class="story" :class="{ 'story-with-image': post.images?.length }">
    <div class="story-author">
      <span class="avatar">{{ (post.display_name || 'F')[0] }}</span>
      <div><b>{{ post.display_name || 'สมาชิก FitTogether' }}</b><small>{{ date(post.created_at) }}</small></div>
      <MoreHorizontal :size="20" class="story-more" aria-hidden="true" />
    </div>
    <div v-if="post.images?.length" class="story-image" tabindex="0" :aria-label="`รูปภาพโพสต์ ${imageIndex + 1} จาก ${post.images.length}`" @keydown.left.prevent="moveImage(-1)" @keydown.right.prevent="moveImage(1)" @touchstart.passive="onTouchStart" @touchend.passive="onTouchEnd">
      <img :src="post.images[imageIndex]" :alt="`${post.content || 'รูปกิจกรรม'} รูปที่ ${imageIndex + 1} จาก ${post.images.length}`" @click="emit('open', imageIndex)" />
      <template v-if="post.images.length > 1">
        <span class="story-image-count" aria-live="polite">{{ imageIndex + 1 }} / {{ post.images.length }}</span>
        <button type="button" class="story-image-nav story-image-prev" aria-label="รูปก่อนหน้า" @click="moveImage(-1)"><ChevronLeft :size="22" /></button>
        <button type="button" class="story-image-nav story-image-next" aria-label="รูปถัดไป" @click="moveImage(1)"><ChevronRight :size="22" /></button>
        <div class="story-image-dots" aria-label="เลือกรูปภาพ">
          <button v-for="(_, index) in post.images" :key="index" type="button" :class="{ active: imageIndex === index }" :aria-label="`ดูรูปที่ ${index + 1}`" :aria-current="imageIndex === index ? 'true' : undefined" @click="imageIndex = index" />
        </div>
      </template>
    </div>
    <div class="story-footer">
      <div class="story-actions">
        <button :class="{ engaged: post.liked }" :aria-label="post.liked ? 'ยกเลิกถูกใจ' : 'ถูกใจ'" @click="emit('like')"><Heart :size="21" :fill="post.liked ? 'currentColor' : 'none'" /><span>{{ post.like_count }}</span></button>
        <button aria-label="ความคิดเห็น" @click="emit('comments')"><MessageCircle :size="21" /><span>{{ post.comment_count }}</span></button>
        <button v-if="!post.shared_post_deleted" aria-label="แชร์โพสต์" @click="emit('share')"><Send :size="20" /></button>
        <button class="story-bookmark" :class="{ engaged: post.bookmarked }" :aria-label="post.bookmarked ? 'ยกเลิกบันทึก' : 'บันทึกโพสต์'" @click="emit('bookmark')"><Bookmark :size="21" :fill="post.bookmarked ? 'currentColor' : 'none'" /></button>
      </div>
    </div>
    <div class="story-copy">
      <p v-if="post.shared_post_deleted">โพสต์ต้นฉบับถูกลบแล้ว</p>
      <span v-if="post.shared_post_id || post.shared_post_deleted" class="story-kicker">แชร์โพสต์</span>
      <p><strong>{{ post.display_name || 'สมาชิก FitTogether' }}</strong> {{ post.content }}</p>
      <div class="story-tags"><span v-for="tag in post.hashtags" :key="tag">#{{ tag }}</span></div>
    </div>
    <button v-if="post.comment_count" class="story-comment-link" @click="emit('comments')">ดูความคิดเห็นทั้งหมด {{ post.comment_count }} รายการ</button>
  </article>
</template>
