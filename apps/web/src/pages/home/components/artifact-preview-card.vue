<template>
  <div class="rounded-lg border border-border overflow-hidden my-2">
    <div class="flex items-center justify-between bg-muted/40 px-3 py-1.5">
      <div class="flex items-center gap-1.5 min-w-0">
        <Globe class="size-3.5 shrink-0 text-muted-foreground" />
        <span class="text-xs text-muted-foreground truncate">{{ displayUrl }}</span>
      </div>
      <div class="flex items-center gap-1">
        <button
          type="button"
          class="p-1 rounded hover:bg-muted text-muted-foreground hover:text-foreground transition-colors"
          :title="expanded ? $t('common.collapse') : $t('common.expand')"
          @click="expanded = !expanded"
        >
          <ChevronDown
            v-if="expanded"
            class="size-3.5"
          />
          <ChevronRight
            v-else
            class="size-3.5"
          />
        </button>
        <a
          :href="url"
          target="_blank"
          rel="noopener noreferrer"
          class="p-1 rounded hover:bg-muted text-muted-foreground hover:text-foreground transition-colors"
          :title="$t('common.openInNewTab')"
        >
          <ExternalLink class="size-3.5" />
        </a>
      </div>
    </div>
    <div
      v-if="expanded"
      class="relative bg-background"
    >
      <iframe
        :src="url"
        sandbox="allow-scripts allow-same-origin"
        class="w-full border-0"
        :style="{ height: iframeHeight + 'px' }"
        loading="lazy"
        referrerpolicy="no-referrer"
        @load="onIframeLoad"
      />
      <div
        v-if="loading"
        class="absolute inset-0 flex items-center justify-center bg-background/80"
      >
        <Spinner />
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref } from 'vue'
import { Globe, ChevronDown, ChevronRight, ExternalLink } from 'lucide-vue-next'
import { Spinner } from '@memohai/ui'

const props = defineProps<{
  url: string
}>()

const expanded = ref(true)
const loading = ref(true)
const iframeHeight = ref(300)

const displayUrl = (() => {
  try {
    const u = new URL(props.url)
    return u.hostname + u.pathname
  } catch {
    return props.url
  }
})()

function onIframeLoad() {
  loading.value = false
}
</script>
