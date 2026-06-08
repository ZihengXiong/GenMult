<template>
  <div class="space-y-1.5">
    <div
      v-if="content && shiki.loading.value"
      class="flex items-center gap-1.5 text-xs text-muted-foreground"
    >
      <LoaderCircle class="size-3 animate-spin" />
    </div>
    <!-- eslint-disable vue/no-v-html -->
    <div
      v-else-if="content"
      class="shiki-container overflow-x-auto overflow-y-auto max-h-96 text-xs rounded-sm bg-muted/30 [&_pre]:bg-transparent! [&_pre]:p-2 [&_pre]:m-0 [&_code]:text-xs"
      v-html="shiki.html.value"
    />
    <!-- eslint-enable vue/no-v-html -->
    <p
      v-else
      class="text-xs text-muted-foreground italic"
    >
      {{ t('chat.tools.detail.noContent') }}
    </p>
    <div
      v-if="content"
      class="flex items-center gap-2 pt-1"
    >
      <Button
        variant="outline"
        size="sm"
        class="h-6 text-xs gap-1"
        :disabled="applying"
        @click="handleApply"
      >
        <Check
          v-if="applied"
          class="size-3 text-success-foreground"
        />
        <Play
          v-else
          class="size-3"
        />
        {{ applied ? t('common.applied', 'Applied') : t('common.apply', 'Apply') }}
      </Button>
      <Button
        variant="ghost"
        size="sm"
        class="h-6 text-xs gap-1"
        @click="viewerOpen = true"
      >
        <Maximize2 class="size-3" />
        {{ t('common.viewInEditor', 'View') }}
      </Button>
    </div>
    <ArtifactCodeViewer
      v-model:open="viewerOpen"
      :content="content"
      :filename="extractFilename(filePath)"
      :readonly="true"
    />
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, ref, inject, type Ref } from 'vue'
import { LoaderCircle, Play, Check, Maximize2 } from 'lucide-vue-next'
import { Button } from '@memohai/ui'
import { useI18n } from 'vue-i18n'
import { toast } from 'vue-sonner'
import type { ToolCallBlock } from '@/store/chat-list'
import { extractFilename, useShikiHighlighter } from '@/composables/useShikiHighlighter'
import { client } from '@memohai/sdk/client'
import ArtifactCodeViewer from './artifact-code-viewer.vue'

const props = defineProps<{ block: ToolCallBlock }>()
const { t } = useI18n()
const shiki = useShikiHighlighter()

const applying = ref(false)
const applied = ref(false)
const viewerOpen = ref(false)

const botIdRef = inject<Ref<string>>('botId', ref(''))
const botId = computed(() => botIdRef.value)

const filePath = computed(() => {
  const input = props.block.input as Record<string, unknown> | undefined
  return (input?.path as string) ?? ''
})

const content = computed(() => {
  const input = props.block.input as Record<string, unknown> | undefined
  return (input?.content as string) ?? ''
})

async function handleApply() {
  if (!botId.value || applying.value || applied.value) return
  applying.value = true
  try {
    await client.post({
      url: '/bots/{bot_id}/apply-write',
      path: { bot_id: botId.value },
      body: { path: filePath.value, content: content.value },
    })
    applied.value = true
    toast.success(t('common.applied', 'Applied'))
  } catch {
    toast.error(t('common.applyFailed', 'Apply failed'))
  } finally {
    applying.value = false
  }
}

onMounted(() => {
  if (content.value) {
    void shiki.highlight(content.value, extractFilename(filePath.value))
  }
})
</script>
