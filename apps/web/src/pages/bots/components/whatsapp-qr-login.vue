<template>
  <div class="space-y-4">
    <div class="flex items-center justify-between">
      <div>
        <h4 class="text-xs font-medium">
          {{ $t('bots.channels.whatsappQr.title') }}
        </h4>
        <p class="text-xs text-muted-foreground mt-1">
          {{ $t('bots.channels.whatsappQr.description') }}
        </p>
      </div>
    </div>

    <div
      v-if="qrState === 'idle'"
      class="flex flex-col items-center gap-3 py-4"
    >
      <Button
        :disabled="isStarting"
        @click="startLogin"
      >
        <Spinner
          v-if="isStarting"
          class="mr-1.5"
        />
        <QrCode
          v-else
          class="mr-1.5 size-3.5"
        />
        {{ $t('bots.channels.whatsappQr.startScan') }}
      </Button>
    </div>

    <div
      v-else-if="qrState === 'showing'"
      class="flex flex-col items-center gap-4 py-4"
    >
      <div class="relative rounded-lg border bg-white p-3">
        <img
          v-if="qrImageDataUrl"
          :src="qrImageDataUrl"
          alt="WhatsApp QR Code"
          class="size-52"
        >
        <div
          v-else
          class="size-52 flex items-center justify-center text-muted-foreground"
        >
          <Spinner />
        </div>

        <div
          v-if="pollStatus === 'scanned'"
          class="absolute inset-0 flex items-center justify-center rounded-lg bg-background/80"
        >
          <div class="text-center">
            <Smartphone class="size-8 text-primary mb-2" />
            <p class="text-xs font-medium text-foreground">
              {{ $t('bots.channels.whatsappQr.scanned') }}
            </p>
          </div>
        </div>

        <div
          v-if="pollStatus === 'expired'"
          class="absolute inset-0 flex flex-col items-center justify-center rounded-lg bg-background/80 gap-2"
        >
          <p class="text-xs text-muted-foreground">
            {{ $t('bots.channels.whatsappQr.expired') }}
          </p>
          <Button
            size="sm"
            variant="outline"
            @click="startLogin"
          >
            {{ $t('bots.channels.whatsappQr.refresh') }}
          </Button>
        </div>
      </div>

      <p class="text-xs text-muted-foreground text-center max-w-xs">
        {{ statusText }}
      </p>

      <div
        v-if="pairCode"
        class="rounded-md border bg-muted/30 px-3 py-2 text-center"
      >
        <p class="text-xs text-muted-foreground">
          {{ $t('bots.channels.whatsappQr.pairCodeHint') }}
        </p>
        <p class="font-mono text-lg font-semibold tracking-widest mt-1">
          {{ pairCode }}
        </p>
      </div>

      <div
        v-else
        class="w-full max-w-xs"
      >
        <button
          v-if="!showPairForm"
          type="button"
          class="text-xs text-primary hover:underline"
          @click="showPairForm = true"
        >
          {{ $t('bots.channels.whatsappQr.usePairCode') }}
        </button>
        <div
          v-else
          class="flex flex-col gap-2"
        >
          <Input
            v-model="phoneInput"
            type="tel"
            :placeholder="$t('bots.channels.whatsappQr.pairCodePhonePlaceholder')"
            inputmode="numeric"
          />
          <Button
            size="sm"
            :disabled="isRequestingPairCode || !phoneInput"
            @click="requestPairCode"
          >
            <Spinner
              v-if="isRequestingPairCode"
              class="mr-1.5"
            />
            {{ $t('bots.channels.whatsappQr.pairCodeRequest') }}
          </Button>
          <button
            type="button"
            class="text-xs text-muted-foreground hover:underline"
            @click="showPairForm = false"
          >
            {{ $t('bots.channels.whatsappQr.useQr') }}
          </button>
        </div>
      </div>

      <Button
        variant="ghost"
        size="sm"
        @click="cancel"
      >
        {{ $t('common.cancel') }}
      </Button>
    </div>

    <div
      v-else-if="qrState === 'success'"
      class="flex flex-col items-center gap-3 py-4"
    >
      <div class="flex size-12 items-center justify-center rounded-full bg-success-soft">
        <Check class="size-5 text-success-foreground" />
      </div>
      <p class="text-xs font-medium">
        {{ $t('bots.channels.whatsappQr.success') }}
      </p>
    </div>

    <div
      v-else-if="qrState === 'error'"
      class="flex flex-col items-center gap-3 py-4"
    >
      <p class="text-xs text-destructive">
        {{ errorMessage }}
      </p>
      <Button
        variant="outline"
        size="sm"
        @click="startLogin"
      >
        {{ $t('bots.channels.whatsappQr.retry') }}
      </Button>
    </div>
  </div>
</template>

<script setup lang="ts">
import { QrCode, Smartphone, Check } from 'lucide-vue-next'
import { ref, computed, onUnmounted } from 'vue'
import { Button, Spinner, Input } from '@memohai/ui'
import { useI18n } from 'vue-i18n'
import { toast } from 'vue-sonner'
import QRCode from 'qrcode'
import { client } from '@memohai/sdk/client'
import { resolveApiErrorMessage } from '@/utils/api-error'

const props = defineProps<{
  botId: string
  proxy?: string
  clientName?: string
}>()

const emit = defineEmits<{
  loginSuccess: []
}>()

const { t } = useI18n()

type QRState = 'idle' | 'showing' | 'success' | 'error'

interface WhatsAppQrResponse {
  qr_code?: string
  pair_code?: string
  status?: string
  message?: string
}

const qrState = ref<QRState>('idle')
const qrImageDataUrl = ref('')
const pollStatus = ref('')
const isStarting = ref(false)
const errorMessage = ref('')
const pairCode = ref('')
const phoneInput = ref('')
const showPairForm = ref(false)
const isRequestingPairCode = ref(false)
let pollTimer: ReturnType<typeof setTimeout> | null = null
let aborted = false

const linkingOptions = computed(() => {
  const body: Record<string, string> = {}
  if (props.proxy?.trim()) {
    body.proxy = props.proxy.trim()
  }
  if (props.clientName?.trim()) {
    body.clientName = props.clientName.trim()
  }
  return body
})

const statusText = computed(() => {
  switch (pollStatus.value) {
    case 'code':
    case 'wait':
      return t('bots.channels.whatsappQr.waitingScan')
    case 'scanned':
      return t('bots.channels.whatsappQr.scanned')
    case 'expired':
      return t('bots.channels.whatsappQr.expired')
    case 'pair_code':
      return t('bots.channels.whatsappQr.pairCodeWaiting')
    default:
      return t('bots.channels.whatsappQr.waitingScan')
  }
})

async function setQrImage(qrCode: string) {
  qrImageDataUrl.value = await QRCode.toDataURL(qrCode, { width: 208, margin: 1 })
}

async function startLogin() {
  aborted = false
  isStarting.value = true
  errorMessage.value = ''
  pollStatus.value = ''
  qrImageDataUrl.value = ''
  pairCode.value = ''
  showPairForm.value = false

  try {
    const { data } = await client.post<{ 200: WhatsAppQrResponse }, unknown, true>({
      url: '/bots/{bot_id}/channel/whatsapp/qr/start',
      path: { bot_id: props.botId },
      body: linkingOptions.value,
      throwOnError: true,
    })
    if (data.status === 'confirmed') {
      qrState.value = 'success'
      toast.success(t('bots.channels.whatsappQr.success'))
      emit('loginSuccess')
      return
    }
    if (data.qr_code) {
      await setQrImage(data.qr_code)
    }
    qrState.value = 'showing'
    pollStatus.value = data.status ?? 'wait'
    startPolling()
  } catch (err) {
    errorMessage.value = resolveApiErrorMessage(err, err instanceof Error ? err.message : String(err))
    qrState.value = 'error'
  } finally {
    isStarting.value = false
  }
}

async function requestPairCode() {
  if (!phoneInput.value || isRequestingPairCode.value) return
  isRequestingPairCode.value = true
  try {
    const { data } = await client.post<{ 200: WhatsAppQrResponse }, unknown, true>({
      url: '/bots/{bot_id}/channel/whatsapp/pair',
      path: { bot_id: props.botId },
      body: {
        phone: phoneInput.value,
        ...linkingOptions.value,
      },
      throwOnError: true,
    })
    if (data.pair_code) {
      pairCode.value = data.pair_code
      pollStatus.value = 'pair_code'
    }
  } catch (err) {
    toast.error(resolveApiErrorMessage(err, err instanceof Error ? err.message : String(err)))
  } finally {
    isRequestingPairCode.value = false
  }
}

function startPolling() {
  if (aborted) return
  pollOnce()
}

async function pollOnce() {
  if (aborted || qrState.value !== 'showing') return

  try {
    const { data } = await client.post<{ 200: WhatsAppQrResponse }, unknown, true>({
      url: '/bots/{bot_id}/channel/whatsapp/qr/poll',
      path: { bot_id: props.botId },
      body: {},
      throwOnError: true,
    })
    pollStatus.value = data.status ?? ''
    if (data.qr_code) {
      await setQrImage(data.qr_code)
    }
    if (data.pair_code) {
      pairCode.value = data.pair_code
    }

    switch (data.status) {
      case 'confirmed':
        qrState.value = 'success'
        toast.success(t('bots.channels.whatsappQr.success'))
        emit('loginSuccess')
        return
      case 'expired':
        return
      default:
        if (!aborted) {
          pollTimer = setTimeout(pollOnce, 1500)
        }
    }
  } catch {
    if (!aborted) {
      pollTimer = setTimeout(pollOnce, 3000)
    }
  }
}

function cancel() {
  aborted = true
  if (pollTimer) {
    clearTimeout(pollTimer)
    pollTimer = null
  }
  qrState.value = 'idle'
  qrImageDataUrl.value = ''
  pollStatus.value = ''
  pairCode.value = ''
  phoneInput.value = ''
  showPairForm.value = false
}

onUnmounted(() => {
  aborted = true
  if (pollTimer) {
    clearTimeout(pollTimer)
    pollTimer = null
  }
})
</script>
