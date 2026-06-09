<template>
  <article class="group flex gap-3">
    <span
      class="mt-0.5 flex size-8 shrink-0 items-center justify-center rounded-md border"
      :class="event.tone"
    >
      <component
        :is="event.icon"
        class="size-4"
      />
    </span>
    <div class="relative min-w-0 flex-1 rounded-md border border-border bg-card p-3 shadow-sm">
      <div class="flex flex-wrap items-center gap-2">
        <p class="text-sm font-semibold">
          {{ event.title }}
        </p>
        <span class="text-[11px] text-muted-foreground">{{ event.time }}</span>
        <span class="rounded-sm bg-muted px-1.5 py-0.5 text-[10px] font-medium text-muted-foreground">
          {{ event.kind }}
        </span>
        <span
          v-if="pinned"
          class="inline-flex items-center gap-0.5 rounded-sm bg-amber-500/15 px-1.5 py-0.5 text-[10px] font-medium text-amber-600 dark:text-amber-400"
          title="已置顶为长期上下文"
        >
          <Pin class="size-3" />置顶
        </span>
      </div>

      <!-- Reply reference -->
      <div
        v-if="event.replyTo"
        class="relative mt-2 overflow-hidden rounded-sm bg-muted/50 py-1 pl-3 pr-2"
      >
        <span class="absolute inset-y-0 left-0 w-[3px] bg-primary/70" />
        <div class="truncate text-[11px] font-semibold text-primary">
          {{ event.replyTo.sender }}
        </div>
        <div
          v-if="event.replyTo.preview"
          class="mt-0.5 line-clamp-2 whitespace-pre-wrap break-words text-[11px] text-muted-foreground"
        >
          {{ event.replyTo.preview }}
        </div>
      </div>

      <!-- Body (markdown + code blocks with copy) -->
      <div
        v-if="event.body"
        class="prose prose-sm dark:prose-invert mt-1 max-w-none *:first:mt-0"
      >
        <MarkdownRender
          :content="event.body"
          :is-dark="isDark"
          :code-block-props="{ showCopyButton: true }"
          custom-id="agenthub-msg"
        />
      </div>

      <!-- Web preview cards auto-detected from body URLs -->
      <ArtifactPreviewCard
        v-for="url in artifactUrls"
        :key="url"
        :url="url"
      />

      <!-- Message-level attachments (partial Memoh demo) -->
      <AttachmentBlock
        v-if="event.attachments"
        class="mt-2"
        :block="event.attachments"
      />

      <!-- thinking: stored in metadata.thinking -->
      <details
        v-if="event.thinking"
        class="mt-2"
      >
        <summary class="cursor-pointer select-none text-xs text-muted-foreground hover:text-foreground">
          查看思考过程
        </summary>
        <p class="mt-1 whitespace-pre-wrap text-xs leading-5 text-muted-foreground/70">
          {{ event.thinking }}
        </p>
      </details>

      <!-- tools: edit → DiffCard (read-only + 撤回); rest → home tool-call suite -->
      <div
        v-if="toolBlocks.length"
        class="mt-2 space-y-1"
      >
        <template
          v-for="block in toolBlocks"
          :key="block.id"
        >
          <DiffCard
            v-if="block.toolName === 'edit'"
            :block="block"
          />
          <ToolCallBlock
            v-else
            :block="block"
          />
        </template>
      </div>

      <div
        v-if="event.actions?.length"
        class="mt-3 flex flex-wrap gap-2"
      >
        <Button
          v-for="action in event.actions"
          :key="action"
          variant="outline"
          size="sm"
          class="h-7 text-xs"
        >
          {{ action }}
        </Button>
      </div>

      <!-- Hover action bar: copy / quote / reply / regenerate -->
      <div class="absolute right-2 top-2 z-10 hidden gap-1 group-hover:flex">
        <button
          type="button"
          class="inline-flex items-center gap-1 rounded-md border border-border bg-card px-1.5 py-0.5 text-[10px] text-muted-foreground shadow-sm transition-colors hover:bg-muted/60 hover:text-foreground"
          :title="copied ? '已复制' : '复制'"
          @click="copyBody"
        >
          <component
            :is="copied ? Check : Copy"
            class="size-3"
          />
        </button>
        <button
          v-if="event.body"
          type="button"
          class="inline-flex items-center gap-1 rounded-md border border-border bg-card px-1.5 py-0.5 text-[10px] text-muted-foreground shadow-sm transition-colors hover:bg-muted/60 hover:text-foreground"
          title="引用"
          @click="$emit('quote', event)"
        >
          <Quote class="size-3" />
        </button>
        <button
          v-if="event.body"
          type="button"
          class="inline-flex items-center gap-1 rounded-md border border-border bg-card px-1.5 py-0.5 text-[10px] shadow-sm transition-colors hover:bg-muted/60"
          :class="pinned ? 'text-amber-600 dark:text-amber-400' : 'text-muted-foreground hover:text-foreground'"
          :title="pinned ? '取消置顶' : '置顶为长期上下文'"
          @click="$emit('pin', event)"
        >
          <component
            :is="pinned ? PinOff : Pin"
            class="size-3"
          />
        </button>
        <button
          type="button"
          class="inline-flex items-center gap-1 rounded-md border border-border bg-card px-1.5 py-0.5 text-[10px] text-muted-foreground shadow-sm transition-colors hover:bg-muted/60 hover:text-foreground"
          title="回复"
          @click="$emit('reply', event)"
        >
          <Reply class="size-3" />
        </button>
        <DropdownMenu v-if="isAgentMessage && canRegenerate">
          <DropdownMenuTrigger as-child>
            <button
              type="button"
              class="inline-flex items-center gap-1 rounded-md border border-border bg-card px-1.5 py-0.5 text-[10px] text-muted-foreground shadow-sm transition-colors hover:bg-muted/60 hover:text-foreground"
              title="重新生成"
            >
              <RefreshCw class="size-3" />
            </button>
          </DropdownMenuTrigger>
          <DropdownMenuContent align="end">
            <DropdownMenuItem @select="$emit('regenerate', event, 'detailed')">
              更细节
            </DropdownMenuItem>
            <DropdownMenuItem @select="$emit('regenerate', event, 'imaginative')">
              更有想象力
            </DropdownMenuItem>
            <DropdownMenuItem>
              取消
            </DropdownMenuItem>
          </DropdownMenuContent>
        </DropdownMenu>
      </div>
    </div>
  </article>
</template>

<script setup lang="ts">
import { computed, provide, ref } from 'vue'
import { Check, Copy, Quote, Reply, RefreshCw, Pin, PinOff } from 'lucide-vue-next'
import { Button, DropdownMenu, DropdownMenuTrigger, DropdownMenuContent, DropdownMenuItem } from '@memohai/ui'
import MarkdownRender, { enableKatex, enableMermaid } from 'markstream-vue'
import { useSettingsStore } from '@/store/settings'
import { extractArtifactUrls } from '@/utils/artifact-urls'
import ArtifactPreviewCard from '@/pages/home/components/artifact-preview-card.vue'
import AttachmentBlock from '@/pages/home/components/attachment-block.vue'
import ToolCallBlock from '@/pages/home/components/tool-call-block.vue'
import DiffCard from './diff-card.vue'
import { toToolBlock } from '../utils/tool-bridge'
import type { AgentItem, TimelineEvent } from '../types'

// markstream's KaTeX/Mermaid extensions are opt-in and only enabled in home's
// message-item; enable them here too so a user landing directly on AgentHub
// (without visiting chat first) still gets math/mermaid parity. Idempotent.
enableKatex()
enableMermaid()

const props = defineProps<{
  event: TimelineEvent
  // Sending agent, used to resolve the apply-edit/undo target bot. Only Memoh
  // bots (non-codex/claudecode framework) expose a real botId so the diff 撤回
  // button is enabled; bridge agents already executed the edit in their own
  // container, so their diffs stay read-only.
  agent?: AgentItem
  // Only the last agent message offers regenerate (this round's scope).
  canRegenerate?: boolean
  // Whether this message is pinned as long-term context for the room's agents.
  pinned?: boolean
}>()

defineEmits<{
  reply: [event: TimelineEvent]
  quote: [event: TimelineEvent]
  regenerate: [event: TimelineEvent, mode: 'detailed' | 'imaginative']
  pin: [event: TimelineEvent]
}>()

const settingsStore = useSettingsStore()
const isDark = computed(() => settingsStore.theme === 'dark')

const memohBotId = computed(() => {
  const agent = props.agent
  if (!agent?.botId) return ''
  const fw = (agent.framework ?? '').toLowerCase()
  if (fw === 'codex' || fw === 'claudecode') return ''
  return agent.botId
})
// tool-call-detail-edit.vue injects this to target POST /bots/{id}/apply-edit;
// an empty string disables Apply (the component early-returns on no botId).
provide('botId', memohBotId)

const artifactUrls = computed(() => extractArtifactUrls(props.event.body))

const toolBlocks = computed(() =>
  (props.event.tools ?? []).map((tool, i) => toToolBlock(tool, i)),
)

const isAgentMessage = computed(() => props.event.senderType === 'agent')

const copied = ref(false)
async function copyBody() {
  if (!props.event.body) return
  try {
    await navigator.clipboard.writeText(props.event.body)
    copied.value = true
    window.setTimeout(() => { copied.value = false }, 1500)
  } catch {
    // Clipboard can be blocked (insecure context / permissions); silently ignore.
  }
}
</script>
