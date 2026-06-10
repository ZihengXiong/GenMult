<template>
  <div class="space-y-6">
    <div class="space-y-4">
      <template
        v-for="(msg, i) in conversation"
        :key="i"
      >
        <div
          v-if="msg.role === 'assistant'"
          class="flex gap-3"
        >
          <Avatar class="size-8 shrink-0">
            <AvatarFallback class="text-xs bg-primary/10 text-primary">
              AI
            </AvatarFallback>
          </Avatar>
          <div class="rounded-2xl rounded-tl-sm bg-muted px-3 py-2 text-sm max-w-[80%]">
            {{ msg.text }}
          </div>
        </div>
        <div
          v-else
          class="flex justify-end"
        >
          <div class="rounded-2xl rounded-tr-sm bg-accent px-3 py-2 text-sm max-w-[80%]">
            {{ msg.text }}
          </div>
        </div>
      </template>
    </div>

    <div
      v-if="currentStep === 'name'"
      class="space-y-3"
    >
      <Input
        v-model="wizardData.display_name"
        :placeholder="$t('bots.wizard.namePlaceholder')"
        @keydown.enter="nextStep"
      />
      <Button
        size="sm"
        :disabled="!wizardData.display_name.trim()"
        @click="nextStep"
      >
        {{ $t('bots.wizard.continue') }}
      </Button>
    </div>

    <div
      v-else-if="currentStep === 'framework'"
      class="space-y-3"
    >
      <div class="flex flex-col gap-2">
        <button
          v-for="opt in frameworkOptions"
          :key="opt.value"
          type="button"
          class="flex items-start gap-3 rounded-lg border p-3 text-left text-sm transition-colors hover:bg-accent"
          :class="wizardData.framework === opt.value ? 'border-primary bg-primary/5' : 'border-border'"
          @click="wizardData.framework = opt.value; nextStep()"
        >
          <div>
            <div class="font-medium">
              {{ opt.label }}
            </div>
            <div class="text-xs text-muted-foreground">
              {{ opt.description }}
            </div>
          </div>
        </button>
      </div>
    </div>

    <div
      v-else-if="currentStep === 'prompt'"
      class="space-y-3"
    >
      <textarea
        v-model="wizardData.system_prompt"
        :placeholder="$t('bots.wizard.promptPlaceholder')"
        rows="3"
        class="flex min-h-[80px] w-full rounded-md border border-input bg-background px-3 py-2 text-sm placeholder:text-muted-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring resize-y"
      />
      <div class="flex gap-2">
        <Button
          size="sm"
          variant="outline"
          @click="skipStep"
        >
          {{ $t('bots.wizard.skip') }}
        </Button>
        <Button
          size="sm"
          @click="nextStep"
        >
          {{ $t('bots.wizard.continue') }}
        </Button>
      </div>
    </div>

    <div
      v-else-if="currentStep === 'capabilities'"
      class="space-y-3"
    >
      <div class="flex flex-wrap gap-2">
        <label
          v-for="cap in availableCaps"
          :key="cap"
          class="inline-flex items-center gap-1.5 rounded-full border px-3 py-1.5 text-xs cursor-pointer transition-colors"
          :class="wizardData.capabilities.includes(cap) ? 'bg-primary/10 border-primary text-primary' : 'border-border text-muted-foreground hover:border-foreground/30'"
        >
          <input
            type="checkbox"
            :checked="wizardData.capabilities.includes(cap)"
            class="sr-only"
            @change="toggleCap(cap)"
          >
          {{ cap }}
        </label>
      </div>
      <Button
        size="sm"
        @click="nextStep"
      >
        {{ $t('bots.wizard.continue') }}
      </Button>
    </div>

    <div
      v-else-if="currentStep === 'done'"
      class="space-y-3"
    >
      <div class="rounded-lg border p-4 space-y-2 text-sm">
        <div><span class="text-muted-foreground">{{ $t('bots.wizard.summaryName') }}</span> {{ wizardData.display_name }}</div>
        <div><span class="text-muted-foreground">{{ $t('bots.wizard.summaryFramework') }}</span> {{ wizardData.framework }}</div>
        <div v-if="wizardData.system_prompt">
          <span class="text-muted-foreground">{{ $t('bots.wizard.summaryPrompt') }}</span> {{ wizardData.system_prompt.slice(0, 100) }}{{ wizardData.system_prompt.length > 100 ? '...' : '' }}
        </div>
        <div v-if="wizardData.capabilities.length">
          <span class="text-muted-foreground">{{ $t('bots.wizard.summaryCapabilities') }}</span> {{ wizardData.capabilities.join(', ') }}
        </div>
      </div>
      <Button @click="$emit('submit', wizardData)">
        {{ $t('bots.wizard.createAgent') }}
      </Button>
    </div>
  </div>
</template>

<script setup lang="ts">
import { reactive, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { Avatar, AvatarFallback, Button, Input } from '@memohai/ui'

const { t } = useI18n()

defineEmits<{
  submit: [data: typeof wizardData]
}>()

const frameworkOptions = [
  { value: 'memoh', label: 'Memoh', description: t('bots.wizard.frameworks.memoh') },
  { value: 'claudecode', label: 'Claude Code', description: t('bots.wizard.frameworks.claudecode') },
  { value: 'codex', label: 'Codex', description: t('bots.wizard.frameworks.codex') },
]

const availableCaps = ['plan', 'code', 'test', 'review', 'edit', 'exec', 'search', 'memory']

type Step = 'name' | 'framework' | 'prompt' | 'capabilities' | 'done'

const currentStep = ref<Step>('name')

const wizardData = reactive({
  display_name: '',
  framework: 'memoh',
  system_prompt: '',
  capabilities: [] as string[],
})

const conversation = reactive<{ role: 'assistant' | 'user'; text: string }[]>([
  { role: 'assistant', text: t('bots.wizard.intro') },
])

const steps: Step[] = ['name', 'framework', 'prompt', 'capabilities', 'done']

function nextStep() {
  const idx = steps.indexOf(currentStep.value)
  if (currentStep.value === 'name') {
    conversation.push({ role: 'user', text: wizardData.display_name })
    conversation.push({ role: 'assistant', text: t('bots.wizard.nameConfirm', { name: wizardData.display_name }) })
  } else if (currentStep.value === 'framework') {
    const label = frameworkOptions.find(o => o.value === wizardData.framework)?.label ?? wizardData.framework
    conversation.push({ role: 'user', text: label })
    conversation.push({ role: 'assistant', text: t('bots.wizard.promptQuestion') })
  } else if (currentStep.value === 'prompt') {
    if (wizardData.system_prompt.trim()) {
      conversation.push({ role: 'user', text: wizardData.system_prompt.slice(0, 80) + (wizardData.system_prompt.length > 80 ? '...' : '') })
    }
    conversation.push({ role: 'assistant', text: t('bots.wizard.capabilitiesQuestion') })
  } else if (currentStep.value === 'capabilities') {
    if (wizardData.capabilities.length) {
      conversation.push({ role: 'user', text: wizardData.capabilities.join(', ') })
    }
    conversation.push({ role: 'assistant', text: t('bots.wizard.summaryQuestion') })
  }
  if (idx < steps.length - 1) {
    currentStep.value = steps[idx + 1]
  }
}

function skipStep() {
  conversation.push({ role: 'user', text: t('bots.wizard.skipped') })
  nextStep()
}

function toggleCap(cap: string) {
  const idx = wizardData.capabilities.indexOf(cap)
  if (idx >= 0) wizardData.capabilities.splice(idx, 1)
  else wizardData.capabilities.push(cap)
}
</script>
