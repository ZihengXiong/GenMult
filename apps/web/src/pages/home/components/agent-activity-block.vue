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
      >{{ headerLabel }}</span>
      <span
        v-if="stepCount > 0"
        class="shrink-0 text-xs opacity-70"
      >{{ stepLabel }}</span>
      <ChevronRight
        v-if="blocks.length && !open"
        class="size-3.5 shrink-0 ml-auto opacity-60 group-hover:opacity-100"
      />
      <ChevronDown
        v-else-if="blocks.length"
        class="size-3.5 shrink-0 ml-auto opacity-60 group-hover:opacity-100"
      />
    </button>
    <div
      v-if="open && blocks.length"
      class="mt-1 ml-5 border-l border-border pl-3 py-1 space-y-2"
    >
      <template
        v-for="(block, i) in blocks"
        :key="i"
      >
        <!-- Thinking / reasoning step -->
        <div
          v-if="block.type === 'reasoning'"
          class="prose prose-sm dark:prose-invert max-w-none *:first:mt-0 text-muted-foreground opacity-90 leading-relaxed text-[13px]"
        >
          <MarkdownRender
            :content="block.content"
            :is-dark="isDark"
            :typewriter="streaming && i === blocks.length - 1"
            custom-id="thinking-msg"
          />
        </div>

        <!-- Tool call step -->
        <ToolCallInline
          v-else-if="block.type === 'tool'"
          :block="(block as ToolCallBlockType)"
        />
      </template>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { ChevronDown, ChevronRight, Lightbulb } from 'lucide-vue-next'
import { useI18n } from 'vue-i18n'
import MarkdownRender from 'markstream-vue'
import type { ContentBlock, ToolCallBlock as ToolCallBlockType } from '@/store/chat-list'
import { useSettingsStore } from '@/store/settings'
import ToolCallInline from './tool-call-inline.vue'

const props = defineProps<{
  blocks: ContentBlock[]
  streaming: boolean
}>()

const { t } = useI18n()

const settingsStore = useSettingsStore()
const isDark = computed(() => settingsStore.theme === 'dark')

// Expanded while streaming so progress is visible live; auto-collapses to a
// one-line summary once the turn finishes. The user can re-open it any time.
const open = ref(props.streaming)
watch(
  () => props.streaming,
  (streaming, was) => {
    if (was && !streaming) open.value = false
    else if (!was && streaming) open.value = true
  },
)

const stepCount = computed(
  () => props.blocks.filter((b) => b.type === 'tool').length,
)

const stepLabel = computed(() =>
  t('chat.activitySteps', { n: stepCount.value }),
)

const headerLabel = computed(() =>
  props.streaming ? t('chat.thinkingInProgress') : t('chat.thinkingDone'),
)

const actionClass = computed(() => (props.streaming ? 'tool-shimmer-text' : ''))

function toggleOpen() {
  open.value = !open.value
}
</script>
