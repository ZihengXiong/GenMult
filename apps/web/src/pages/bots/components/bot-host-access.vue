<template>
  <SettingsShell
    width="wide"
    class="space-y-6"
  >
    <header class="space-y-1">
      <h2 class="text-base font-medium">
        {{ $t('bots.hostAccess.title') }}
      </h2>
      <p class="max-w-2xl text-xs leading-relaxed text-muted-foreground">
        {{ $t('bots.hostAccess.intro') }}
      </p>
    </header>

    <Separator />

    <div
      v-if="isLocalWorkspace && !localWorkspaceEnabled"
      class="rounded-md border border-warning-border bg-warning-soft px-3 py-2 text-xs text-warning-foreground"
    >
      {{ $t('bots.hostAccess.unavailable') }}
    </div>

    <template v-else>
      <div
        v-if="isContainerWorkspace"
        class="rounded-md border border-border bg-muted/40 px-3 py-2 text-xs text-muted-foreground"
      >
        {{ $t('bots.hostAccess.containerHint') }}
      </div>

      <section class="space-y-2">
        <Label>{{ $t('bots.hostAccess.trustedRoot') }}</Label>
        <Input
          v-model="trustedRootDraft"
          :placeholder="$t('bots.hostAccess.trustedRootPlaceholder')"
        />
        <p class="text-xs text-muted-foreground">
          {{ $t('bots.hostAccess.trustedRootHint') }}
        </p>
        <p
          v-if="workspaceRoot"
          class="text-xs text-muted-foreground"
        >
          {{ $t('bots.hostAccess.workspaceRootLabel') }} {{ workspaceRoot }}
        </p>
      </section>

      <section class="space-y-2">
        <Label>{{ $t('bots.hostAccess.approvedPaths') }}</Label>
        <Textarea
          v-model="approvedPathsDraft"
          class="min-h-40 resize-none font-mono text-xs"
          :placeholder="$t('bots.hostAccess.approvedPathsPlaceholder')"
        />
        <p class="text-xs text-muted-foreground">
          {{ $t('bots.hostAccess.approvedPathsHint') }}
        </p>
        <p
          v-if="isContainerWorkspace"
          class="text-xs text-muted-foreground"
        >
          {{ $t('bots.hostAccess.containerMountHint') }}
        </p>
      </section>
    </template>

    <Separator />

    <div class="flex items-center justify-end gap-3 pt-1">
      <span
        v-if="hasChanges"
        class="text-xs text-muted-foreground"
      >
        {{ $t('bots.hostAccess.unsavedChanges') }}
      </span>
      <Button
        size="sm"
        :disabled="!canSave"
        @click="handleSave"
      >
        <Spinner
          v-if="isSaving"
          class="mr-2 size-4"
        />
        {{ $t('bots.settings.save') }}
      </Button>
    </div>
  </SettingsShell>
</template>

<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { toast } from 'vue-sonner'
import { useMutation, useQuery, useQueryCache } from '@pinia/colada'
import { getBotsById, putBotsById } from '@memohai/sdk'
import { getBotsQueryKey } from '@memohai/sdk/colada'
import { Button, Input, Label, Separator, Spinner, Textarea } from '@memohai/ui'
import SettingsShell from '@/components/settings-shell/index.vue'
import { resolveApiErrorMessage } from '@/utils/api-error'
import { useCapabilitiesStore } from '@/store/capabilities'

const props = defineProps<{
  botId: string
}>()

const { t } = useI18n()
const queryCache = useQueryCache()
const capabilities = useCapabilitiesStore()

const WORKSPACE_METADATA_KEY = 'workspace'
const LOCAL_WORKSPACE_PATH_METADATA_KEY = 'local_workspace_path'
const HOST_ACCESS_METADATA_KEY = 'host_access'
const TRUSTED_ROOT_METADATA_KEY = 'trusted_root'
const APPROVED_PATHS_METADATA_KEY = 'approved_paths'

type HostAccessDraft = {
  trustedRoot: string
  approvedPaths: string[]
}

const trustedRootDraft = ref('')
const approvedPathsDraft = ref('')
const savedState = ref<HostAccessDraft>({ trustedRoot: '', approvedPaths: [] })

const { data: bot, refetch } = useQuery({
  key: () => ['bot', props.botId],
  query: async () => {
    const { data } = await getBotsById({ path: { id: props.botId }, throwOnError: true })
    return data
  },
  enabled: () => !!props.botId,
})

const { mutateAsync: updateBot, isLoading: isSaving } = useMutation({
  mutation: async (metadata: Record<string, unknown>) => {
    const { data } = await putBotsById({
      path: { id: props.botId },
      body: { metadata },
      throwOnError: true,
    })
    return data
  },
  onSettled: () => {
    void queryCache.invalidateQueries({ key: ['bot', props.botId] })
    void queryCache.invalidateQueries({ key: ['bot'] })
    void queryCache.invalidateQueries({ key: getBotsQueryKey() })
  },
})

const localWorkspaceEnabled = computed(() => capabilities.localWorkspaceEnabled)
const metadataRecord = computed(() => (isRecord(bot.value?.metadata) ? bot.value.metadata as Record<string, unknown> : undefined))
const workspaceRoot = computed(() => readLocalWorkspacePath(metadataRecord.value))
const workspaceBackend = computed(() => readWorkspaceBackend(metadataRecord.value))
const isLocalWorkspace = computed(() => workspaceBackend.value === 'local')
const isContainerWorkspace = computed(() => workspaceBackend.value !== 'local')

const normalizedApprovedPaths = computed(() => normalizePathLines(approvedPathsDraft.value))
const normalizedTrustedRoot = computed(() => trustedRootDraft.value.trim() || workspaceRoot.value)

const hasChanges = computed(() =>
  normalizedTrustedRoot.value !== savedState.value.trustedRoot ||
  JSON.stringify(normalizedApprovedPaths.value) !== JSON.stringify(savedState.value.approvedPaths),
)

const canSave = computed(() =>
  !isSaving.value &&
  ((isLocalWorkspace.value && localWorkspaceEnabled.value) || isContainerWorkspace.value) &&
  hasChanges.value,
)

watch(() => bot.value, (value) => {
  const next = readHostAccessDraft(value?.metadata as Record<string, unknown> | undefined)
  savedState.value = next
  trustedRootDraft.value = next.trustedRoot
  approvedPathsDraft.value = next.approvedPaths.join('\n')
}, { immediate: true })

async function handleSave() {
  if (!canSave.value) return

  try {
    const metadata = withHostAccessMetadata(
      metadataRecord.value,
      normalizedTrustedRoot.value,
      normalizedApprovedPaths.value,
      workspaceRoot.value,
    )
    await updateBot(metadata)
    savedState.value = {
      trustedRoot: normalizedTrustedRoot.value,
      approvedPaths: normalizedApprovedPaths.value,
    }
    approvedPathsDraft.value = normalizedApprovedPaths.value.join('\n')
    toast.success(t('bots.hostAccess.saveSuccess'))
    await refetch()
  } catch (error) {
    toast.error(resolveApiErrorMessage(error, t('bots.hostAccess.saveFailed')))
  }
}

function isRecord(value: unknown): value is Record<string, unknown> {
  return !!value && typeof value === 'object' && !Array.isArray(value)
}

function readWorkspaceBackend(metadata: Record<string, unknown> | undefined): string {
  const workspace = metadata?.[WORKSPACE_METADATA_KEY]
  if (!isRecord(workspace)) return 'container'
  const backend = workspace.backend
  return typeof backend === 'string' && backend.trim() ? backend.trim() : 'container'
}

function readLocalWorkspacePath(metadata: Record<string, unknown> | undefined): string {
  const workspace = metadata?.[WORKSPACE_METADATA_KEY]
  if (!isRecord(workspace)) return ''
  const raw = workspace[LOCAL_WORKSPACE_PATH_METADATA_KEY]
  return typeof raw === 'string' ? raw.trim() : ''
}

function readHostAccessDraft(metadata: Record<string, unknown> | undefined): HostAccessDraft {
  const workspace = metadata?.[WORKSPACE_METADATA_KEY]
  const localWorkspacePath = readLocalWorkspacePath(metadata)
  if (!isRecord(workspace)) {
    return { trustedRoot: localWorkspacePath, approvedPaths: [] }
  }

  const hostAccess = workspace[HOST_ACCESS_METADATA_KEY]
  if (!isRecord(hostAccess)) {
    return { trustedRoot: localWorkspacePath, approvedPaths: [] }
  }

  const trustedRoot = typeof hostAccess[TRUSTED_ROOT_METADATA_KEY] === 'string'
    ? String(hostAccess[TRUSTED_ROOT_METADATA_KEY]).trim()
    : localWorkspacePath

  const rawApprovedPaths = hostAccess[APPROVED_PATHS_METADATA_KEY]
  const approvedPaths = Array.isArray(rawApprovedPaths)
    ? rawApprovedPaths.flatMap((entry) => {
        if (!isRecord(entry)) return []
        const source = typeof entry.source === 'string' ? entry.source.trim() : ''
        const status = typeof entry.status === 'string' ? entry.status.trim() : ''
        if (!source) return []
        if (status && status !== 'approved') return []
        return [source]
      })
    : []

  return {
    trustedRoot: trustedRoot || localWorkspacePath,
    approvedPaths: dedupePaths(approvedPaths),
  }
}

function withHostAccessMetadata(
  metadata: Record<string, unknown> | undefined,
  trustedRoot: string,
  approvedPaths: string[],
  localWorkspacePath: string,
) {
  const nextMetadata = isRecord(metadata) ? { ...metadata } : {}
  const workspaceSection = isRecord(nextMetadata[WORKSPACE_METADATA_KEY])
    ? { ...(nextMetadata[WORKSPACE_METADATA_KEY] as Record<string, unknown>) }
    : {}

  const canonicalTrustedRoot = trustedRoot.trim()
  const storedTrustedRoot = canonicalTrustedRoot !== '' && canonicalTrustedRoot !== localWorkspacePath ? canonicalTrustedRoot : ''
  const normalizedApprovedPaths = dedupePaths(approvedPaths)

  if (!storedTrustedRoot && normalizedApprovedPaths.length === 0) {
    delete workspaceSection[HOST_ACCESS_METADATA_KEY]
  } else {
    const hostAccessSection: Record<string, unknown> = {}
    if (storedTrustedRoot) {
      hostAccessSection[TRUSTED_ROOT_METADATA_KEY] = storedTrustedRoot
    }
    hostAccessSection[APPROVED_PATHS_METADATA_KEY] = normalizedApprovedPaths.map(source => ({
      source,
      status: 'approved',
    }))
    workspaceSection[HOST_ACCESS_METADATA_KEY] = hostAccessSection
  }

  if (Object.keys(workspaceSection).length === 0) {
    delete nextMetadata[WORKSPACE_METADATA_KEY]
  } else {
    nextMetadata[WORKSPACE_METADATA_KEY] = workspaceSection
  }
  return nextMetadata
}

function normalizePathLines(value: string): string[] {
  return dedupePaths(value.split('\n').map(item => item.trim()).filter(Boolean))
}

function dedupePaths(paths: string[]): string[] {
  return [...new Set(paths.map(item => item.trim()).filter(Boolean))]
}
</script>
