<template>
  <div class="space-y-6 mt-4 pt-4 border-t border-border">
    <div>
      <h3 class="text-lg font-medium">
        Claude Code Settings
      </h3>
      <p class="text-sm text-muted-foreground">
        Configuration options specific to the Claude Code CLI framework.
      </p>
    </div>

    <div class="space-y-4 rounded-md border p-4">
      <div class="flex items-center justify-between">
        <div class="space-y-0.5">
          <Label>Use Custom API Credentials</Label>
          <p class="text-xs text-muted-foreground">
            Override the global API settings specifically for this bot.
          </p>
        </div>
        <Switch
          :model-value="!!config.override_credentials"
          @update:model-value="(val) => config.override_credentials = !!val"
        />
      </div>

      <template v-if="config.override_credentials">
        <div class="space-y-2">
          <Label>API Base URL</Label>
          <Input
            v-model="config.base_url"
            placeholder="https://api.deepseek.com/anthropic"
          />
          <p class="text-xs text-muted-foreground">
            Optional. Overrides the default Anthropic API endpoint.
          </p>
        </div>

        <div class="space-y-2">
          <Label>Anthropic Auth Token</Label>
          <Input
            v-model="config.auth_token"
            type="password"
            placeholder="sk-..."
          />
          <p class="text-xs text-muted-foreground">
            Used for DeepSeek compatibility (ANTHROPIC_AUTH_TOKEN).
          </p>
        </div>

        <div class="space-y-2">
          <Label>Anthropic API Key</Label>
          <Input
            v-model="config.api_key"
            type="password"
            placeholder="sk-..."
          />
          <p class="text-xs text-muted-foreground">
            Optional. Overrides the global API key for this bot.
          </p>
        </div>
      </template>
      <div
        v-else
        class="text-xs text-muted-foreground bg-muted/40 p-2 rounded-md"
      >
        Currently using global/main provider credentials.
      </div>
    </div>

    <div class="space-y-2">
      <Label>Model</Label>
      <Input
        v-model="config.model"
        list="claude-models-list"
        placeholder="e.g. sonnet, deepseek-v4-flash, or leave empty for global default"
      />
      <datalist id="claude-models-list">
        <option value="sonnet">
          Claude 3.7 Sonnet (Default)
        </option>
        <option value="opus">
          Claude 3 Opus
        </option>
        <option value="haiku">
          Claude 3.5 Haiku
        </option>
        <option
          v-for="m in props.models"
          :key="m.id"
          :value="m.model_id"
        >
          {{ m.name }}
        </option>
      </datalist>
      <p class="text-xs text-muted-foreground">
        Overrides the global default model. You can type any compatible model name or pick from the list.
      </p>
    </div>

    <div class="space-y-2">
      <Label>Permission Mode</Label>
      <Select v-model="config.permission_mode">
        <SelectTrigger>
          <SelectValue placeholder="Select Permission Mode" />
        </SelectTrigger>
        <SelectContent>
          <SelectItem value="auto">
            Auto Edit (Default)
          </SelectItem>
          <SelectItem value="auto-accept">
            Auto Accept (Risky)
          </SelectItem>
          <SelectItem value="interactive">
            Interactive
          </SelectItem>
        </SelectContent>
      </Select>
      <p class="text-xs text-muted-foreground">
        Determines how Claude Code handles tool execution approvals.
      </p>
    </div>

    <div class="space-y-2">
      <Label>Max Turns</Label>
      <Input
        v-model.number="config.max_turns"
        type="number"
        placeholder="15"
      />
      <p class="text-xs text-muted-foreground">
        Maximum number of agent iteration turns allowed per run (CLI --max-turns).
      </p>
    </div>

    <div class="space-y-2">
      <Label>Max Context Messages</Label>
      <Input
        v-model.number="config.max_context_messages"
        type="number"
        placeholder="15"
      />
      <p class="text-xs text-muted-foreground">
        How many recent conversation messages (user/assistant) to replay as
        context for multi-turn memory. Defaults to 15.
      </p>
    </div>

    <div class="space-y-2">
      <Label>Allowed Tools</Label>
      <Input
        v-model="allowedToolsStr"
        placeholder="e.g. Bash(git *), Bash(npm run lint)"
      />
      <p class="text-xs text-muted-foreground">
        Comma-separated list of tool signatures to automatically allow.
      </p>
    </div>

    <div class="space-y-2">
      <Label>Custom Environment Variables</Label>
      <div class="space-y-2">
        <div
          v-for="(v, idx) in envList"
          :key="idx"
          class="flex items-center gap-2"
        >
          <Input
            v-model="v.key"
            placeholder="Key (e.g. CLAUDE_CODE_SUBAGENT_MODEL)"
            class="flex-1"
          />
          <Input
            v-model="v.value"
            placeholder="Value"
            class="flex-1"
          />
          <Button
            variant="ghost"
            size="icon"
            class="shrink-0"
            @click="removeEnv(idx)"
          >
            <Trash class="size-4 text-destructive" />
          </Button>
        </div>
      </div>
      <Button
        variant="outline"
        size="sm"
        @click="addEnv"
      >
        <Plus class="size-4 mr-2" />
        Add Variable
      </Button>
      <p class="text-xs text-muted-foreground mt-2">
        Variables defined here will override system-wide environment variables for this bot.
      </p>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { watch, ref } from 'vue'
import {
  Label,
  Input,
  Select,
  SelectTrigger,
  SelectValue,
  SelectContent,
  SelectItem,
  Button,
  Switch
} from '@memohai/ui'
import { Plus, Trash } from 'lucide-vue-next'

import type { ModelsGetResponse } from '@memohai/sdk'

const props = defineProps<{
  models?: ModelsGetResponse[]
}>()

const config = defineModel<Record<string, unknown>>({ default: () => ({}) })

const allowedToolsStr = computed({
  get() {
    return (config.value.allowed_tools || []).join(', ')
  },
  set(val: string) {
    if (!val.trim()) {
      config.value.allowed_tools = []
    } else {
      config.value.allowed_tools = val.split(',').map(s => s.trim()).filter(Boolean)
    }
  }
})

const envList = ref<{key: string, value: string}[]>([])

watch(() => config.value.custom_env, (newVal) => {
  if (newVal && typeof newVal === 'object') {
    const currentListStr = JSON.stringify(envList.value.filter(e => e.key).map(e => [e.key, e.value]))
    const newListStr = JSON.stringify(Object.entries(newVal))
    if (currentListStr !== newListStr) {
      envList.value = Object.entries(newVal).map(([k, v]) => ({ key: k, value: String(v) }))
    }
  }
}, { immediate: true })

watch(envList, (newVal) => {
  const env: Record<string, string> = {}
  newVal.forEach(item => {
    if (item.key.trim()) {
      env[item.key.trim()] = item.value
    }
  })
  config.value.custom_env = env
}, { deep: true })

function addEnv() {
  envList.value.push({ key: '', value: '' })
}

function removeEnv(index: number) {
  envList.value.splice(index, 1)
}
</script>
