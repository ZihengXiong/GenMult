<template>
  <div class="space-y-1.5">
    <div
      v-if="hasChanges && shiki.loading.value"
      class="flex items-center gap-1.5 text-xs text-muted-foreground"
    >
      <LoaderCircle class="size-3 animate-spin" />
    </div>
    <!-- eslint-disable vue/no-v-html -->
    <div
      v-else-if="hasChanges"
      class="shiki-diff-container overflow-x-auto overflow-y-auto max-h-96 text-xs rounded-sm [&_pre]:bg-transparent! [&_pre]:p-2 [&_pre]:m-0 [&_code]:text-xs"
      v-html="shiki.html.value"
    />
    <!-- eslint-enable vue/no-v-html -->
    <p
      v-else
      class="text-xs text-muted-foreground italic"
    >
      {{ t('chat.tools.detail.noChanges') }}
    </p>
    <div
      v-if="hasChanges"
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
      :content="newText"
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

const oldText = computed(() => {
  const input = props.block.input as Record<string, unknown> | undefined
  return (input?.old_text as string) ?? ''
})

const newText = computed(() => {
  const input = props.block.input as Record<string, unknown> | undefined
  return (input?.new_text as string) ?? ''
})

const hasChanges = computed(() => Boolean(oldText.value || newText.value))

async function handleApply() {
  if (!botId.value || applying.value || applied.value) return
  applying.value = true
  try {
    await client.post({
      url: '/bots/{bot_id}/apply-edit',
      path: { bot_id: botId.value },
      body: { path: filePath.value, old_text: oldText.value, new_text: newText.value },
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
  if (hasChanges.value) {
    void shiki.highlightDiff(oldText.value, newText.value, extractFilename(filePath.value))
  }
})
</script>

<style>
.shiki-diff-container .diff-block pre {
  margin: 0 !important;
  padding: 0.5rem 0.75rem !important;
  background: transparent !important;
}
.shiki-diff-container .diff-remove {
  background-color: var(--diff-remove);
  border-left: 3px solid var(--diff-remove-border);
}
.shiki-diff-container .diff-add {
  background-color: var(--diff-add);
  border-left: 3px solid var(--diff-add-border);
}
</style>
