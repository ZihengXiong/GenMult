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

    <div class="space-y-2">
      <Label>Model</Label>
      <Input
        v-model="config.model"
        list="claude-models-list"
        placeholder="e.g. sonnet, deepseek-v4-flash, or leave empty for global default"
      />
      <datalist id="claude-models-list">
        <option value="sonnet">Claude 3.7 Sonnet (Default)</option>
        <option value="opus">Claude 3 Opus</option>
        <option value="haiku">Claude 3.5 Haiku</option>
        <option v-for="m in props.models" :key="m.id" :value="m.model_id">
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
        Maximum number of conversation turns allowed per run.
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
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import {
  Label,
  Input,
  Select,
  SelectTrigger,
  SelectValue,
  SelectContent,
  SelectItem
} from '@memohai/ui'

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
</script>
