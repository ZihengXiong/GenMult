<template>
  <main class="flex h-full min-w-0 overflow-hidden bg-background text-foreground">
    <aside class="flex w-13 shrink-0 flex-col items-center border-r border-border bg-sidebar py-2">
      <div class="mb-2 flex size-8 items-center justify-center rounded-md bg-primary text-primary-foreground">
        <Network class="size-4" />
      </div>

      <nav class="flex flex-1 flex-col items-center gap-1">
        <button
          v-for="item in activityItems"
          :key="item.id"
          type="button"
          class="relative flex size-9 items-center justify-center rounded-md transition-colors"
          :class="activeActivity === item.id
            ? 'bg-sidebar-accent text-sidebar-accent-foreground'
            : 'text-muted-foreground hover:bg-sidebar-accent/70 hover:text-foreground'"
          :title="item.label"
          :aria-label="item.label"
          @click="activeActivity = item.id"
        >
          <component
            :is="item.icon"
            class="size-4"
          />
          <span
            v-if="item.badge"
            class="absolute right-1 top-1 size-1.5 rounded-full bg-emerald-500"
          />
        </button>
      </nav>

      <RouterLink
        to="/settings"
        class="flex size-9 items-center justify-center rounded-md text-muted-foreground transition-colors hover:bg-sidebar-accent/70 hover:text-foreground"
        title="设置"
        aria-label="设置"
      >
        <Settings class="size-4" />
      </RouterLink>
    </aside>

    <section class="flex w-[292px] shrink-0 flex-col border-r border-border bg-sidebar">
      <header class="flex h-12 shrink-0 items-center gap-2 border-b border-border px-3 [-webkit-app-region:drag]">
        <div class="min-w-0 flex-1">
          <div class="flex items-center gap-2">
            <h1 class="truncate text-sm font-semibold">
              AgentHub
            </h1>
            <span class="rounded-sm bg-emerald-500/10 px-1.5 py-0.5 text-[10px] font-medium text-emerald-600 dark:text-emerald-400">
              Local
            </span>
          </div>
          <p class="truncate text-[11px] text-muted-foreground">
            面向 Agent 的协作客户端
          </p>
        </div>
        <Button
          variant="ghost"
          size="icon"
          class="size-7 [-webkit-app-region:no-drag]"
          title="新建 Agent"
          @click="router.push({ name: 'bot-new' })"
        >
          <SquarePen class="size-3.5" />
        </Button>
      </header>

      <div class="border-b border-border p-2">
        <div class="flex h-8 items-center gap-2 rounded-md border border-border bg-background px-2 text-muted-foreground">
          <Search class="size-3.5 shrink-0" />
          <input
            v-model="searchQuery"
            class="h-full min-w-0 flex-1 bg-transparent text-xs text-foreground outline-none placeholder:text-muted-foreground"
            placeholder="搜索群聊、Agent、任务"
          >
        </div>
      </div>

      <div class="min-h-0 flex-1 overflow-y-auto px-2 py-3">
        <div class="mb-2 flex items-center justify-between px-1">
          <p class="text-[11px] font-semibold uppercase tracking-[0.08em] text-muted-foreground">
            {{ activeActivityLabel }}
          </p>
          <Button
            v-if="activeActivity === 'rooms'"
            variant="ghost"
            size="icon"
            class="size-6 text-muted-foreground"
            title="新建群聊"
            @click="startCreateRoom"
          >
            <Plus class="size-3.5" />
          </Button>
          <Button
            v-else
            variant="ghost"
            size="icon"
            class="size-6 text-muted-foreground"
            title="刷新"
          >
            <RefreshCw class="size-3.5" />
          </Button>
        </div>

        <template v-if="activeActivity === 'rooms'">
          <form
            v-if="isCreatingRoom"
            class="mb-3 rounded-md border border-border bg-background p-2.5"
            @submit.prevent="createRoom"
          >
            <input
              v-model="newRoomName"
              class="mb-2 h-8 w-full rounded-md border border-border bg-card px-2 text-xs outline-none focus:border-primary"
              placeholder="群聊名称"
            >
            <select
              v-model="newRoomOrchestratorAgentId"
              class="mb-2 h-8 w-full rounded-md border border-border bg-card px-2 text-xs outline-none focus:border-primary"
            >
              <option value="">
                (选择群聊主 Agent)
              </option>
              <option
                v-for="agent in agents"
                :key="`select-agent-${agent.id}`"
                :value="agent.id"
              >
                {{ agent.name }} ({{ agent.kind }})
              </option>
            </select>
            <textarea
              v-model="newRoomSummary"
              class="min-h-18 w-full resize-none rounded-md border border-border bg-card px-2 py-1.5 text-xs leading-5 outline-none focus:border-primary"
              placeholder="这个群聊要协作什么"
            />
            <div class="mt-2 flex justify-end gap-1.5">
              <Button
                type="button"
                variant="ghost"
                size="sm"
                class="h-7 text-xs"
                @click="cancelCreateRoom"
              >
                取消
              </Button>
              <Button
                type="submit"
                size="sm"
                class="h-7 text-xs"
                :disabled="!newRoomName.trim()"
              >
                创建
              </Button>
            </div>
          </form>

          <div class="space-y-1.5">
            <button
              v-for="room in filteredRooms"
              :key="room.id"
              type="button"
              class="group w-full rounded-md border px-2.5 py-2 text-left transition-colors"
              :class="selectedRoomId === room.id
                ? 'border-primary/20 bg-background shadow-sm'
                : 'border-transparent hover:border-border hover:bg-background/80'"
              @click="selectedRoomId = room.id"
            >
              <div class="flex items-center gap-2">
                <span
                  class="flex size-7 shrink-0 items-center justify-center rounded-md text-xs font-semibold text-white"
                  :class="room.accent"
                >
                  {{ room.shortName }}
                </span>
                <span class="min-w-0 flex-1">
                  <span class="block truncate text-xs font-semibold text-foreground">{{ room.name }}</span>
                  <span class="block truncate text-[11px] text-muted-foreground">{{ room.subtitle }}</span>
                </span>
                <span
                  v-if="room.attention"
                  class="rounded-full bg-primary px-1.5 py-0.5 text-[10px] font-semibold text-primary-foreground"
                >
                  {{ room.attention }}
                </span>
              </div>
              <div class="mt-2 flex items-center gap-1.5 text-[11px] text-muted-foreground">
                <Users class="size-3" />
                <span>{{ room.members }} 人</span>
                <span>·</span>
                <Bot class="size-3" />
                <span>{{ roomAgentCount(room) }} Agents</span>
                <span>·</span>
                <span>{{ room.privacy }}</span>
              </div>
            </button>
          </div>
        </template>

        <template v-else-if="activeActivity === 'agents'">
          <div class="space-y-1.5">
            <button
              v-for="agent in filteredAgents"
              :key="agent.id"
              type="button"
              class="flex w-full items-center gap-2 rounded-md border border-transparent px-2.5 py-2 text-left transition-colors hover:border-border hover:bg-background"
              :class="selectedAgentId === agent.id ? 'border-border bg-background shadow-sm' : ''"
              @click="selectedAgentId = agent.id"
            >
              <span
                class="flex size-8 shrink-0 items-center justify-center rounded-md border"
                :class="agent.tone"
              >
                <component
                  :is="agent.icon"
                  class="size-4"
                />
              </span>
              <span class="min-w-0 flex-1">
                <span class="block truncate text-xs font-semibold">{{ agent.name }}</span>
                <span class="block truncate text-[11px] text-muted-foreground">{{ agent.kind }}</span>
              </span>
              <span
                class="size-1.5 shrink-0 rounded-full"
                :class="statusDotClass(agent.status)"
              />
            </button>
          </div>
        </template>

        <template v-else-if="activeActivity === 'tasks'">
          <div class="space-y-2">
            <article
              v-for="task in tasks"
              :key="task.id"
              class="rounded-md border border-border bg-background p-2.5"
            >
              <div class="flex items-start justify-between gap-2">
                <div class="min-w-0">
                  <p class="truncate text-xs font-semibold">
                    {{ task.title }}
                  </p>
                  <p class="mt-1 truncate text-[11px] text-muted-foreground">
                    {{ task.owner }} · {{ task.agent }}
                  </p>
                </div>
                <span
                  class="shrink-0 rounded-sm px-1.5 py-0.5 text-[10px] font-medium"
                  :class="taskBadgeClass(task.status)"
                >
                  {{ task.status }}
                </span>
              </div>
              <div class="mt-2 h-1.5 overflow-hidden rounded-full bg-muted">
                <div
                  class="h-full rounded-full"
                  :class="task.progressClass"
                  :style="{ width: `${task.progress}%` }"
                />
              </div>
            </article>
          </div>
        </template>

        <template v-else>
          <div class="space-y-2">
            <RouterLink
              v-for="entry in libraryCards"
              :key="entry.title"
              :to="entry.to"
              class="block rounded-md border border-border bg-background p-3 transition-colors hover:bg-accent"
            >
              <div class="flex items-center gap-2">
                <span
                  class="flex size-8 items-center justify-center rounded-md border"
                  :class="entry.tone"
                >
                  <component
                    :is="entry.icon"
                    class="size-4"
                  />
                </span>
                <span class="min-w-0">
                  <span class="block truncate text-xs font-semibold">{{ entry.title }}</span>
                  <span class="block truncate text-[11px] text-muted-foreground">{{ entry.caption }}</span>
                </span>
              </div>
            </RouterLink>
          </div>
        </template>
      </div>
    </section>

    <section class="flex min-w-0 flex-1 flex-col bg-background">
      <header class="flex h-11 shrink-0 items-center border-b border-border bg-card/60 [-webkit-app-region:drag]">
        <div class="flex h-full min-w-0 flex-1 items-center gap-1 overflow-x-auto px-2 [-webkit-app-region:no-drag]">
          <button
            v-for="room in rooms"
            :key="room.id"
            type="button"
            class="flex h-8 max-w-[190px] shrink-0 items-center gap-2 rounded-md border px-2.5 text-xs transition-colors"
            :class="selectedRoomId === room.id
              ? 'border-border bg-background text-foreground shadow-sm'
              : 'border-transparent text-muted-foreground hover:bg-background hover:text-foreground'"
            @click="selectedRoomId = room.id"
          >
            <span
              class="size-1.5 rounded-full"
              :class="room.statusClass"
            />
            <span class="truncate">{{ room.name }}</span>
            <X
              v-if="selectedRoomId === room.id"
              class="size-3 text-muted-foreground"
            />
          </button>
        </div>

        <div class="flex h-full shrink-0 items-center gap-1 border-l border-border px-2 [-webkit-app-region:no-drag]">
          <Button
            variant="ghost"
            size="icon"
            class="size-7"
            title="打开终端"
          >
            <TerminalSquare class="size-3.5" />
          </Button>
          <Button
            variant="ghost"
            size="icon"
            class="size-7"
            title="文件"
          >
            <FolderOpen class="size-3.5" />
          </Button>
        </div>
      </header>

      <div class="min-h-0 flex-1 overflow-y-auto">
        <div class="mx-auto flex min-h-full w-full max-w-4xl flex-col px-6 py-5">
          <section class="mb-5 rounded-md border border-border bg-card p-4 shadow-sm">
            <div class="flex items-start justify-between gap-4">
              <div class="min-w-0">
                <div class="flex flex-wrap items-center gap-2">
                  <h2 class="truncate text-base font-semibold">
                    {{ selectedRoom?.name }}
                  </h2>
                  <span class="rounded-sm border border-border bg-background px-1.5 py-0.5 text-[11px] text-muted-foreground">
                    {{ selectedRoom?.privacy }}
                  </span>
                  <span class="rounded-sm bg-emerald-500/10 px-1.5 py-0.5 text-[11px] text-emerald-600 dark:text-emerald-400">
                    {{ selectedRoom?.live }}
                  </span>
                </div>
                <p class="mt-1 text-sm leading-6 text-muted-foreground">
                  {{ selectedRoom?.summary }}
                </p>
              </div>
              <Button
                size="sm"
                class="h-8 shrink-0 gap-1.5 text-xs"
                :disabled="selectedRoomAgents.length === 0"
              >
                <MessageSquarePlus class="size-3.5" />
                发起任务
              </Button>
            </div>
          </section>

          <div class="flex flex-1 flex-col gap-3">
            <article
              v-for="event in timeline"
              :key="event.id"
              class="group flex gap-3"
            >
              <span
                class="mt-0.5 flex size-8 shrink-0 items-center justify-center rounded-md border"
                :class="event.tone"
              >
                <component
                  :is="event.icon"
                  class="size-4"
                />
              </span>
              <div class="min-w-0 flex-1 rounded-md border border-border bg-card p-3 shadow-sm">
                <div class="flex flex-wrap items-center gap-2">
                  <p class="text-sm font-semibold">
                    {{ event.title }}
                  </p>
                  <span class="text-[11px] text-muted-foreground">{{ event.time }}</span>
                  <span class="rounded-sm bg-muted px-1.5 py-0.5 text-[10px] font-medium text-muted-foreground">
                    {{ event.kind }}
                  </span>
                </div>
                <p class="mt-1 text-sm leading-6 text-muted-foreground">
                  {{ event.body }}
                </p>

                <!-- thinking: stored in metadata.thinking, not in body/history context -->
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

                <!-- tools: stored in metadata.tools, not in body/history context -->
                <details
                  v-if="event.tools?.length"
                  class="mt-2"
                >
                  <summary class="cursor-pointer select-none text-xs text-muted-foreground hover:text-foreground">
                    工具调用 ({{ event.tools.length }})
                  </summary>
                  <div class="mt-1 space-y-1">
                    <details
                      v-for="(tool, i) in event.tools"
                      :key="i"
                      class="rounded border border-border bg-muted/30 px-2 py-1"
                    >
                      <summary class="cursor-pointer select-none text-xs font-mono text-muted-foreground hover:text-foreground">
                        {{ tool.name }}
                      </summary>
                      <pre class="mt-1 overflow-x-auto whitespace-pre-wrap text-[11px] leading-4 text-muted-foreground/70">Input: {{ JSON.stringify(tool.input, null, 2) }}{{ tool.output !== undefined ? `\nOutput: ${JSON.stringify(tool.output, null, 2)}` : '' }}</pre>
                    </details>
                  </div>
                </details>

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
              </div>
            </article>

            <!-- thinking indicator -->
            <article
              v-if="isAgentReplying"
              class="flex gap-3 items-center text-sm text-muted-foreground animate-pulse"
            >
              <span class="flex size-8 shrink-0 items-center justify-center rounded-md border border-border bg-muted/50">
                <Spinner class="size-3.5" />
              </span>
              <div class="flex items-center gap-2">
                <span>Agent 正在思考中...</span>
              </div>
            </article>
          </div>

          <section class="sticky bottom-0 mt-5 bg-background/95 pb-4 pt-2 backdrop-blur">
            <div class="rounded-lg border border-border bg-card p-2 shadow-sm">
              <div class="flex flex-wrap gap-1.5 border-b border-border px-1 pb-2">
                <button
                  v-for="chip in composerChips"
                  :key="chip"
                  type="button"
                  class="rounded-sm border border-border bg-background px-2 py-1 text-[11px] text-muted-foreground transition-colors hover:bg-accent hover:text-foreground"
                >
                  {{ chip }}
                </button>
              </div>
              <textarea
                v-model="composerText"
                class="min-h-20 w-full resize-none bg-transparent px-2 py-2 text-sm leading-6 outline-none placeholder:text-muted-foreground"
                placeholder="@主 Agent 拆任务，或 @Codex / @Claude Code 进入并行执行"
                @keydown.enter.exact.prevent="sendRoomMessage"
              />
              <div class="flex items-center justify-between gap-3 px-1 pb-1">
                <div class="flex items-center gap-1 text-[11px] text-muted-foreground">
                  <AtSign class="size-3.5" />
                  <span>@ 文件、@@ Agent、@@ all(config)</span>
                </div>
                <Button
                  size="sm"
                  class="h-8 gap-1.5 text-xs"
                  :disabled="!canSendRoomMessage"
                  @click="sendRoomMessage"
                >
                  <SendHorizontal class="size-3.5" />
                  发送
                </Button>
              </div>
            </div>
          </section>
        </div>
      </div>
    </section>

    <aside class="flex w-[324px] shrink-0 flex-col border-l border-border bg-sidebar/70">
      <section class="border-b border-border p-4">
        <div class="flex items-start gap-3">
          <span
            class="flex size-10 shrink-0 items-center justify-center rounded-md border"
            :class="selectedAgent?.tone"
          >
            <component
              :is="selectedAgent?.icon"
              class="size-5"
            />
          </span>
          <div class="min-w-0 flex-1">
            <p class="truncate text-sm font-semibold">
              {{ selectedAgent?.name }}
            </p>
            <p class="truncate text-xs text-muted-foreground">
              {{ selectedAgent?.kind }}
            </p>
            <div class="mt-2 flex flex-wrap gap-1">
              <span
                v-for="capability in selectedAgent?.capabilities"
                :key="capability"
                class="rounded-sm border border-border bg-background px-1.5 py-0.5 text-[11px] text-muted-foreground"
              >
                {{ capability }}
              </span>
            </div>
          </div>
        </div>

        <div class="mt-4 grid grid-cols-2 gap-2">
          <Button
            variant="outline"
            size="sm"
            class="h-8 gap-1.5 text-xs"
            :disabled="!selectedAgent?.botId"
            @click="openSelectedBot"
          >
            <MessageSquare class="size-3.5" />
            对话
          </Button>
          <Button
            size="sm"
            class="h-8 gap-1.5 text-xs"
            :variant="isSelectedAgentInRoom ? 'outline' : 'default'"
            :disabled="!selectedAgent"
            @click="toggleSelectedAgentInRoom"
          >
            <UserMinus
              v-if="isSelectedAgentInRoom"
              class="size-3.5"
            />
            <UserPlus
              v-else
              class="size-3.5"
            />
            {{ isSelectedAgentInRoom ? '移出群' : '加入群' }}
          </Button>
        </div>
      </section>

      <section class="min-h-0 flex-1 overflow-y-auto p-4">
        <div class="mb-5 rounded-md border border-border bg-card p-3">
          <div class="mb-3 flex items-center justify-between">
            <p class="text-xs font-semibold">
              当前群成员
            </p>
            <span class="text-[11px] text-muted-foreground">{{ selectedRoomAgents.length }} Agents</span>
          </div>
          <div
            v-if="selectedRoomAgents.length"
            class="space-y-1.5"
          >
            <div
              v-for="agent in selectedRoomAgents"
              :key="`room-agent-${agent.id}`"
              class="group/item flex items-center gap-2 rounded-md border px-2 py-1.5 transition-colors cursor-pointer"
              :class="selectedAgentId === agent.id ? 'border-primary/25 bg-primary/5' : 'border-border bg-background'"
              @click="selectedAgentId = agent.id"
            >
              <span
                class="flex size-6 shrink-0 items-center justify-center rounded-md border"
                :class="agent.tone"
              >
                <component
                  :is="agent.icon"
                  class="size-3.5"
                />
              </span>
              <span class="min-w-0 flex-1 truncate text-xs">{{ agent.name }}</span>
              
              <!-- crown for current main agent -->
              <span
                v-if="selectedRoom?.orchestratorAgentId === agent.id"
                class="flex shrink-0 items-center gap-0.5 rounded bg-emerald-500/10 px-1 py-0.5 text-[9px] font-medium text-emerald-600 dark:text-emerald-400"
                title="当前是主 Agent"
              >
                <Crown class="size-2.5" />
                主
              </span>
              <!-- click to designate as main agent -->
              <button
                v-else
                type="button"
                class="opacity-0 group-hover/item:opacity-100 flex shrink-0 items-center gap-0.5 rounded bg-muted hover:bg-emerald-500/10 hover:text-emerald-600 px-1 py-0.5 text-[9px] font-medium text-muted-foreground transition-all"
                title="设为主 Agent"
                @click.stop="setRoomOrchestrator(selectedRoom, agent.id)"
              >
                设为主
              </button>

              <button
                type="button"
                class="flex size-6 items-center justify-center rounded-md text-muted-foreground transition-colors hover:bg-muted hover:text-foreground"
                title="移出群聊"
                @click.stop="removeAgentFromSelectedRoom(agent.id)"
              >
                <X class="size-3.5" />
              </button>
            </div>
          </div>
          <p
            v-else
            class="text-xs leading-5 text-muted-foreground"
          >
            这个群还没有 Agent。先从左侧 Agents 列表选择一个，然后点“加入群”。
          </p>
        </div>

        <div class="mb-2 flex items-center justify-between">
          <h3 class="text-xs font-semibold uppercase tracking-[0.08em] text-muted-foreground">
            Agent 接入
          </h3>
          <span class="text-[11px] text-muted-foreground">4 adapters</span>
        </div>

        <div class="space-y-2">
          <button
            v-for="connector in connectors"
            :key="connector.name"
            type="button"
            class="w-full rounded-md border border-border bg-card p-3 text-left transition-colors hover:bg-accent disabled:cursor-not-allowed disabled:opacity-60"
            :disabled="!connector.enabled"
            @click="handleConnectorClick(connector)"
          >
            <div class="flex items-start gap-2">
              <span
                class="flex size-8 shrink-0 items-center justify-center rounded-md border"
                :class="connector.tone"
              >
                <component
                  :is="connector.icon"
                  class="size-4"
                />
              </span>
              <div class="min-w-0 flex-1">
                <div class="flex items-center justify-between gap-2">
                  <p class="truncate text-xs font-semibold">
                    {{ connector.name }}
                  </p>
                  <span
                    class="shrink-0 rounded-sm px-1.5 py-0.5 text-[10px] font-medium"
                    :class="connector.enabled
                      ? 'bg-emerald-500/10 text-emerald-600 dark:text-emerald-400'
                      : 'bg-muted text-muted-foreground'"
                  >
                    {{ connector.enabled ? '在线' : '待接入' }}
                  </span>
                </div>
                <p class="mt-1 text-[11px] leading-4 text-muted-foreground">
                  {{ connector.description }}
                </p>
              </div>
            </div>
          </button>
        </div>

        <div class="mt-5 rounded-md border border-border bg-card p-3">
          <div class="flex items-center gap-2">
            <ShieldCheck class="size-4 text-emerald-600 dark:text-emerald-400" />
            <p class="text-xs font-semibold">
              隐私边界
            </p>
          </div>
          <p class="mt-2 text-xs leading-5 text-muted-foreground">
            群内共享上下文；跨群只召回脱敏经验。用户、金额、仓库和任务细节默认按群权限隔离。
          </p>
        </div>

        <div class="mt-5 rounded-md border border-border bg-card p-3">
          <div class="mb-3 flex items-center justify-between">
            <p class="text-xs font-semibold">
              当前队列
            </p>
            <span class="text-[11px] text-muted-foreground">并行 2 / 排队 1</span>
          </div>
          <div class="space-y-2">
            <div
              v-for="task in tasks.slice(0, 3)"
              :key="`side-${task.id}`"
              class="flex items-center gap-2"
            >
              <span
                class="size-1.5 rounded-full"
                :class="task.progressClass"
              />
              <span class="min-w-0 flex-1 truncate text-xs">{{ task.title }}</span>
              <span class="text-[11px] text-muted-foreground">{{ task.progress }}%</span>
            </div>
          </div>
        </div>
      </section>
    </aside>
  </main>
</template>

<script setup lang="ts">
import { computed, ref, watch, type Component } from 'vue'
import { RouterLink, useRouter } from 'vue-router'
import { useMutation, useQuery, useQueryCache } from '@pinia/colada'
import { getBotsQuery } from '@memohai/sdk/colada'
import type { BotsBot } from '@memohai/sdk'
import { client } from '@memohai/sdk/client'
import { Button, Spinner } from '@memohai/ui'
import { connectWebSocket, createSession, type UIMessage, type UIStreamEvent, type UIToolMessage } from '@/composables/api/useChat'
import { visibleBots } from '@/utils/bots'
import {
  AlertCircle,
  AtSign,
  Bot,
  Boxes,
  BrainCircuit,
  Code2,
  FolderOpen,
  Library,
  ListChecks,
  MessageSquare,
  MessageSquarePlus,
  Network,
  Plug,
  Plus,
  RefreshCw,
  Search,
  SendHorizontal,
  Settings,
  ShieldCheck,
  Sparkles,
  SquarePen,
  TerminalSquare,
  UserPlus,
  UserMinus,
  Users,
  Workflow,
  Wrench,
  X,
  Crown,
} from 'lucide-vue-next'

type ActivityId = 'rooms' | 'agents' | 'tasks' | 'skills' | 'mcp'

interface RoomItem {
  id: string
  name: string
  shortName: string
  subtitle: string
  summary: string
  members: number
  attention: number
  privacy: string
  live: string
  accent: string
  statusClass: string
  agentIds: string[]
  orchestratorAgentId?: string
  metadata?: Record<string, unknown>
}

interface AgentHubRoom {
  id: string
  name: string
  short_name: string
  subtitle: string
  summary: string
  members: number
  attention: number
  privacy: string
  live: string
  accent: string
  status_class: string
  agent_ids: string[]
  orchestrator_agent_id?: string
  metadata?: Record<string, unknown>
}

interface AgentHubRoomList {
  items: AgentHubRoom[]
}

interface AgentHubMessage {
  id: string
  room_id: string
  sender_id: string
  sender_type: 'user' | 'agent' | 'system' | string
  sender_name: string
  kind: string
  title: string
  body: string
  metadata?: Record<string, unknown>
  created_at: string
}

interface AgentHubMessageList {
  items: AgentHubMessage[]
}

interface CreateAgentHubMessagePayload {
  sender_id?: string
  sender_type?: string
  sender_name?: string
  kind?: string
  title?: string
  body: string
  metadata?: Record<string, unknown>
}

interface AgentItem {
  id: string
  name: string
  kind: string
  status: 'online' | 'busy' | 'draft'
  icon: Component
  tone: string
  capabilities: string[]
  botId?: string
}

interface StoredTool {
  name: string
  input: unknown
  output?: unknown
}

interface TimelineEvent {
  id: string
  time: string
  kind: string
  title: string
  body: string
  thinking?: string
  tools?: StoredTool[]
  icon: Component
  tone: string
  actions?: string[]
}

interface ConnectorItem {
  id: 'memoh-bot' | 'codex-bridge' | 'claude-bridge' | 'custom-agent'
  name: string
  description: string
  enabled: boolean
  icon: Component
  tone: string
}

const router = useRouter()
const activeActivity = ref<ActivityId>('rooms')
const selectedRoomId = ref('payment')
const selectedAgentId = ref('orchestrator')
const searchQuery = ref('')
const isCreatingRoom = ref(false)
const newRoomName = ref('')
const newRoomSummary = ref('')
const newRoomOrchestratorAgentId = ref('')
const composerText = ref('')
const ROOM_STORAGE_KEY = 'memoh.agenthub.rooms.v1'
const ROOM_MIGRATION_KEY = 'memoh.agenthub.rooms.migrated.v1'
const AGENT_HUB_ROOMS_KEY = ['agent-hub', 'rooms']

const activityItems: Array<{ id: ActivityId; label: string; icon: Component; badge?: boolean }> = [
  { id: 'rooms', label: '群聊', icon: MessageSquare, badge: true },
  { id: 'agents', label: 'Agents', icon: Bot },
  { id: 'tasks', label: '任务', icon: ListChecks, badge: true },
  { id: 'skills', label: 'Skills 库', icon: Library },
  { id: 'mcp', label: 'MCP 库', icon: Plug },
]

const defaultRooms: RoomItem[] = [
  {
    id: 'payment',
    name: '电商支付模块',
    shortName: '支',
    subtitle: '后端、前端、测试并行推进',
    summary: '这是一个混合团队群：PM、开发、测试和多个 Agent 在同一个任务上下文里协作，任务卡、代码审查、记忆召回都会留在群内。',
    members: 7,
    attention: 4,
    privacy: '项目群',
    live: 'Codex 执行中',
    accent: 'bg-emerald-600',
    statusClass: 'bg-emerald-500',
    agentIds: ['orchestrator', 'codex', 'claude-code', 'test-agent'],
  },
  {
    id: 'mobile',
    name: '移动端体验',
    shortName: '移',
    subtitle: '通知、生物认证、移动适配',
    summary: '面向手机端的 Agent 交互，重点是关键通知、移动端可读性和跨设备继续处理。',
    members: 5,
    attention: 0,
    privacy: '私有群',
    live: '等待输入',
    accent: 'bg-blue-600',
    statusClass: 'bg-blue-500',
    agentIds: ['orchestrator', 'claude-code'],
  },
  {
    id: 'ops',
    name: '上线与运维',
    shortName: '运',
    subtitle: 'CI/CD、远程终端、发布检查',
    summary: '部署 Agent、Codex 和人工 reviewer 共同处理上线队列，所有高风险操作需要显式审批。',
    members: 6,
    attention: 2,
    privacy: '受控群',
    live: '审查中',
    accent: 'bg-amber-600',
    statusClass: 'bg-amber-500',
    agentIds: ['orchestrator', 'codex'],
  },
]

const baseAgents: AgentItem[] = [
  {
    id: 'orchestrator',
    name: '主 Agent',
    kind: 'Orchestrator',
    status: 'online',
    icon: Workflow,
    tone: 'border-emerald-500/30 bg-emerald-500/10 text-emerald-700 dark:text-emerald-300',
    capabilities: ['任务拆解', '群聊调度', '记忆召回'],
  },
  {
    id: 'codex',
    name: 'Codex',
    kind: 'CLI Agent',
    status: 'busy',
    icon: TerminalSquare,
    tone: 'border-blue-500/30 bg-blue-500/10 text-blue-700 dark:text-blue-300',
    capabilities: ['改代码', '跑测试', '开子任务'],
  },
  {
    id: 'claude-code',
    name: 'Claude Code',
    kind: 'CLI Agent',
    status: 'online',
    icon: Code2,
    tone: 'border-violet-500/30 bg-violet-500/10 text-violet-700 dark:text-violet-300',
    capabilities: ['代码审查', '方案讨论', '仓库分析'],
  },
  {
    id: 'test-agent',
    name: '测试 Agent',
    kind: 'Custom Agent',
    status: 'draft',
    icon: ListChecks,
    tone: 'border-amber-500/30 bg-amber-500/10 text-amber-700 dark:text-amber-300',
    capabilities: ['用例生成', '覆盖率检查', '回归提醒'],
  },
]

function cloneDefaultRooms(): RoomItem[] {
  return defaultRooms.map((room) => ({
    ...room,
    agentIds: [...room.agentIds],
  }))
}

function isRoomItem(value: unknown): value is RoomItem {
  if (!value || typeof value !== 'object') return false
  const room = value as Partial<RoomItem>
  return Boolean(
    room.id
    && room.name
    && room.shortName
    && Array.isArray(room.agentIds),
  )
}

function loadRooms(): RoomItem[] {
  if (typeof window === 'undefined') return cloneDefaultRooms()

  try {
    const raw = window.localStorage.getItem(ROOM_STORAGE_KEY)
    if (!raw) return cloneDefaultRooms()
    const parsed = JSON.parse(raw) as unknown
    if (!Array.isArray(parsed)) return cloneDefaultRooms()
    const loaded = parsed.filter(isRoomItem).map((room) => ({
      ...room,
      agentIds: [...new Set(room.agentIds)],
    }))
    return loaded.length ? loaded : cloneDefaultRooms()
  } catch {
    return cloneDefaultRooms()
  }
}

const localRooms = loadRooms()
const rooms = ref<RoomItem[]>(localRooms)
const hasMigratedRooms = ref(
  typeof window !== 'undefined' && window.localStorage.getItem(ROOM_MIGRATION_KEY) === 'true',
)
const isMigratingRooms = ref(false)
const isAgentReplying = ref(false)
const joiningMainAgentKey = ref('')
const queryCache = useQueryCache()

const { data: botData } = useQuery(getBotsQuery())
const { data: roomData } = useQuery({
  key: AGENT_HUB_ROOMS_KEY,
  query: listAgentHubRooms,
})

const { mutateAsync: createRoomMutation } = useMutation({
  mutation: createAgentHubRoom,
  onSettled: () => queryCache.invalidateQueries({ key: AGENT_HUB_ROOMS_KEY }),
})

const { mutateAsync: addAgentMutation } = useMutation({
  mutation: ({ roomId, agentId }: { roomId: string, agentId: string }) =>
    addAgentHubRoomAgent(roomId, agentId),
  onSettled: () => {
    queryCache.invalidateQueries({ key: AGENT_HUB_ROOMS_KEY })
    if (selectedRoom.value?.id) {
      queryCache.invalidateQueries({ key: ['agent-hub', 'messages', selectedRoom.value.id] })
    }
  },
})

const { mutateAsync: removeAgentMutation } = useMutation({
  mutation: ({ roomId, agentId }: { roomId: string, agentId: string }) =>
    removeAgentHubRoomAgent(roomId, agentId),
  onSettled: () => {
    queryCache.invalidateQueries({ key: AGENT_HUB_ROOMS_KEY })
    if (selectedRoom.value?.id) {
      queryCache.invalidateQueries({ key: ['agent-hub', 'messages', selectedRoom.value.id] })
    }
  },
})

watch(
  roomData,
  (data) => {
    if (!data) return
    const apiRooms = data.items.map(agentHubRoomToItem)
    if (apiRooms.length) {
      rooms.value = apiRooms
      hasMigratedRooms.value = true
      if (typeof window !== 'undefined') {
        window.localStorage.setItem(ROOM_MIGRATION_KEY, 'true')
      }
      ensureSelectedRoom()
      return
    }
    void migrateLocalRooms()
  },
  { immediate: true },
)

const memohAgents = computed<AgentItem[]>(() =>
  visibleBots(botData.value?.items ?? [])
    .map((bot: BotsBot) => {
      const name = bot.display_name || 'Memoh Bot'
      return {
        id: `bot:${bot.id ?? bot.display_name}`,
        name,
        kind: bot.framework === 'claudecode'
          ? 'Claude Code Bot'
          : bot.framework === 'codex'
          ? 'Codex Bot'
          : isPeppaAgentName(name) ? '主 Agent · Memoh Bot' : 'Memoh Bot',
        status: bot.status === 'creating' || bot.status === 'deleting' ? 'busy' : 'online',
        icon: Bot,
        tone: isPeppaAgentName(name)
          ? 'border-emerald-500/30 bg-emerald-500/10 text-emerald-700 dark:text-emerald-300'
          : 'border-slate-500/30 bg-slate-500/10 text-slate-700 dark:text-slate-300',
        capabilities: ['本地聊天', '容器工具', '长期记忆'],
        botId: bot.id,
      }
    }),
)

const agents = computed(() => [...baseAgents, ...memohAgents.value])
const mainAgent = computed(() => {
  const room = selectedRoom.value
  if (room && room.orchestratorAgentId) {
    const found = agents.value.find((agent) => agent.id === room.orchestratorAgentId)
    if (found) return found
  }
  return memohAgents.value.find((agent) => isPeppaAgentName(agent.name))
    ?? memohAgents.value[0]
    ?? baseAgents[0]
})

const selectedRoom = computed(() =>
  rooms.value.find((room) => room.id === selectedRoomId.value) ?? rooms.value[0],
)
const canSendRoomMessage = computed(() =>
  Boolean(composerText.value.trim() && isPersistedRoomId(selectedRoom.value?.id) && !isAgentReplying.value),
)

const { data: messageData } = useQuery({
  key: () => ['agent-hub', 'messages', selectedRoom.value?.id ?? 'none'],
  query: () => listAgentHubRoomMessages(selectedRoom.value?.id ?? ''),
  enabled: () => isPersistedRoomId(selectedRoom.value?.id),
})

const { mutateAsync: createMessageMutation } = useMutation({
  mutation: ({ roomId, payload }: { roomId: string, payload: CreateAgentHubMessagePayload }) =>
    createAgentHubRoomMessage(roomId, payload),
  onSettled: () => {
    if (!selectedRoom.value?.id) return
    queryCache.invalidateQueries({ key: ['agent-hub', 'messages', selectedRoom.value.id] })
  },
})

const selectedAgent = computed(() =>
  agents.value.find((agent) => agent.id === selectedAgentId.value) ?? agents.value[0],
)

const selectedRoomAgents = computed(() => {
  const ids = new Set(selectedRoom.value?.agentIds ?? [])
  return agents.value.filter((agent) => ids.has(agent.id))
})

const selectedRoomAgentNames = computed(() =>
  selectedRoomAgents.value.map((agent) => agent.name).join('、') || '暂未加入 Agent',
)

const isSelectedAgentInRoom = computed(() =>
  Boolean(selectedAgent.value && selectedRoom.value?.agentIds.includes(selectedAgent.value.id)),
)

watch(
  mainAgent,
  (agent) => {
    if (!agent) return
    if (selectedAgentId.value === 'orchestrator' || !agents.value.some((item) => item.id === selectedAgentId.value)) {
      selectedAgentId.value = agent.id
    }
    void ensureMainAgentInSelectedRoom()
  },
  { immediate: true },
)

watch(
  selectedRoom,
  () => {
    void ensureMainAgentInSelectedRoom()
  },
)

const activeActivityLabel = computed(() =>
  activityItems.find((item) => item.id === activeActivity.value)?.label ?? 'AgentHub',
)

const normalizedSearch = computed(() => searchQuery.value.trim().toLowerCase())

const filteredRooms = computed(() => {
  const q = normalizedSearch.value
  if (!q) return rooms.value
  return rooms.value.filter((room) =>
    `${room.name} ${room.subtitle} ${room.privacy}`.toLowerCase().includes(q),
  )
})

const filteredAgents = computed(() => {
  const q = normalizedSearch.value
  if (!q) return agents.value
  return agents.value.filter((agent) =>
    `${agent.name} ${agent.kind} ${agent.capabilities.join(' ')}`.toLowerCase().includes(q),
  )
})

const timeline = computed<TimelineEvent[]>(() => {
  const messages = messageData.value?.items ?? []
  if (messages.length) {
    return messages.map(messageToTimelineEvent)
  }
  return [
    {
      id: 'room-ready',
      time: '刚刚',
      kind: '群聊',
      title: `${selectedRoom.value?.name ?? '新群聊'} 已准备就绪`,
      body: `当前群内 Agent：${selectedRoomAgentNames.value}。群内上下文、任务卡和后续执行结果会在这里汇总。`,
      icon: MessageSquare,
      tone: 'border-emerald-500/30 bg-emerald-500/10 text-emerald-700 dark:text-emerald-300',
      actions: selectedRoomAgents.value.length ? ['发起任务', '查看成员'] : ['添加 Agent'],
    },
  ]
})

const tasks = [
  {
    id: 'payment-api',
    title: '微信支付 API 对接',
    owner: '李四',
    agent: 'Codex',
    status: '执行中',
    progress: 80,
    progressClass: 'bg-emerald-500',
  },
  {
    id: 'payment-page',
    title: '支付页与状态回显',
    owner: '张三',
    agent: 'Claude Code',
    status: '排队',
    progress: 35,
    progressClass: 'bg-blue-500',
  },
  {
    id: 'regression',
    title: '回归测试矩阵',
    owner: '王五',
    agent: '测试 Agent',
    status: '待审查',
    progress: 58,
    progressClass: 'bg-amber-500',
  },
]

const connectors: ConnectorItem[] = [
  {
    id: 'memoh-bot',
    name: 'Memoh Client Bot',
    description: '接入自有客户端消息、通知、文件和任务卡。',
    enabled: true,
    icon: Boxes,
    tone: 'border-emerald-500/30 bg-emerald-500/10 text-emerald-700 dark:text-emerald-300',
  },
  {
    id: 'codex-bridge',
    name: 'Codex Agent Bridge',
    description: 'CLI/SDK 接入，保留工具追踪、流式输出和会话恢复。',
    enabled: false,
    icon: TerminalSquare,
    tone: 'border-blue-500/30 bg-blue-500/10 text-blue-700 dark:text-blue-300',
  },
  {
    id: 'claude-bridge',
    name: 'Claude Code Bridge',
    description: '以可 @ 的 Agent 接入，不走单纯模型配置。',
    enabled: false,
    icon: Code2,
    tone: 'border-violet-500/30 bg-violet-500/10 text-violet-700 dark:text-violet-300',
  },
  {
    id: 'custom-agent',
    name: '自建 Agent 模板',
    description: '绑定 skills、MCP、容器和权限策略后加入群聊。',
    enabled: true,
    icon: Wrench,
    tone: 'border-amber-500/30 bg-amber-500/10 text-amber-700 dark:text-amber-300',
  },
]

const libraryCards = [
  {
    title: 'Skills 库',
    caption: '会议纪要、制表、审查、发布检查',
    to: '/settings/supermarket',
    icon: Sparkles,
    tone: 'border-amber-500/30 bg-amber-500/10 text-amber-700 dark:text-amber-300',
  },
  {
    title: 'MCP 库',
    caption: 'GitHub、文档、搜索和内部系统',
    to: '/settings/supermarket',
    icon: Plug,
    tone: 'border-blue-500/30 bg-blue-500/10 text-blue-700 dark:text-blue-300',
  },
]

const composerChips = computed(() => [
  ...selectedRoomAgents.value.map((agent) => `@${agent.name}`),
  '@@all(config)',
])

async function listAgentHubRooms(): Promise<AgentHubRoomList> {
  const { data } = await client.request<{ 200: AgentHubRoomList }, unknown, true>({
    method: 'GET',
    url: '/agent-hub/rooms',
    throwOnError: true,
  })
  return data
}

async function listAgentHubRoomMessages(roomId: string): Promise<AgentHubMessageList> {
  if (!isPersistedRoomId(roomId)) return { items: [] }
  const { data } = await client.request<{ 200: AgentHubMessageList }, unknown, true>({
    method: 'GET',
    url: '/agent-hub/rooms/{room_id}/messages',
    path: { room_id: roomId },
    query: { limit: 200 },
    throwOnError: true,
  })
  return data
}

async function createAgentHubRoom(room: RoomItem): Promise<AgentHubRoom> {
  const { data } = await client.request<{ 201: AgentHubRoom }, unknown, true>({
    method: 'POST',
    url: '/agent-hub/rooms',
    body: roomItemToPayload(room),
    headers: { 'Content-Type': 'application/json' },
    throwOnError: true,
  })
  return data
}

async function createAgentHubRoomMessage(
  roomId: string,
  payload: CreateAgentHubMessagePayload,
): Promise<AgentHubMessage> {
  const { data } = await client.request<{ 201: AgentHubMessage }, unknown, true>({
    method: 'POST',
    url: '/agent-hub/rooms/{room_id}/messages',
    path: { room_id: roomId },
    body: payload,
    headers: { 'Content-Type': 'application/json' },
    throwOnError: true,
  })
  return data
}

async function addAgentHubRoomAgent(roomId: string, agentId: string): Promise<AgentHubRoom> {
  const { data } = await client.request<{ 200: AgentHubRoom }, unknown, true>({
    method: 'POST',
    url: '/agent-hub/rooms/{room_id}/agents',
    path: { room_id: roomId },
    body: { agent_id: agentId },
    headers: { 'Content-Type': 'application/json' },
    throwOnError: true,
  })
  return data
}

async function removeAgentHubRoomAgent(roomId: string, agentId: string): Promise<AgentHubRoom> {
  const { data } = await client.request<{ 200: AgentHubRoom }, unknown, true>({
    method: 'DELETE',
    url: '/agent-hub/rooms/{room_id}/agents/{agent_id}',
    path: { room_id: roomId, agent_id: agentId },
    throwOnError: true,
  })
  return data
}

async function migrateLocalRooms() {
  if (isMigratingRooms.value) return
  isMigratingRooms.value = true
  const created: RoomItem[] = []
  const sourceRooms = localRooms.length ? localRooms : cloneDefaultRooms()
  for (const room of sourceRooms) {
    try {
      const result = await createRoomMutation(room)
      created.push(agentHubRoomToItem(result))
    }
    catch (error) {
      console.error('Failed to migrate AgentHub room:', error)
    }
  }
  if (created.length) {
    rooms.value = created
    selectedRoomId.value = created[0].id
    hasMigratedRooms.value = true
    if (typeof window !== 'undefined') {
      window.localStorage.setItem(ROOM_MIGRATION_KEY, 'true')
    }
  }
  else {
    hasMigratedRooms.value = false
    if (typeof window !== 'undefined') {
      window.localStorage.removeItem(ROOM_MIGRATION_KEY)
    }
  }
  isMigratingRooms.value = false
}

function agentHubRoomToItem(room: AgentHubRoom): RoomItem {
  return {
    id: room.id,
    name: room.name,
    shortName: room.short_name,
    subtitle: room.subtitle,
    summary: room.summary,
    members: room.members,
    attention: room.attention,
    privacy: room.privacy,
    live: room.live,
    accent: room.accent,
    statusClass: room.status_class,
    agentIds: [...new Set(room.agent_ids)],
    orchestratorAgentId: room.orchestrator_agent_id || '',
    metadata: room.metadata ?? {},
  }
}

function roomItemToPayload(room: RoomItem) {
  return {
    name: room.name,
    short_name: room.shortName,
    subtitle: room.subtitle,
    summary: room.summary,
    members: room.members,
    attention: room.attention,
    privacy: room.privacy,
    live: room.live,
    accent: room.accent,
    status_class: room.statusClass,
    agent_ids: room.agentIds,
    orchestrator_agent_id: room.orchestratorAgentId || '',
    metadata: room.metadata ?? {},
  }
}

function replaceRoom(updated: AgentHubRoom) {
  const nextRoom = agentHubRoomToItem(updated)
  rooms.value = rooms.value.map((room) => room.id === nextRoom.id ? nextRoom : room)
  selectedRoomId.value = nextRoom.id
}

function ensureSelectedRoom() {
  if (!rooms.value.length) {
    selectedRoomId.value = ''
    return
  }
  if (!rooms.value.some((room) => room.id === selectedRoomId.value)) {
    selectedRoomId.value = rooms.value[0].id
  }
}

function isPersistedRoomId(value?: string) {
  return Boolean(value && /^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/i.test(value))
}

function messageToTimelineEvent(message: AgentHubMessage): TimelineEvent {
  const icon = messageIcon(message)
  const thinking = typeof message.metadata?.thinking === 'string' && message.metadata.thinking.trim()
    ? message.metadata.thinking
    : undefined
  const rawTools = message.metadata?.tools
  const tools = Array.isArray(rawTools) && rawTools.length > 0
    ? (rawTools as StoredTool[])
    : undefined
  return {
    id: message.id,
    time: formatMessageTime(message.created_at),
    kind: messageKindLabel(message.kind),
    title: message.title || message.sender_name || 'AgentHub',
    body: message.body,
    thinking,
    tools,
    icon,
    tone: messageTone(message),
  }
}

function messageIcon(message: AgentHubMessage): Component {
  if (message.kind === 'error') return AlertCircle
  if (message.kind === 'member') return Users
  if (message.kind === 'room') return Network
  if (message.kind === 'task') return Workflow
  if (message.kind === 'execution') return TerminalSquare
  if (message.kind === 'memory') return BrainCircuit
  if (message.sender_type === 'agent') {
    if (message.sender_id === 'codex') return TerminalSquare
    if (message.sender_id === 'claude-code') return Code2
    return Bot
  }
  return MessageSquare
}

function messageTone(message: AgentHubMessage) {
  if (message.kind === 'error') {
    return 'border-red-500/30 bg-red-500/10 text-red-700 dark:text-red-300'
  }
  if (message.sender_id === 'codex' || message.kind === 'execution') {
    return 'border-blue-500/30 bg-blue-500/10 text-blue-700 dark:text-blue-300'
  }
  if (message.sender_id === 'claude-code') {
    return 'border-violet-500/30 bg-violet-500/10 text-violet-700 dark:text-violet-300'
  }
  if (message.kind === 'member') {
    return 'border-amber-500/30 bg-amber-500/10 text-amber-700 dark:text-amber-300'
  }
  if (message.sender_type === 'system' || message.kind === 'room') {
    return 'border-emerald-500/30 bg-emerald-500/10 text-emerald-700 dark:text-emerald-300'
  }
  if (message.sender_type === 'agent') {
    return 'border-slate-500/30 bg-slate-500/10 text-slate-700 dark:text-slate-300'
  }
  return 'border-primary/25 bg-primary/5 text-primary'
}

function messageKindLabel(kind: string) {
  switch (kind) {
    case 'error':
      return '错误'
    case 'room':
      return '群聊'
    case 'member':
      return '成员'
    case 'task':
      return '任务卡'
    case 'execution':
      return '执行'
    case 'memory':
      return '记忆'
    default:
      return '消息'
  }
}

function formatMessageTime(value: string) {
  const date = new Date(value)
  if (Number.isNaN(date.getTime())) return ''
  return date.toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' })
}

function statusDotClass(status?: AgentItem['status']) {
  switch (status) {
    case 'online':
      return 'bg-emerald-500'
    case 'busy':
      return 'bg-amber-500'
    case 'draft':
      return 'bg-muted-foreground'
    default:
      return 'bg-muted-foreground'
  }
}

function taskBadgeClass(status: string) {
  switch (status) {
    case '执行中':
      return 'bg-emerald-500/10 text-emerald-600 dark:text-emerald-400'
    case '排队':
      return 'bg-blue-500/10 text-blue-600 dark:text-blue-400'
    case '待审查':
      return 'bg-amber-500/10 text-amber-600 dark:text-amber-400'
    default:
      return 'bg-muted text-muted-foreground'
  }
}

function roomAgentCount(room: RoomItem) {
  const knownIds = new Set(agents.value.map((agent) => agent.id))
  return room.agentIds.filter((agentId) => knownIds.has(agentId)).length
}

function roomShortName(name: string) {
  const trimmed = name.trim()
  return trimmed.slice(0, 1).toUpperCase() || '群'
}

function isPeppaAgentName(name: string) {
  return /佩奇|peppa/i.test(name.trim())
}

function startCreateRoom() {
  activeActivity.value = 'rooms'
  isCreatingRoom.value = true
}

function cancelCreateRoom() {
  isCreatingRoom.value = false
  newRoomName.value = ''
  newRoomSummary.value = ''
  newRoomOrchestratorAgentId.value = ''
}

const { mutateAsync: updateRoomMutation } = useMutation({
  mutation: (room: RoomItem) =>
    updateAgentHubRoom(room),
  onSettled: () => queryCache.invalidateQueries({ key: AGENT_HUB_ROOMS_KEY }),
})

async function updateAgentHubRoom(room: RoomItem): Promise<AgentHubRoom> {
  const { data } = await client.request<{ 200: AgentHubRoom }, unknown, true>({
    method: 'PUT',
    url: '/agent-hub/rooms/{room_id}',
    path: { room_id: room.id },
    body: roomItemToPayload(room),
    headers: { 'Content-Type': 'application/json' },
    throwOnError: true,
  })
  return data
}

async function setRoomOrchestrator(room: RoomItem | undefined, agentId: string) {
  if (!room) return
  const updated = { ...room, orchestratorAgentId: agentId }
  if (isPersistedRoomId(room.id)) {
    try {
      const result = await updateRoomMutation(updated)
      replaceRoom(result)
    } catch (error) {
      console.error('Failed to update room orchestrator:', error)
    }
  } else {
    rooms.value = rooms.value.map((r) => r.id === room.id ? updated : r)
  }
}

async function createRoom() {
  const name = newRoomName.value.trim()
  if (!name) return

  const room: RoomItem = {
    id: `custom:${Date.now()}`,
    name,
    shortName: roomShortName(name),
    subtitle: '自建 Agent 群聊',
    summary: newRoomSummary.value.trim() || '这个群聊还没有描述。你可以先把 Agent 加进来，再从这里发起协作任务。',
    members: 1,
    attention: 0,
    privacy: '本地群',
    live: '等待输入',
    accent: 'bg-slate-700',
    statusClass: 'bg-slate-500',
    agentIds: selectedAgent.value ? [selectedAgent.value.id] : [],
    orchestratorAgentId: newRoomOrchestratorAgentId.value,
  }

  try {
    const created = await createRoomMutation(room)
    const nextRoom = agentHubRoomToItem(created)
    rooms.value = [nextRoom, ...rooms.value]
    selectedRoomId.value = nextRoom.id
    cancelCreateRoom()
  }
  catch (error) {
    console.error('Failed to create AgentHub room:', error)
  }
}

async function addAgentToRoom(room: RoomItem | undefined, agent: AgentItem | undefined) {
  if (!room || !agent || !isPersistedRoomId(room.id) || room.agentIds.includes(agent.id)) return false

  try {
    const updated = await addAgentMutation({ roomId: room.id, agentId: agent.id })
    replaceRoom(updated)
    return true
  }
  catch (error) {
    console.error('Failed to add AgentHub room agent:', error)
    return false
  }
}

async function addAgentToSelectedRoom() {
  await addAgentToRoom(selectedRoom.value, selectedAgent.value)
}

async function ensureMainAgentInSelectedRoom() {
  const room = selectedRoom.value
  const agent = mainAgent.value
  if (!room || !agent?.botId || !isPersistedRoomId(room.id) || room.agentIds.includes(agent.id)) return

  const key = `${room.id}:${agent.id}`
  if (joiningMainAgentKey.value === key) return
  joiningMainAgentKey.value = key
  try {
    await addAgentToRoom(room, agent)
  }
  finally {
    if (joiningMainAgentKey.value === key) {
      joiningMainAgentKey.value = ''
    }
  }
}

async function removeAgentFromSelectedRoom(agentId: string) {
  const room = selectedRoom.value
  if (!room || !isPersistedRoomId(room.id)) return
  try {
    const updated = await removeAgentMutation({ roomId: room.id, agentId })
    replaceRoom(updated)
  }
  catch (error) {
    console.error('Failed to remove AgentHub room agent:', error)
  }
}

async function sendRoomMessage() {
  const room = selectedRoom.value
  const body = composerText.value.trim()
  if (!room || !body || !isPersistedRoomId(room.id) || isAgentReplying.value) return

  try {
    await createMessageMutation({
      roomId: room.id,
      payload: {
        sender_type: 'user',
        sender_name: '我',
        kind: 'message',
        title: '我',
        body,
      },
    })
    composerText.value = ''
    await ensureMainAgentInSelectedRoom()
    void requestMainAgentReply(room, body)
  }
  catch (error) {
    console.error('Failed to create AgentHub room message:', error)
  }
}

// AgentHub rooms reuse one bot session per (room, agent) so the conversation
// accumulates history across turns. The session id lives in the room's
// metadata (persisted backend-side via UpsertRoom), keyed by agent id.
function roomAgentSessions(room: RoomItem | undefined): Record<string, unknown> {
  const map = room?.metadata?.agent_sessions
  return map && typeof map === 'object' ? map as Record<string, unknown> : {}
}

function getAgentSessionId(room: RoomItem | undefined, agentId: string): string {
  const value = roomAgentSessions(room)[agentId]
  return typeof value === 'string' && value.trim() ? value.trim() : ''
}

async function persistAgentSessionId(room: RoomItem, agentId: string, sessionId: string) {
  const nextMetadata: Record<string, unknown> = {
    ...(room.metadata ?? {}),
    agent_sessions: { ...roomAgentSessions(room), [agentId]: sessionId },
  }
  const nextRoom: RoomItem = { ...room, metadata: nextMetadata }
  rooms.value = rooms.value.map((r) => r.id === room.id ? nextRoom : r)
  if (isPersistedRoomId(room.id)) {
    try {
      await updateAgentHubRoom(nextRoom)
    }
    catch (error) {
      // Non-fatal: the in-memory room still carries the session for this page
      // session, so the current conversation stays multi-turn either way.
      console.error('Failed to persist AgentHub agent session id:', error)
    }
  }
}

async function requestMainAgentReply(room: RoomItem, prompt: string) {
  const roomId = room.id
  const agent = mainAgent.value
  if (!agent?.botId) {
    await createMessageMutation({
      roomId,
      payload: {
        sender_type: 'system',
        sender_name: 'AgentHub',
        kind: 'error',
        title: '没有可执行的主 Agent',
        body: '还没有找到可以执行回复的 Agent。请先创建或接入一个机器人。',
      },
    })
    return
  }

  isAgentReplying.value = true
  try {
    const { reply, thinking, tools } = await collectMainAgentReply(agent, room, prompt)
    await createMessageMutation({
      roomId,
      payload: {
        sender_id: agent.id,
        sender_type: 'agent',
        sender_name: agent.name,
        kind: 'reply',
        title: agent.name,
        body: reply || '我收到了，但这次没有生成可展示文本。',
        metadata: {
          ...(thinking ? { thinking } : {}),
          ...(tools.length > 0 ? { tools } : {}),
        } || undefined,
      },
    })
    queryCache.invalidateQueries({ key: ['agent-hub', 'messages', roomId] })
  }
  catch (error) {
    const message = error instanceof Error ? error.message : '主 Agent 回复失败'
    await createMessageMutation({
      roomId,
      payload: {
        sender_id: agent.id,
        sender_type: 'agent',
        sender_name: agent.name,
        kind: 'error',
        title: `${agent.name} 回复失败`,
        body: message,
      },
    })
  }
  finally {
    isAgentReplying.value = false
  }
}

async function collectMainAgentReply(agent: AgentItem, room: RoomItem, prompt: string): Promise<{ reply: string; thinking: string; tools: StoredTool[] }> {
  if (!agent.botId) throw new Error('主 Agent 没有关联的 bot')

  // Reuse this room+agent's existing session so the bot keeps multi-turn
  // memory; only mint (and persist) a new one the first time.
  const existingSessionId = getAgentSessionId(room, agent.id)
  let sessionId = existingSessionId
  if (!sessionId) {
    const session = await createSession(agent.botId, `AgentHub · ${room.name}`)
    sessionId = session.id
    await persistAgentSessionId(room, agent.id, sessionId)
  }
  const textById = new Map<number, string>()
  const reasoningById = new Map<number, string>()
  const toolsById = new Map<number, UIToolMessage>()
  // On the first turn (new session), include the room preamble so the agent
  // knows its role. On subsequent turns, send only the user's actual message
  // so history stays clean and the model can recognize prior conversation.
  const requestText = existingSessionId
    ? prompt
    : [`你是 AgentHub 房间「${room.name}」的主 Agent。`, '请直接回复用户消息，语气保持自然，不要解释内部调度流程。', '', prompt].join('\n')

  return await new Promise<{ reply: string; thinking: string; tools: StoredTool[] }>((resolve, reject) => {
    let settled = false
    let ws: ReturnType<typeof connectWebSocket> | null = null

    const timer = window.setTimeout(() => {
      finish(new Error(`${agent.name} 回复超时`))
    }, 120_000)

    function finish(error?: Error) {
      if (settled) return
      settled = true
      window.clearTimeout(timer)
      ws?.close()
      if (error) {
        reject(error)
        return
      }
      resolve({
        reply: renderCollectedReply(textById),
        thinking: renderCollectedReply(reasoningById),
        tools: [...toolsById.values()].map(t => ({ name: t.name, input: t.input, output: t.output })),
      })
    }

    ws = connectWebSocket(agent.botId!, (event: UIStreamEvent) => {
      if (event.type === 'message') {
        collectUIMessageBlocks(textById, reasoningById, toolsById, event.data)
        return
      }
      if (event.type === 'error') {
        finish(new Error(event.message || '主 Agent 回复失败'))
        return
      }
      if (event.type === 'end') {
        finish()
      }
    })
    ws.send({ type: 'message', text: requestText, session_id: sessionId })
  })
}

function collectUIMessageBlocks(
  textById: Map<number, string>,
  reasoningById: Map<number, string>,
  toolsById: Map<number, UIToolMessage>,
  message: UIMessage,
) {
  if (message.type === 'text' || message.type === 'error') {
    textById.set(message.id, message.content)
  } else if (message.type === 'reasoning') {
    reasoningById.set(message.id, message.content)
  } else if (message.type === 'tool') {
    toolsById.set(message.id, message)
  }
}

function renderCollectedReply(textById: Map<number, string>) {
  return [...textById.entries()]
    .sort(([left], [right]) => left - right)
    .map(([, content]) => content.trim())
    .filter(Boolean)
    .join('\n\n')
}

function handleConnectorClick(connector: ConnectorItem) {
  if (!connector.enabled) return

  activeActivity.value = 'agents'
  if (connector.id === 'memoh-bot') {
    selectedAgentId.value = mainAgent.value.id
    void addAgentToSelectedRoom()
  }
}

function toggleSelectedAgentInRoom() {
  const agent = selectedAgent.value
  if (!agent) return
  if (isSelectedAgentInRoom.value) {
    void removeAgentFromSelectedRoom(agent.id)
    return
  }
  void addAgentToSelectedRoom()
}

function openSelectedBot() {
  if (!selectedAgent.value?.botId) return
  void router.push({
    name: 'chat',
    params: { botId: selectedAgent.value.botId },
  })
}
</script>
