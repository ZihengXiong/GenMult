<template>
  <Dialog
    v-model:open="dialogOpen"
  >
    <DialogContent class="max-w-[90vw] w-[90vw] h-[85vh] flex flex-col p-0 gap-0">
      <DialogHeader class="px-4 py-3 border-b border-border shrink-0">
        <DialogTitle class="flex items-center gap-2 text-sm font-mono">
          <FileCode class="size-4" />
          {{ filename || 'Untitled' }}
        </DialogTitle>
      </DialogHeader>
      <div class="flex-1 min-h-0">
        <MonacoEditor
          :model-value="content"
          :filename="filename"
          :readonly="readonly"
          @update:model-value="$emit('update:content', $event)"
        />
      </div>
      <div class="px-4 py-2 border-t border-border flex items-center justify-between text-xs text-muted-foreground shrink-0">
        <span>{{ readonly ? 'Read-only' : 'Editable' }}</span>
        <DialogClose as-child>
          <Button
            variant="outline"
            size="sm"
          >
            Close
          </Button>
        </DialogClose>
      </div>
    </DialogContent>
  </Dialog>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { FileCode } from 'lucide-vue-next'
import { Button } from '@memohai/ui'
import { Dialog, DialogContent, DialogHeader, DialogTitle, DialogClose } from '@memohai/ui'
import MonacoEditor from '@/components/monaco-editor/index.vue'

const props = defineProps<{
  open: boolean
  content: string
  filename?: string
  readonly?: boolean
}>()

const emit = defineEmits<{
  'update:open': [value: boolean]
  'update:content': [value: string]
}>()

const dialogOpen = computed({
  get: () => props.open,
  set: (v) => emit('update:open', v),
})
</script>
