<template>
  <div class="flex h-full overflow-hidden">
    <template v-if="currentBotId">
      <ChatWorkspace />
    </template>
  </div>
</template>

<script setup lang="ts">
import { watch, provide, nextTick } from 'vue'
import { storeToRefs } from 'pinia'
import { useRoute, useRouter } from 'vue-router'
import { useChatStore } from '@/store/chat-list'
import { useWorkspaceTabsStore } from '@/store/workspace-tabs'
import { openInFileManagerKey } from './composables/useFileManagerProvider'
import ChatWorkspace from './components/chat-workspace.vue'

const route = useRoute()
const router = useRouter()
const chatStore = useChatStore()
const workspaceTabs = useWorkspaceTabsStore()
const { currentBotId } = storeToRefs(chatStore)

const FILE_MANAGER_ROOT = '/data'

function normalizeFileManagerPath(path: string): string {
  const trimmedPath = path.trim()
  if (!trimmedPath) return FILE_MANAGER_ROOT
  if (trimmedPath === FILE_MANAGER_ROOT || trimmedPath.startsWith(`${FILE_MANAGER_ROOT}/`)) {
    return trimmedPath
  }
  if (trimmedPath === '/') return FILE_MANAGER_ROOT
  if (trimmedPath.startsWith('/')) {
    return `${FILE_MANAGER_ROOT}${trimmedPath}`
  }
  return `${FILE_MANAGER_ROOT}/${trimmedPath}`
}

provide(openInFileManagerKey, (path: string, _isDir = false) => {
  const normalizedPath = normalizeFileManagerPath(path)
  workspaceTabs.openFile(normalizedPath)
})

function openFreshBotWorkspace(botId: string) {
  workspaceTabs.resetBot(botId)
  workspaceTabs.openDraft()
}

const urlBotId = ((route.params.botId as string) ?? '').trim()

if (urlBotId) {
  void chatStore.selectBot(urlBotId).then(() => nextTick(() => openFreshBotWorkspace(urlBotId)))
}

let suppressUrlSync = false

watch(currentBotId, (newBotId) => {
  if (suppressUrlSync) return
  const urlBot = ((route.params.botId as string) ?? '').trim()
  const storeBot = (newBotId ?? '').trim()
  if (storeBot) {
    void nextTick(() => openFreshBotWorkspace(storeBot))
  }
  if (storeBot === urlBot) return
  if (storeBot) {
    void router.replace({
      name: 'chat',
      params: { botId: storeBot },
    })
  } else if (route.name !== 'home') {
    void router.replace({ name: 'home' })
  }
})

watch(
  () => route.params.botId,
  async (paramBotId) => {
    const urlBot = ((paramBotId as string) ?? '').trim()
    const storeBot = (currentBotId.value ?? '').trim()
    if (!urlBot || urlBot === storeBot) return

    suppressUrlSync = true
    try {
      await chatStore.selectBot(urlBot)
      await nextTick()
      openFreshBotWorkspace(urlBot)
    } finally {
      suppressUrlSync = false
    }
  },
)
</script>
