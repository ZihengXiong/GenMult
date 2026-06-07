<template>
  <div class="text-sm leading-relaxed">
    <button
      class="group flex items-center gap-1.5 w-full text-left transition-colors cursor-pointer py-0.5 text-muted-foreground hover:text-foreground"
      @click="toggleOpen"
    >
      <Lightbulb class="size-3.5 shrink-0" />
      <span
        class="shrink-0"
        :class="actionClass"
      >{{ actionLabel }}</span>
      <ChevronRight
        v-if="!open"
        class="size-3.5 shrink-0 ml-auto opacity-60 group-hover:opacity-100"
      />
      <ChevronDown
        v-else
        class="size-3.5 shrink-0 ml-auto opacity-60 group-hover:opacity-100"
      />
    </button>
    <div
      v-if="open"
      class="mt-1 ml-5 border-l border-border pl-3 py-1"
    >
      <div class="prose prose-sm dark:prose-invert max-w-none *:first:mt-0 text-muted-foreground opacity-90 leading-relaxed text-[13px]">
        <MarkdownRender
          :content="block.content"
          :is-dark="isDark"
          :typewriter="streaming"
          custom-id="thinking-msg"
        />
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, ref } from 'vue'
import { ChevronDown, ChevronRight, Lightbulb } from 'lucide-vue-next'
import { useI18n } from 'vue-i18n'
import type { ThinkingBlock } from '@/store/chat-list'
import MarkdownRender from 'markstream-vue'
import { useSettingsStore } from '@/store/settings'

const props = defineProps<{
  block: ThinkingBlock
  streaming: boolean
}>()

const { t } = useI18n()

const settingsStore = useSettingsStore()
const isDark = computed(() => settingsStore.theme === 'dark')

const open = ref(props.streaming)

const actionLabel = computed(() =>
  props.streaming ? t('chat.thinkingInProgress') : t('chat.thinkingDone'),
)

const actionClass = computed(() => (props.streaming ? 'tool-shimmer-text' : ''))

function toggleOpen() {
  open.value = !open.value
}
</script>
