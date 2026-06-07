<template>
  <section class="p-4 mx-auto max-w-2xl">
    <h2 class="text-lg font-semibold mb-6">
      {{ $t('bots.createBot') }}
    </h2>

    <form @submit.prevent="handleSubmit">
      <!-- Basic Info -->
      <div>
        <h3 class="text-sm font-medium mb-4">
          {{ $t('bots.steps.basicInfo') }}
        </h3>
        <div class="flex items-start gap-4">
          <div class="group/avatar relative size-16 shrink-0 rounded-full overflow-hidden cursor-pointer">
            <Avatar class="size-16 rounded-full">
              <AvatarImage
                v-if="form.avatar_url?.trim()"
                :src="form.avatar_url.trim()"
                :alt="form.display_name"
              />
              <AvatarFallback class="text-xl">
                {{ avatarFallback }}
              </AvatarFallback>
            </Avatar>
            <button
              type="button"
              class="absolute inset-0 flex items-center justify-center rounded-full bg-black/40 opacity-0 transition-opacity group-hover/avatar:opacity-100"
              :title="$t('common.edit')"
              :aria-label="$t('common.edit')"
              @click="avatarDialogOpen = true"
            >
              <SquarePen class="size-6 text-white" />
            </button>
          </div>
          <div class="flex-1 min-w-0">
            <Label class="mb-2">
              {{ $t('bots.displayName') }}
              <span class="text-destructive">*</span>
            </Label>
            <Input
              v-model="form.display_name"
              type="text"
              :placeholder="$t('bots.displayNamePlaceholder')"
            />
          </div>
        </div>
      </div>

      <Separator class="my-6" />

      <!-- Agent Framework (placed early so subsequent sections adapt) -->
      <div>
        <h3 class="text-sm font-medium mb-4">
          {{ $t('bots.steps.framework') }}
          <span class="text-destructive">*</span>
        </h3>
        <div class="flex flex-col gap-3">
          <Label>{{ $t('bots.framework') }}</Label>
          <Select v-model="form.framework">
            <SelectTrigger class="w-full">
              <SelectValue :placeholder="$t('bots.frameworkPlaceholder') || $t('bots.framework')" />
            </SelectTrigger>
            <SelectContent>
              <SelectItem
                v-for="opt in frameworkOptions"
                :key="opt.value"
                :value="opt.value"
              >
                <div class="flex flex-col">
                  <span>{{ opt.label }}</span>
                  <span class="text-xs text-muted-foreground">{{ opt.description }}</span>
                </div>
              </SelectItem>
            </SelectContent>
          </Select>
          <p class="text-xs text-muted-foreground">
            {{ $t('bots.frameworkHelp') }}
          </p>
        </div>
      </div>

      <Separator class="my-6" />

      <!-- Workspace (conditional) -->
      <template v-if="localWorkspaceEnabled">
        <div>
          <h3 class="text-sm font-medium mb-4">
            {{ $t('bots.steps.workspace') }}
          </h3>
          <div class="flex flex-col gap-4">
            <div>
              <div class="mb-2 flex items-center gap-2">
                <Label>{{ $t('bots.workspaceBackend') }}</Label>
                <Tooltip>
                  <TooltipTrigger as-child>
                    <Button
                      type="button"
                      variant="ghost"
                      size="icon-sm"
                      class="size-5 text-muted-foreground hover:text-foreground"
                    >
                      <CircleHelp class="size-3.5" />
                    </Button>
                  </TooltipTrigger>
                  <TooltipContent class="max-w-80 text-left leading-relaxed">
                    {{ $t('bots.workspaceBackendHint') }}
                  </TooltipContent>
                </Tooltip>
              </div>
              <Select v-model="form.workspace_backend">
                <SelectTrigger class="w-full">
                  <SelectValue :placeholder="$t('bots.workspaceBackend')" />
                </SelectTrigger>
                <SelectContent>
                  <SelectItem value="container">
                    {{ $t('bots.workspaceBackends.container') }}
                  </SelectItem>
                  <SelectItem value="local">
                    {{ $t('bots.workspaceBackends.local') }}
                  </SelectItem>
                </SelectContent>
              </Select>
            </div>

            <template v-if="form.workspace_backend === 'local'">
              <div>
                <Label class="mb-2">
                  {{ $t('bots.localWorkspacePath') }}
                  <span class="text-destructive">*</span>
                </Label>
                <div class="flex items-center gap-2">
                  <div
                    class="flex h-9 min-w-0 flex-1 items-center rounded-md border bg-background px-3 text-sm"
                    :class="localWorkspaceError ? 'border-destructive' : 'border-input'"
                  >
                    <Folder
                      v-if="form.local_workspace_path"
                      class="mr-2 size-4 shrink-0 text-muted-foreground"
                    />
                    <span
                      v-if="form.local_workspace_path"
                      class="truncate"
                    >{{ form.local_workspace_path }}</span>
                    <span
                      v-else
                      class="truncate text-muted-foreground"
                    >{{ $t('bots.localWorkspacePathPlaceholder') }}</span>
                    <Spinner
                      v-if="localWorkspaceValidating"
                      class="ml-auto size-4 shrink-0"
                    />
                  </div>
                  <Button
                    type="button"
                    variant="outline"
                    size="sm"
                    class="shrink-0 gap-1.5"
                    @click="openFolderBrowser"
                  >
                    <FolderOpen class="size-3.5" />
                    选择文件夹
                  </Button>
                </div>
                <p
                  v-if="localWorkspaceError"
                  class="text-xs text-destructive mt-1.5"
                >
                  {{ localWorkspaceError }}
                </p>
              </div>
              <div class="rounded-md border border-warning-border bg-warning-soft px-3 py-2 text-xs text-warning-foreground">
                {{ $t('bots.localWorkspaceWarning') }}
              </div>

              <Dialog
                v-model:open="folderDialogOpen"
              >
                <DialogContent class="sm:max-w-lg">
                  <DialogHeader>
                    <DialogTitle>选择工作目录</DialogTitle>
                  </DialogHeader>

                  <div class="space-y-3">
                    <div class="flex items-center gap-2 rounded-md border border-border bg-muted/50 px-3 py-2">
                      <Folder class="size-4 shrink-0 text-muted-foreground" />
                      <span class="min-w-0 flex-1 truncate text-sm">{{ folderBrowserPath }}</span>
                    </div>

                    <ScrollArea class="h-[320px] rounded-md border border-border">
                      <div
                        v-if="folderBrowserLoading"
                        class="flex items-center justify-center py-12 text-sm text-muted-foreground"
                      >
                        <Spinner class="mr-2" />
                        加载中…
                      </div>
                      <div
                        v-else
                        class="p-1"
                      >
                        <button
                          v-if="folderBrowserParent"
                          type="button"
                          class="flex w-full items-center gap-2 rounded-md px-3 py-2 text-sm transition-colors hover:bg-accent"
                          @click="browseTo(folderBrowserParent)"
                        >
                          <FolderUp class="size-4 shrink-0 text-muted-foreground" />
                          <span class="text-muted-foreground">..</span>
                        </button>
                        <button
                          v-for="entry in folderBrowserEntries"
                          :key="entry.path"
                          type="button"
                          class="flex w-full items-center gap-2 rounded-md px-3 py-2 text-sm transition-colors hover:bg-accent"
                          @click="browseTo(entry.path)"
                        >
                          <Folder class="size-4 shrink-0 text-amber-500" />
                          <span class="min-w-0 flex-1 truncate text-left">{{ entry.name }}</span>
                          <ChevronRight class="size-3.5 shrink-0 text-muted-foreground" />
                        </button>
                        <div
                          v-if="!folderBrowserEntries.length && !folderBrowserParent"
                          class="py-8 text-center text-sm text-muted-foreground"
                        >
                          空目录
                        </div>
                      </div>
                    </ScrollArea>
                  </div>

                  <DialogFooter>
                    <DialogClose as-child>
                      <Button variant="outline">
                        取消
                      </Button>
                    </DialogClose>
                    <Button @click="selectFolder">
                      选择当前目录
                    </Button>
                  </DialogFooter>
                </DialogContent>
              </Dialog>
            </template>
          </div>
        </div>

        <Separator class="my-6" />
      </template>

      <!-- Security Policy -->
      <div>
        <h3 class="text-sm font-medium mb-4">
          {{ $t('bots.steps.security') }}
        </h3>
        <div class="flex flex-col gap-3">
          <div class="mb-2 flex items-center gap-2">
            <Label>
              {{ $t('bots.aclPreset') }}
              <span class="text-destructive">*</span>
            </Label>
            <Tooltip>
              <TooltipTrigger as-child>
                <Button
                  type="button"
                  variant="ghost"
                  size="icon-sm"
                  class="size-5 text-muted-foreground hover:text-foreground"
                >
                  <CircleHelp class="size-3.5" />
                </Button>
              </TooltipTrigger>
              <TooltipContent class="max-w-80 text-left leading-relaxed">
                {{ $t('bots.aclPresetHelp') }}
              </TooltipContent>
            </Tooltip>
          </div>
          <Select v-model="form.acl_preset">
            <SelectTrigger class="w-full">
              <SelectValue :placeholder="$t('bots.aclPreset')" />
            </SelectTrigger>
            <SelectContent>
              <SelectItem
                v-for="preset in aclPresetOptions"
                :key="preset.value"
                :value="preset.value"
              >
                {{ $t(preset.titleKey) }}
              </SelectItem>
            </SelectContent>
          </Select>
          <p
            v-if="aclDescription"
            class="text-xs text-muted-foreground"
          >
            {{ aclDescription }}
          </p>
        </div>
      </div>

      <Separator class="my-6" />

      <!-- Model (only for memoh framework) -->
      <template v-if="form.framework === 'memoh'">
        <div>
          <h3 class="text-sm font-medium mb-4">
            {{ $t('bots.steps.model') }}
          </h3>
          <p class="text-xs text-muted-foreground mb-3">
            {{ $t('bots.steps.modelDesc') }}
          </p>
          <Label class="mb-2">{{ $t('bots.settings.chatModel') }}</Label>
          <ModelSelect
            v-model="form.chat_model_id"
            :models="models"
            :providers="providers"
            model-type="chat"
            :placeholder="$t('common.none')"
          />
        </div>

        <Separator class="my-6" />

        <!-- Memory -->
        <div>
          <h3 class="text-sm font-medium mb-4">
            {{ $t('bots.steps.memory') }}
          </h3>
          <p class="text-xs text-muted-foreground mb-3">
            {{ $t('bots.steps.memoryDesc') }}
          </p>
          <Label class="mb-2">{{ $t('bots.settings.memoryProvider') }}</Label>
          <MemoryProviderSelect
            v-model="form.memory_provider_id"
            :providers="memoryProviders"
            :placeholder="$t('common.none')"
          />
        </div>
      </template>

      <Separator class="my-6" />

      <!-- Settings -->
      <div>
        <h3 class="text-sm font-medium mb-4">
          {{ $t('bots.steps.settings') }}
        </h3>
        <Label class="mb-2">
          {{ $t('bots.timezone') }}
          <span class="text-muted-foreground text-xs ml-1">({{ $t('common.optional') }})</span>
        </Label>
        <TimezoneSelect
          v-model="form.timezone"
          :placeholder="$t('bots.timezonePlaceholder')"
          allow-empty
          :empty-label="$t('bots.timezoneInherited')"
        />
      </div>

      <!-- Hint -->
      <div class="rounded-md border bg-muted/40 px-3 py-2 text-xs text-muted-foreground mt-6">
        {{ $t('bots.createBotWaitHint') }}
      </div>

      <!-- Actions -->
      <div class="flex justify-end gap-3 mt-6 pb-4">
        <Button
          type="button"
          variant="outline"
          @click="router.back()"
        >
          {{ $t('common.cancel') }}
        </Button>
        <Button
          type="submit"
          :disabled="!canSubmit || submitLoading"
        >
          <Spinner v-if="submitLoading" />
          {{ $t('bots.createBot') }}
        </Button>
      </div>
    </form>

    <AvatarEditDialog
      v-model:open="avatarDialogOpen"
      v-model:avatar-url="form.avatar_url"
      :fallback-text="avatarFallback"
    />
  </section>
</template>

<script setup lang="ts">
import {
  Avatar,
  AvatarImage,
  AvatarFallback,
  Button,
  Input,
  Label,
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
  Separator,
  Spinner,
  Tooltip,
  TooltipContent,
  TooltipTrigger,
} from '@memohai/ui'
import { Dialog, DialogContent, DialogHeader, DialogTitle, DialogFooter, DialogClose, ScrollArea } from '@memohai/ui'
import { SquarePen, CircleHelp, FolderOpen, FolderUp, Folder, ChevronRight } from 'lucide-vue-next'
import { ref, reactive, computed, watch, onMounted } from 'vue'
import { useRouter } from 'vue-router'
import { toast } from 'vue-sonner'
import { useI18n } from 'vue-i18n'
import { useQuery, useMutation, useQueryCache } from '@pinia/colada'
import { getModels, getProviders, getMemoryProviders, putBotsByBotIdSettings } from '@memohai/sdk'
import { client } from '@memohai/sdk/client'
import { postBotsMutation, getBotsQueryKey } from '@memohai/sdk/colada'
import { useCapabilitiesStore } from '@/store/capabilities'
import { useAvatarInitials } from '@/composables/useAvatarInitials'
import { resolveApiErrorMessage } from '@/utils/api-error'
import { aclPresetOptions, defaultAclPreset } from '@/constants/acl-presets'
import { emptyTimezoneValue } from '@/utils/timezones'
import TimezoneSelect from '@/components/timezone-select/index.vue'
import ModelSelect from './components/model-select.vue'
import MemoryProviderSelect from './components/memory-provider-select.vue'
import AvatarEditDialog from './components/avatar-edit-dialog.vue'

const router = useRouter()
const { t } = useI18n()
const queryCache = useQueryCache()
const capabilities = useCapabilitiesStore()

onMounted(() => {
  void capabilities.load()
})

const localWorkspaceEnabled = computed(() => capabilities.localWorkspaceEnabled)

const frameworkOptions = [
  { value: 'memoh', label: 'Memoh', description: '内置 AI Agent，支持模型、记忆、工具调用' },
  { value: 'claudecode', label: 'Claude Code', description: 'Anthropic CLI Agent，自带模型和工具链' },
  { value: 'codex', label: 'Codex', description: 'OpenAI CLI Agent，自带模型和工具链' },
] as const

const form = reactive({
  display_name: '',
  avatar_url: '',
  acl_preset: defaultAclPreset as string,
  framework: '' as string,
  chat_model_id: '',
  memory_provider_id: '',
  timezone: emptyTimezoneValue,
  workspace_backend: 'container',
  local_workspace_path: '',
})

watch(localWorkspaceEnabled, (enabled) => {
  if (enabled) {
    form.workspace_backend = 'local'
  }
}, { immediate: true })

const localPathTouched = ref(false)

watch([() => form.display_name, () => form.workspace_backend], async ([displayName, backend]) => {
  if (backend !== 'local' || !displayName?.trim()) return
  try {
    const api = (window as Record<string, unknown>).api as
      | { desktop?: { defaultWorkspacePath?: (name: string) => Promise<string> } }
      | undefined
    const path = await api?.desktop?.defaultWorkspacePath?.(displayName.trim())
    if (path && !localPathTouched.value) {
      form.local_workspace_path = path
    }
  } catch {
    // Not in Electron or IPC unavailable
  }
})

const localWorkspaceError = ref('')
const localWorkspaceValidating = ref(false)

let debounceTimeout: ReturnType<typeof setTimeout> | null = null

watch(() => form.local_workspace_path, (newPath) => {
  localPathTouched.value = true
  localWorkspaceError.value = ''
  if (form.workspace_backend !== 'local') return
  if (!newPath.trim()) {
    localWorkspaceError.value = t('common.required') || 'Required'
    return
  }

  if (debounceTimeout) {
    clearTimeout(debounceTimeout)
  }

  debounceTimeout = setTimeout(async () => {
    localWorkspaceValidating.value = true
    try {
      const { data } = await client.post<{ 200: { valid: boolean; error?: string } }, { path: string }, true>({
        url: '/system/validate-directory',
        body: { path: newPath.trim() },
        throwOnError: true,
      })
      if (data && !data.valid) {
        localWorkspaceError.value = data.error || 'Invalid directory'
      }
    } catch (err: unknown) {
      localWorkspaceError.value = (err instanceof Error ? err.message : null) || 'Failed to validate directory'
    } finally {
      localWorkspaceValidating.value = false
    }
  }, 500)
})

interface DirEntry {
  name: string
  path: string
  is_dir: boolean
}

const folderDialogOpen = ref(false)
const folderBrowserPath = ref('')
const folderBrowserParent = ref('')
const folderBrowserEntries = ref<DirEntry[]>([])
const folderBrowserLoading = ref(false)

async function openFolderBrowser() {
  folderDialogOpen.value = true
  await browseTo(form.local_workspace_path || '')
}

async function browseTo(path: string) {
  folderBrowserLoading.value = true
  try {
    const { data } = await client.post<{ 200: { path: string; parent: string; entries: DirEntry[] } }, { path: string }, true>({
      url: '/system/list-directory',
      body: { path: path.trim() },
      throwOnError: true,
    })
    folderBrowserPath.value = data.path
    folderBrowserParent.value = data.parent || ''
    folderBrowserEntries.value = data.entries || []
  } catch {
    folderBrowserEntries.value = []
  } finally {
    folderBrowserLoading.value = false
  }
}

function selectFolder() {
  form.local_workspace_path = folderBrowserPath.value
  folderDialogOpen.value = false
}

const avatarDialogOpen = ref(false)
const avatarFallback = useAvatarInitials(() => form.display_name || '')

// Data queries
const { data: modelData } = useQuery({
  key: ['models'],
  query: async () => {
    const { data } = await getModels({ throwOnError: true })
    return data
  },
})

const { data: providerData } = useQuery({
  key: ['providers'],
  query: async () => {
    const { data } = await getProviders({ throwOnError: true })
    return data
  },
})

const { data: memoryProviderData } = useQuery({
  key: ['memory-providers'],
  query: async () => {
    const { data } = await getMemoryProviders({ throwOnError: true })
    return data
  },
})

const models = computed(() => modelData.value ?? [])
const providers = computed(() => providerData.value ?? [])
const memoryProviders = computed(() => memoryProviderData.value ?? [])
const codexProviderIds = computed(() =>
  new Set(
    providers.value
      .filter((provider) => provider.client_type === 'openai-codex')
      .map((provider) => provider.id)
      .filter((id): id is string => Boolean(id)),
  ),
)
const chatSelectableModels = computed(() => {
  if (form.framework !== 'codex') return models.value
  return models.value.filter((model) => codexProviderIds.value.has(model.provider_id ?? ''))
})

watch([() => form.framework, chatSelectableModels], () => {
  if (!form.chat_model_id) return
  if (chatSelectableModels.value.some((model) => model.id === form.chat_model_id)) return
  form.chat_model_id = ''
}, { immediate: true })

watch(memoryProviders, (list) => {
  if (form.memory_provider_id) return
  const builtin = list.find(p => p.provider === 'builtin')
  if (builtin?.id) {
    form.memory_provider_id = builtin.id
  }
}, { immediate: true })

// ACL description
const aclDescription = computed(() => {
  const opt = aclPresetOptions.find(o => o.value === form.acl_preset)
  return opt ? t(opt.descriptionKey) : ''
})

// Validation
const canSubmit = computed(() => {
  if (!form.display_name.trim()) return false
  if (!form.framework) return false
  if (!form.acl_preset) return false
  if (form.framework === 'codex' && !form.chat_model_id) return false
  if (localWorkspaceEnabled.value && form.workspace_backend === 'local') {
    if (!form.local_workspace_path.trim()) return false
    if (localWorkspaceError.value) return false
    if (localWorkspaceValidating.value) return false
  }
  return true
})

// Submit
const { mutateAsync: createBot, isLoading: submitLoading } = useMutation({
  ...postBotsMutation(),
  onSettled: () => queryCache.invalidateQueries({ key: getBotsQueryKey() }),
})

async function handleSubmit() {
  if (!canSubmit.value || submitLoading.value) return

  const metadata = localWorkspaceEnabled.value && form.workspace_backend === 'local'
    ? {
        workspace: {
          backend: 'local',
          local_workspace_path: form.local_workspace_path,
        },
      }
    : undefined

  const tz = form.timezone === emptyTimezoneValue ? undefined : form.timezone || undefined

  try {
    const bot = await createBot({
      body: {
        display_name: form.display_name.trim(),
        avatar_url: form.avatar_url.trim() || undefined,
        timezone: tz,
        is_active: true,
        acl_preset: form.acl_preset,
        framework: form.framework,
        metadata,
      },
    })

    const botId = bot?.id
    if (botId && (form.chat_model_id || form.memory_provider_id)) {
      try {
        await putBotsByBotIdSettings({
          path: { bot_id: botId },
          body: {
            ...(form.chat_model_id ? { chat_model_id: form.chat_model_id } : {}),
            ...(form.memory_provider_id ? { memory_provider_id: form.memory_provider_id } : {}),
          },
          throwOnError: true,
        })
      } catch {
        // Bot created successfully, settings save failed — non-fatal
      }
    }

    toast.success(t('bots.createBotSuccess'))
    if (botId) {
      router.push({ name: 'bot-detail', params: { botId } })
    } else {
      router.push({ name: 'bots' })
    }
  } catch (error) {
    toast.error(resolveApiErrorMessage(error, t('common.saveFailed')))
  }
}
</script>
