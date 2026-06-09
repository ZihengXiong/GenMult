<template>
  <div class="my-1 overflow-hidden rounded-md border border-border">
    <div class="flex items-center justify-between gap-2 bg-muted/40 px-2.5 py-1.5">
      <div class="flex min-w-0 items-center gap-1.5">
        <FilePen class="size-3.5 shrink-0 text-muted-foreground" />
        <span class="truncate font-mono text-xs text-muted-foreground">{{ displayName }}</span>
        <span
          v-if="addCount"
          class="shrink-0 font-mono text-[11px] text-success-foreground"
        >+{{ addCount }}</span>
        <span
          v-if="removeCount"
          class="shrink-0 font-mono text-[11px] text-destructive"
        >-{{ removeCount }}</span>
      </div>
      <Button
        v-if="canUndo"
        variant="outline"
        size="sm"
        class="h-6 shrink-0 gap-1 text-xs"
        :disabled="undoing || undone"
        @click="handleUndo"
      >
        <Check
          v-if="undone"
          class="size-3 text-success-foreground"
        />
        <Undo2
          v-else
          class="size-3"
        />
        {{ undone ? '已撤回' : '撤回' }}
      </Button>
    </div>

    <div
      v-if="hasChanges && shiki.loading.value"
      class="flex items-center gap-1.5 px-2.5 py-2 text-xs text-muted-foreground"
    >
      <LoaderCircle class="size-3 animate-spin" />
    </div>
    <!-- eslint-disable vue/no-v-html -->
    <div
      v-else-if="hasChanges"
      class="shiki-diff-container max-h-96 overflow-auto rounded-sm text-xs [&_code]:text-xs [&_pre]:m-0! [&_pre]:bg-transparent! [&_pre]:p-2"
      v-html="shiki.html.value"
    />
    <!-- eslint-enable vue/no-v-html -->
    <p
      v-else
      class="px-2.5 py-2 text-xs italic text-muted-foreground"
    >
      无改动
    </p>
  </div>
</template>

<script setup lang="ts">
import { computed, inject, onMounted, ref, type Ref } from 'vue'
import { FilePen, LoaderCircle, Undo2, Check } from 'lucide-vue-next'
import { Button } from '@memohai/ui'
import { toast } from 'vue-sonner'
import { client } from '@memohai/sdk/client'
import type { ToolCallBlock } from '@/store/chat-list'
import { extractFilename, useShikiHighlighter } from '@/composables/useShikiHighlighter'

const props = defineProps<{ block: ToolCallBlock }>()
const shiki = useShikiHighlighter()

// Provided by timeline-event-item: the sending agent's botId, but only for
// Memoh bots (empty for codex/claude). Undo writes to this bot's container.
const botIdRef = inject<Ref<string>>('botId', ref(''))

const undoing = ref(false)
const undone = ref(false)

const filePath = computed(() => {
  const input = props.block.input as Record<string, unknown> | undefined
  return (input?.path as string) ?? ''
})
const displayName = computed(() => extractFilename(filePath.value) || 'untitled')

const oldText = computed(() => {
  const input = props.block.input as Record<string, unknown> | undefined
  return (input?.old_text as string) ?? ''
})
const newText = computed(() => {
  const input = props.block.input as Record<string, unknown> | undefined
  return (input?.new_text as string) ?? ''
})

const hasChanges = computed(() => Boolean(oldText.value || newText.value))
// Undo only when new_text is non-empty. A deletion edit (new_text="") would
// reverse-apply with old_text="" → the backend's empty-old_text branch
// overwrites the WHOLE file with old_text. So deletion edits stay read-only.
const canUndo = computed(() => Boolean(botIdRef.value) && newText.value.length > 0)
const addCount = computed(() => lineCount(newText.value))
const removeCount = computed(() => lineCount(oldText.value))

// Undo = reverse apply: the file currently holds new_text (the agent already
// executed the edit), so we replace new_text back with old_text via the same
// apply-edit endpoint with swapped params. Best-effort — the endpoint uses an
// exact strings.Contains match, so CRLF/whitespace-normalized edits may 400.
async function handleUndo() {
  if (!canUndo.value || undoing.value || undone.value) return
  undoing.value = true
  try {
    await client.post({
      url: '/bots/{bot_id}/apply-edit',
      path: { bot_id: botIdRef.value },
      body: { path: filePath.value, old_text: newText.value, new_text: oldText.value },
      throwOnError: true,
    })
    undone.value = true
    toast.success('已撤回')
  } catch {
    toast.error('撤回失败：文件可能已变更')
  } finally {
    undoing.value = false
  }
}

function lineCount(text: string): number {
  if (!text) return 0
  return text.split('\n').length
}

onMounted(() => {
  if (hasChanges.value) {
    void shiki.highlightDiff(oldText.value, newText.value, displayName.value)
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
