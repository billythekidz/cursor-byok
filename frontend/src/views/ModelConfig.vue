<script setup>
import Button from "@/components/ui/Button.vue";
import Card from "@/components/ui/Card.vue";
import Input from "@/components/ui/Input.vue";
import ModelContextWindowControl from "@/components/ModelContextWindowControl.vue";
import ModelAdapterTestCard from "@/components/ModelAdapterTestCard.vue";
import { showModal } from "@/composables/useModal";
import { scanOpenAIModels } from "@/services/clientApi";
import {
  appState,
  buildOpenAIEndpointGroupKey,
  createEmptyModelAdapter,
  deleteModelAdapterAt,
  duplicateModelAdapterAt,
  getModelAdapterTestResultByID,
  normalizeModelAdapter,
  openModelEditorWindow,
  OPENAI_ENDPOINT_RESPONSES,
  reloadUserConfig,
  runModelAdapterTest,
  saveModelAdapterAt,
  saveModelAdapters,
  startModelAdapterTest,
  toUserError,
} from "@/state/appState";
import { computed, onBeforeUnmount, onMounted, reactive, ref, watch } from "vue";

const BATCH_TEST_CONCURRENCY = 10;

const typeTabs = [
  { label: "OpenAI", value: "openai", icon: "icon-[bxl--openai]" },
  { label: "Anthropic", value: "anthropic", icon: "icon-[logos--claude-icon]" },
];

const activeType = ref("openai");
const batchTesting = ref(false);
const batchStopping = ref(false);
const batchTotal = ref(0);
const batchCompleted = ref(0);
const batchActiveCalls = new Set();
let batchStopRequested = false;

const scanBaseURL = ref("");
const scanAPIKey = ref("");
const scanning = ref(false);
const scanError = ref("");
const expandedGroupID = ref("");
const contextWindowSavingKeys = reactive(new Set());
const contextWindowErrors = reactive({});

const filteredAdapters = computed(() =>
  appState.modelAdapters.filter((adapter) => adapter.type === activeType.value),
);

// openaiGroups groups OpenAI adapters by openAIEndpointGroupID for display.
// Adapters with a groupID are merged into endpoint-group cards; manually added adapters (no groupID) each get their own card.
const openaiGroups = computed(() => {
  const groups = [];
  const grouped = new Map();
  for (const adapter of appState.modelAdapters) {
    if (adapter.type !== "openai") {
      continue;
    }
    if (adapter.openAIEndpointGroupID) {
      if (!grouped.has(adapter.openAIEndpointGroupID)) {
        grouped.set(adapter.openAIEndpointGroupID, []);
      }
      grouped.get(adapter.openAIEndpointGroupID).push(adapter);
    } else {
      groups.push({ groupID: "", adapters: [adapter] });
    }
  }
  for (const [groupID, adapters] of grouped) {
    groups.push({ groupID, adapters });
  }
  return groups;
});

const expandedGroup = computed(() =>
  openaiGroups.value.find((group) => group.groupID !== "" && group.groupID === expandedGroupID.value) || null,
);
const expandedGroupActiveAdapter = computed(() => {
  if (!expandedGroup.value) {
    return null;
  }
  return expandedGroup.value.adapters.find((adapter) => adapter.active) || null;
});

function activeAdapterOf(group) {
  return group.adapters.find((adapter) => adapter.active) || null;
}

const batchButtonText = computed(() => {
  if (batchStopping.value) {
    return "停止中...";
  }
  if (!batchTesting.value) {
    return "测试全部";
  }
  return `停止测试 ${batchCompleted.value}/${batchTotal.value}`;
});

watch(
  () => appState.modelAdapters,
  (adapters) => {
    if (adapters.some((adapter) => adapter.type === activeType.value)) {
      return;
    }
    const fallback = typeTabs.find((tab) => adapters.some((adapter) => adapter.type === tab.value));
    activeType.value = fallback?.value ?? "openai";
  },
  { deep: true, immediate: true },
);

async function showActionError(title, error) {
  await showModal({
    title,
    content: String(error || "服务错误").trim() || "服务错误",
  });
}

function maskSecret(value) {
  const text = String(value || "").trim();
  if (!text) {
    return "-";
  }
  if (text.length <= 8) {
    return `${"*".repeat(Math.max(text.length - 2, 0))}${text.slice(-2)}`;
  }
  return `${text.slice(0, 4)}****${text.slice(-4)}`;
}

function typeLabel(type) {
  return type === "anthropic" ? "Anthropic" : "OpenAI";
}

function formatHost(value) {
  const text = String(value || "").trim();
  if (!text) {
    return "-";
  }
  try {
    const parsed = new URL(text);
    return parsed.host || text;
  } catch {
    return text.replace(/^https?:\/\//, "");
  }
}

async function openEditor(index = -1) {
  const adapter = index >= 0
    ? appState.modelAdapters[index]
    : {
        ...createEmptyModelAdapter(),
        type: activeType.value,
      };
  try {
    await openModelEditorWindow(index, adapter);
  } catch (error) {
    await showActionError("打开失败", toUserError(error));
  }
}

async function handleDeleteModelAdapter(index) {
  const target = appState.modelAdapters[index];
  if (!target) {
    await showActionError("删除失败", "模型配置不存在，无法删除");
    return;
  }
  const result = await deleteModelAdapterAt(index);
  if (!result.ok) {
    await showActionError("删除失败", result.error);
  }
}

async function handleDuplicateModelAdapter(index) {
  const target = appState.modelAdapters[index];
  if (!target) {
    await showActionError("复制失败", "模型配置不存在，无法复制");
    return;
  }
  const result = await duplicateModelAdapterAt(index);
  if (!result.ok) {
    await showActionError("复制失败", result.error);
  }
}

function getAdapterTestResult(adapter) {
  return getModelAdapterTestResultByID(adapter?.id);
}

function adapterContextKey(adapter) {
  return adapter?.id || [adapter?.type, adapter?.baseURL, adapter?.modelID, adapter?.displayName].join("\u0000");
}

function isContextWindowSaving(adapter) {
  return contextWindowSavingKeys.has(adapterContextKey(adapter));
}

function contextWindowError(adapter) {
  return contextWindowErrors[adapterContextKey(adapter)] || "";
}

async function handleSaveContextWindow(adapter, rawValue) {
  const key = adapterContextKey(adapter);
  const index = appState.modelAdapters.indexOf(adapter);
  if (!key || index < 0 || appState.configSaving || contextWindowSavingKeys.has(key)) {
    return;
  }

  const text = String(rawValue || "").trim();
  const contextWindowTokens = text ? Number(text) : 0;
  if (text && (!Number.isSafeInteger(contextWindowTokens) || contextWindowTokens <= 0)) {
    contextWindowErrors[key] = "上下文窗口必须为正整数";
    return;
  }

  contextWindowSavingKeys.add(key);
  delete contextWindowErrors[key];
  try {
    const current = normalizeModelAdapter(appState.modelAdapters[index]);
    const result = await saveModelAdapterAt(index, {
      ...current,
      contextWindowTokens,
    });
    if (!result.ok) {
      contextWindowErrors[key] = result.error || "保存失败";
    }
  } catch (error) {
    contextWindowErrors[key] = toUserError(error);
  } finally {
    contextWindowSavingKeys.delete(key);
  }
}

function isAdapterTesting(adapter) {
  return getAdapterTestResult(adapter)?.status === "running";
}

async function handleTestModelAdapter(adapter) {
  try {
    await runModelAdapterTest(adapter);
  } catch (_error) {
    // Failures are synced to the UI through events; no extra modal interrupts the user here.
  }
}

function isCancelError(error) {
  return String(error?.name || "").trim() === "CancelError";
}

async function stopBatchTesting() {
  if (!batchTesting.value || batchStopping.value) {
    return;
  }
  batchStopRequested = true;
  batchStopping.value = true;
  const activeCalls = Array.from(batchActiveCalls);
  await Promise.allSettled(
    activeCalls.map((call) => (typeof call?.cancel === "function" ? call.cancel("batch-stop") : undefined)),
  );
}

async function handleTestAllModelAdapters() {
  if (batchTesting.value) {
    await stopBatchTesting();
    return;
  }
  const adapters = filteredAdapters.value.slice();
  if (adapters.length === 0) {
    return;
  }
  batchStopRequested = false;
  batchTesting.value = true;
  batchStopping.value = false;
  batchTotal.value = adapters.length;
  batchCompleted.value = 0;
  let nextIndex = 0;
  try {
    const workers = Array.from({ length: Math.min(BATCH_TEST_CONCURRENCY, adapters.length) }, async () => {
      while (!batchStopRequested) {
        const currentIndex = nextIndex;
        nextIndex += 1;
        if (currentIndex >= adapters.length) {
          return;
        }
        const adapter = adapters[currentIndex];
        const call = startModelAdapterTest(adapter);
        batchActiveCalls.add(call);
        try {
          await call;
        } catch (error) {
          if (!isCancelError(error) && !batchStopRequested) {
            // Individual failures are shown by each card itself; testing continues here.
          }
        } finally {
          batchActiveCalls.delete(call);
          batchCompleted.value += 1;
        }
      }
    });
    await Promise.allSettled(workers);
  } finally {
    batchActiveCalls.clear();
    batchStopRequested = false;
    batchTesting.value = false;
    batchStopping.value = false;
  }
}

async function handleScanOpenAI() {
  if (scanning.value) {
    return;
  }
  const baseURL = scanBaseURL.value.trim();
  const apiKey = scanAPIKey.value.trim();
  if (!baseURL || !apiKey) {
    await showActionError("扫描失败", "请先填写接口地址与访问密钥");
    return;
  }
  scanning.value = true;
  scanError.value = "";
  try {
    const models = await scanOpenAIModels(baseURL, apiKey);
    if (!Array.isArray(models) || models.length === 0) {
      scanError.value = "未扫描到任何模型";
      return;
    }
    const groupID = buildOpenAIEndpointGroupKey(baseURL, apiKey);
    const current = appState.modelAdapters.map((adapter) => normalizeModelAdapter(adapter));
    const existingKeys = new Set(current.map((adapter) => `${adapter.openAIEndpointGroupID}::${adapter.modelID}`));
    const newAdapters = [];
    for (const info of models) {
      const modelID = String(info?.modelID || "").trim();
      if (!modelID) {
        continue;
      }
      const key = `${groupID}::${modelID}`;
      if (existingKeys.has(key)) {
        continue;
      }
      existingKeys.add(key);
      newAdapters.push({
        ...createEmptyModelAdapter(),
        type: "openai",
        baseURL,
        apiKey,
        modelID,
        displayName: modelID,
        tooltipData: "备注",
        reasoningEffort: "medium",
        openAIEndpoint: OPENAI_ENDPOINT_RESPONSES,
        openAIEndpointGroupID: groupID,
        active: false,
      });
    }
    const merged = [...current, ...newAdapters];
    const result = await saveModelAdapters(merged);
    if (!result.ok) {
      scanError.value = result.error;
      return;
    }
    await reloadUserConfig({ modelAdaptersOnly: true });
    expandedGroupID.value = groupID;
    scanBaseURL.value = "";
    scanAPIKey.value = "";
    await showModal({
      title: "扫描完成",
      content: newAdapters.length > 0 ? `新增 ${newAdapters.length} 个模型` : "没有新增模型，列表已是最新",
    });
  } catch (error) {
    scanError.value = toUserError(error);
  } finally {
    scanning.value = false;
  }
}

async function handleToggleActive(adapter) {
  if (appState.configSaving || !adapter) {
    return;
  }
  const groupID = adapter.openAIEndpointGroupID;
  const nextActive = !adapter.active;
  const current = appState.modelAdapters.map((item) => normalizeModelAdapter(item));
  for (const item of current) {
    if (groupID) {
      // At most one adapter can be active within the same endpoint group (mutually exclusive).
      if (item.openAIEndpointGroupID === groupID) {
        item.active = item.modelID === adapter.modelID && nextActive;
      }
    } else if (item.modelID === adapter.modelID && item.baseURL === adapter.baseURL && item.apiKey === adapter.apiKey) {
      // Manual models (no group) toggle their own active independently.
      item.active = nextActive;
    }
  }
  const result = await saveModelAdapters(current);
  if (!result.ok) {
    await showActionError("保存失败", result.error);
    return;
  }
  await reloadUserConfig({ modelAdaptersOnly: true });
}

onMounted(async () => {
  await reloadUserConfig({ modelAdaptersOnly: true }).catch(() => { });
});

onBeforeUnmount(() => {
  void stopBatchTesting();
});
</script>

<template>
  <div class="flex h-full min-h-0 flex-col p-4 pt-0 text-[#e5e5e5] overflow-hidden">
    <div class="shrink-0 pb-4">
      <div class="flex items-center justify-between gap-4">
        <div class="center-row gap-2">
          <button
            v-for="tab in typeTabs"
            :key="tab.value"
            type="button"
            class="center-row gap-2 rounded-[8px] border px-3 py-2 text-sm transition-colors duration-150"
            :class="activeType === tab.value
              ? 'border-[#1ca35a] bg-[#123322] text-white'
              : 'border-[#343434] bg-[#252525] text-[#a3a3a3] hover:border-[#4a4a4a] hover:text-[#e5e5e5]'"
            @click="activeType = tab.value"
          >
            <span :class="[tab.icon, 'text-[16px]']"></span>
            <span>{{ tab.label }}</span>
          </button>
        </div>
        <div class="center-row gap-2">
          <Button
            variant="default"
            :disabled="appState.configSaving || (!batchTesting && filteredAdapters.length === 0)"
            @click="handleTestAllModelAdapters"
          >
            {{ batchButtonText }}
          </Button>
          <Button variant="primary" :disabled="appState.configSaving || batchTesting" @click="openEditor()">新增模型</Button>
        </div>
      </div>
    </div>

    <div class="min-h-0 flex-1 overflow-y-auto pr-1">
      <div v-if="activeType === 'openai'" class="mb-3 rounded-[8px] border border-[#343434] bg-[#232323] p-3">
        <div class="mb-2 text-sm font-medium text-white">扫描 OpenAI 兼容接口</div>
        <div class="grid grid-cols-2 gap-2">
          <Input v-model="scanBaseURL" placeholder="https://api.openai.com/v1" />
          <Input
            v-model="scanAPIKey"
            type="password"
            allow-visibility-toggle
            placeholder="访问密钥"
          />
        </div>
        <div class="mt-2 flex items-center gap-3">
          <Button
            variant="primary"
            :disabled="scanning || appState.configSaving"
            @click="handleScanOpenAI"
          >
            {{ scanning ? "扫描中..." : "扫描模型" }}
          </Button>
          <span v-if="scanError" class="min-w-0 truncate text-xs text-[#e06c75]">{{ scanError }}</span>
        </div>
      </div>

      <template v-if="activeType === 'anthropic'">
        <div
          v-if="filteredAdapters.length === 0"
          class="flex h-full min-h-[220px] items-center justify-center rounded-[8px] border border-dashed border-[#3a3a3a] bg-[#232323] px-4 text-sm text-[#a3a3a3]"
        >
          当前还没有配置任何 {{ typeLabel(activeType) }} 模型。
        </div>

        <div v-else class="grid gap-3 pb-1 [grid-template-columns:repeat(auto-fill,minmax(250px,1fr))]">
          <Card
            v-for="(adapter, index) in filteredAdapters"
            :key="adapter.id || `${adapter.baseURL}-${adapter.modelID}-${index}`"
          >
            <div class="flex h-full min-h-[154px] flex-col justify-between gap-3">
              <div class="flex flex-col gap-2.5">
                <div class="flex items-start justify-between gap-3">
                  <div class="min-w-0 flex-1">
                    <div class="truncate text-base font-medium text-white">{{ adapter.displayName }}</div>
                    <div class="mt-1 truncate text-sm text-[#8f8f8f]">{{ adapter.modelID }}</div>
                  </div>
                  <span
                    class="center-row shrink-0 gap-1 rounded-[999px] border border-[#3f3f3f] px-[7px] py-[4px] text-[11px] font-medium text-[#cfcfcf]"
                  >
                    <span class="icon-[logos--claude-icon] text-[14px]"></span>
                    <span>{{ typeLabel(adapter.type) }}</span>
                  </span>
                </div>

                <div class="grid grid-cols-2 gap-2 text-sm text-[#a3a3a3]">
                  <div class="rounded-[8px] bg-[#232323] px-3 py-2">
                    <div class="text-[11px] uppercase tracking-[0.08em] text-[#666]">Host</div>
                    <div class="mt-1 truncate text-[#d4d4d4]" :title="adapter.baseURL">{{ formatHost(adapter.baseURL) }}</div>
                  </div>
                  <div class="rounded-[8px] bg-[#232323] px-3 py-2">
                    <div class="text-[11px] uppercase tracking-[0.08em] text-[#666]">API Key</div>
                    <div class="mt-1 truncate text-[#d4d4d4]">{{ maskSecret(adapter.apiKey) }}</div>
                  </div>
                </div>

                <ModelContextWindowControl
                  :value="adapter.contextWindowTokens"
                  :disabled="appState.configSaving || batchTesting"
                  :saving="isContextWindowSaving(adapter)"
                  :error="contextWindowError(adapter)"
                  @save="handleSaveContextWindow(adapter, $event)"
                />

                <ModelAdapterTestCard
                  compact
                  title="测试"
                  empty-text="未测试"
                  :result="getAdapterTestResult(adapter)"
                />
              </div>

              <div class="center-row flex-wrap justify-end gap-2 border-t border-[#343434] pt-3">
                <Button
                  variant="default"
                  :disabled="appState.configSaving || batchTesting || isAdapterTesting(adapter)"
                  @click="handleTestModelAdapter(adapter)"
                >
                  {{ isAdapterTesting(adapter) ? "测试中..." : "测试" }}
                </Button>
                <Button variant="default" :disabled="appState.configSaving" @click="openEditor(appState.modelAdapters.indexOf(adapter))">编辑</Button>
                <Button variant="default" :disabled="appState.configSaving" @click="handleDuplicateModelAdapter(appState.modelAdapters.indexOf(adapter))">复制</Button>
                <Button variant="text" :disabled="appState.configSaving"
                  @click="handleDeleteModelAdapter(appState.modelAdapters.indexOf(adapter))">删除</Button>
              </div>
            </div>
          </Card>
        </div>
      </template>

      <template v-else>
        <div
          v-if="openaiGroups.length === 0"
          class="flex h-full min-h-[220px] items-center justify-center rounded-[8px] border border-dashed border-[#3a3a3a] bg-[#232323] px-4 text-sm text-[#a3a3a3]"
        >
          当前还没有配置任何 OpenAI 模型，可以先用上方扫描功能批量导入。
        </div>

        <div v-else>
          <div class="grid gap-3 pb-1 [grid-template-columns:repeat(auto-fill,minmax(250px,1fr))]">
            <Card
              v-for="(group, groupIndex) in openaiGroups"
              :key="group.groupID || group.adapters[0]?.id || `manual-${groupIndex}`"
            >
              <div class="flex h-full min-h-[154px] flex-col justify-between gap-3">
                <div class="flex flex-col gap-2.5">
                  <div
                    v-if="group.groupID"
                    class="flex items-start justify-between gap-3"
                    role="button"
                    tabindex="0"
                    @click="expandedGroupID = expandedGroupID === group.groupID ? '' : group.groupID"
                    @keydown.enter="expandedGroupID = expandedGroupID === group.groupID ? '' : group.groupID"
                  >
                    <div class="min-w-0 flex-1">
                      <div class="truncate text-base font-medium text-white">{{ formatHost(group.adapters[0]?.baseURL) }}</div>
                      <div class="mt-1 truncate text-sm text-[#8f8f8f]">{{ group.adapters.length }} 个模型</div>
                      <div class="mt-0.5 truncate text-xs text-[#737373]">
                        {{ activeAdapterOf(group) ? `激活：${activeAdapterOf(group).modelID}` : "未激活" }}
                      </div>
                    </div>
                    <span
                      class="center-row shrink-0 gap-1 rounded-[999px] border border-[#3f3f3f] px-[7px] py-[4px] text-[11px] font-medium text-[#cfcfcf]"
                    >
                      <span class="icon-[bxl--openai] text-[14px] !text-white"></span>
                      <span>OpenAI</span>
                    </span>
                  </div>

                  <div v-else class="flex items-start justify-between gap-3">
                    <div class="min-w-0 flex-1">
                      <div class="truncate text-base font-medium text-white">{{ group.adapters[0]?.displayName }}</div>
                      <div class="mt-1 truncate text-sm text-[#8f8f8f]">{{ group.adapters[0]?.modelID }}</div>
                      <div class="mt-0.5 truncate text-xs text-[#737373]">
                        {{ group.adapters[0]?.openAIEndpoint || "/v1/responses" }}
                      </div>
                    </div>
                    <span
                      class="center-row shrink-0 gap-1 rounded-[999px] border border-[#3f3f3f] px-[7px] py-[4px] text-[11px] font-medium text-[#cfcfcf]"
                    >
                      <span class="icon-[bxl--openai] text-[14px] !text-white"></span>
                      <span>OpenAI</span>
                    </span>
                  </div>

                  <div class="grid grid-cols-2 gap-2 text-sm text-[#a3a3a3]">
                    <div class="rounded-[8px] bg-[#232323] px-3 py-2">
                      <div class="text-[11px] uppercase tracking-[0.08em] text-[#666]">Host</div>
                      <div class="mt-1 truncate text-[#d4d4d4]" :title="group.adapters[0]?.baseURL">{{ formatHost(group.adapters[0]?.baseURL) }}</div>
                    </div>
                    <div class="rounded-[8px] bg-[#232323] px-3 py-2">
                      <div class="text-[11px] uppercase tracking-[0.08em] text-[#666]">API Key</div>
                      <div class="mt-1 truncate text-[#d4d4d4]">{{ maskSecret(group.adapters[0]?.apiKey) }}</div>
                    </div>
                  </div>

                </div>

                <div class="center-row flex-wrap justify-end gap-2 border-t border-[#343434] pt-3">
                  <template v-if="group.groupID">
                    <Button variant="default" :disabled="appState.configSaving" @click="expandedGroupID = group.groupID">
                      {{ expandedGroupID === group.groupID ? "收起" : "展开" }}
                    </Button>
                  </template>
                  <template v-else>
                    <Button
                      variant="default"
                      :disabled="appState.configSaving || batchTesting || isAdapterTesting(group.adapters[0])"
                      @click="handleTestModelAdapter(group.adapters[0])"
                    >
                      {{ isAdapterTesting(group.adapters[0]) ? "测试中..." : "测试" }}
                    </Button>
                    <Button variant="default" :disabled="appState.configSaving" @click="openEditor(appState.modelAdapters.indexOf(group.adapters[0]))">编辑</Button>
                    <Button variant="default" :disabled="appState.configSaving" @click="handleDuplicateModelAdapter(appState.modelAdapters.indexOf(group.adapters[0]))">复制</Button>
                    <Button variant="text" :disabled="appState.configSaving"
                      @click="handleDeleteModelAdapter(appState.modelAdapters.indexOf(group.adapters[0]))">删除</Button>
                  </template>
                </div>
              </div>
            </Card>
          </div>

          <div
            v-if="expandedGroup"
            class="mt-3 rounded-[8px] border border-[#343434] bg-[#232323] p-4"
          >
            <div class="mb-3 flex items-center justify-between gap-3">
              <div class="min-w-0 flex-1">
                <div class="truncate text-sm font-medium text-white">{{ formatHost(expandedGroup.adapters[0]?.baseURL) }}</div>
                <div class="mt-0.5 truncate text-xs text-[#737373]">{{ expandedGroup.adapters.length }} 个模型</div>
              </div>
              <Button variant="text" :disabled="appState.configSaving" @click="expandedGroupID = ''">收起</Button>
            </div>

            <div class="mb-4">
              <div class="mb-2 text-[11px] uppercase tracking-[0.08em] text-[#666]">当前激活模型</div>
              <Card v-if="expandedGroupActiveAdapter">
                <div class="flex items-center justify-between gap-3">
                  <div class="min-w-0 flex-1">
                    <div class="truncate text-sm font-medium text-white">{{ expandedGroupActiveAdapter.displayName }}</div>
                    <div class="mt-0.5 truncate text-xs text-[#8f8f8f]">{{ expandedGroupActiveAdapter.modelID }}</div>
                  </div>
                  <span class="center-row shrink-0 gap-1 rounded-[999px] border border-[#1ca35a] bg-[#123322] px-[7px] py-[4px] text-[11px] font-medium text-[#10AD5D]">
                    <span class="icon-[mdi--check-circle] text-[13px]"></span>
                    <span>已激活</span>
                  </span>
                </div>
              </Card>
              <div
                v-else
                class="flex h-[68px] items-center justify-center rounded-[8px] border border-dashed border-[#3a3a3a] bg-[#1f1f1f] px-4 text-center text-xs text-[#737373]"
              >
                该组还没有激活模型，请在下方向中选择一个模型设为激活
              </div>
            </div>

            <div class="grid gap-2 [grid-template-columns:repeat(auto-fill,minmax(220px,1fr))]">
              <Card
                v-for="adapter in expandedGroup.adapters"
                :key="adapter.id || `${adapter.baseURL}-${adapter.modelID}`"
              >
                <div class="flex flex-col gap-2.5">
                  <div>
                    <div class="truncate text-sm font-medium text-white">{{ adapter.displayName }}</div>
                    <div class="mt-0.5 truncate text-xs text-[#8f8f8f]">{{ adapter.modelID }}</div>
                  </div>
                  <div class="flex items-center justify-between gap-2">
                    <span
                      class="center-row shrink-0 gap-1 rounded-[999px] border px-[7px] py-[4px] text-[11px] font-medium"
                      :class="adapter.active
                        ? 'border-[#1ca35a] bg-[#123322] text-[#10AD5D]'
                        : 'border-[#3f3f3f] bg-[#1f1f1f] text-[#8f8f8f]'"
                    >
                      <span :class="adapter.active ? 'icon-[mdi--check-circle] text-[13px]' : 'icon-[mdi--circle-outline] text-[13px]'"></span>
                      <span>{{ adapter.active ? "已激活" : "未激活" }}</span>
                    </span>
                    <Button
                      variant="default"
                      :disabled="appState.configSaving || batchTesting"
                      @click="handleToggleActive(adapter)"
                    >
                      {{ adapter.active ? "取消激活" : "设为激活" }}
                    </Button>
                  </div>
                  <ModelContextWindowControl
                    :value="adapter.contextWindowTokens"
                    :disabled="appState.configSaving || batchTesting"
                    :saving="isContextWindowSaving(adapter)"
                    :error="contextWindowError(adapter)"
                    @save="handleSaveContextWindow(adapter, $event)"
                  />
                  <div class="center-row flex-wrap justify-end gap-2 border-t border-[#343434] pt-2.5">
                    <Button
                      variant="default"
                      :disabled="appState.configSaving || batchTesting || isAdapterTesting(adapter)"
                      @click="handleTestModelAdapter(adapter)"
                    >
                      {{ isAdapterTesting(adapter) ? "测试中..." : "测试" }}
                    </Button>
                    <Button variant="default" :disabled="appState.configSaving" @click="openEditor(appState.modelAdapters.indexOf(adapter))">编辑</Button>
                    <Button variant="text" :disabled="appState.configSaving"
                      @click="handleDeleteModelAdapter(appState.modelAdapters.indexOf(adapter))">删除</Button>
                  </div>
                </div>
              </Card>
            </div>
          </div>
        </div>
      </template>
    </div>
  </div>
</template>
