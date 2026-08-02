<script setup>
import Button from "@/components/ui/Button.vue";
import Card from "@/components/ui/Card.vue";
import Input from "@/components/ui/Input.vue";
import ModelContextWindowControl from "@/components/ModelContextWindowControl.vue";
import ModelAdapterTestCard from "@/components/ModelAdapterTestCard.vue";
import Switch from "@/components/ui/Switch.vue";
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
  { label: "Codex", value: "codex", icon: "icon-[mdi--robot-outline]" },
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
const showEndpointForm = ref(false);
const contextWindowSavingKeys = reactive(new Set());
const contextWindowErrors = reactive({});

const filteredAdapters = computed(() =>
  appState.modelAdapters.filter((adapter) => adapter.type === activeType.value),
);

function endpointGroupIDOf(adapter) {
  if (adapter?.openAIEndpointGroupID) {
    return adapter.openAIEndpointGroupID;
  }
  return buildOpenAIEndpointGroupKey(adapter?.baseURL, adapter?.apiKey);
}

// openaiGroups is the endpoint layer of the UI. Older manually added models may
// not have an explicit group ID, so derive the same stable key from their URL and key.
const openaiGroups = computed(() => {
  const grouped = new Map();
  for (const adapter of appState.modelAdapters) {
    if (adapter.type !== "openai") {
      continue;
    }
    const groupID = endpointGroupIDOf(adapter);
    if (!grouped.has(groupID)) {
      grouped.set(groupID, []);
    }
    grouped.get(groupID).push(adapter);
  }
  return Array.from(grouped, ([groupID, adapters]) => ({ groupID, adapters }));
});

const expandedGroup = computed(() =>
  openaiGroups.value.find((group) => group.groupID === expandedGroupID.value) || null,
);

function activeAdapterOf(group) {
  return group.adapters.find((adapter) => adapter.active) || null;
}

const batchButtonText = computed(() => {
  if (batchStopping.value) {
    return "Stopping...";
  }
  if (!batchTesting.value) {
    return "Test All";
  }
  return `Stop Testing ${batchCompleted.value}/${batchTotal.value}`;
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

watch(openaiGroups, (groups) => {
  if (expandedGroupID.value && !groups.some((group) => group.groupID === expandedGroupID.value)) {
    expandedGroupID.value = "";
  }
}, { deep: true });

async function showActionError(title, error) {
  await showModal({
    title,
    content: String(error || "Service error").trim() || "Service error",
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
  if (type === "anthropic") {
    return "Anthropic";
  }
  if (type === "codex") {
    return "Codex";
  }
  return "OpenAI";
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

async function openEditor(index = -1, adapterOverride = null) {
  const adapter = index >= 0
    ? appState.modelAdapters[index]
    : adapterOverride || {
        ...createEmptyModelAdapter(),
        type: activeType.value,
      };
  try {
    await openModelEditorWindow(index, adapter);
  } catch (error) {
    await showActionError("Open failed", toUserError(error));
  }
}

function openEditorForSelectedEndpoint() {
  const endpoint = expandedGroup.value?.adapters[0];
  if (!endpoint) {
    return openEditor();
  }
  return openEditor(-1, {
    ...createEmptyModelAdapter(),
    type: "openai",
    baseURL: endpoint.baseURL,
    apiKey: endpoint.apiKey,
    openAIEndpoint: endpoint.openAIEndpoint,
    openAIEndpointGroupID: endpointGroupIDOf(endpoint),
  });
}

async function handleDeleteModelAdapter(index) {
  const target = appState.modelAdapters[index];
  if (!target) {
    await showActionError("Delete failed", "Model configuration does not exist");
    return;
  }
  const result = await deleteModelAdapterAt(index);
  if (!result.ok) {
    await showActionError("Delete failed", result.error);
  }
}

async function handleDuplicateModelAdapter(index) {
  const target = appState.modelAdapters[index];
  if (!target) {
    await showActionError("Duplicate failed", "Model configuration does not exist");
    return;
  }
  const result = await duplicateModelAdapterAt(index);
  if (!result.ok) {
    await showActionError("Duplicate failed", result.error);
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
    contextWindowErrors[key] = "Context window must be a positive integer";
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
      contextWindowErrors[key] = result.error || "Save failed";
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
    await showActionError("Scan failed", "Please fill in endpoint URL and API Key first");
    return;
  }
  scanning.value = true;
  scanError.value = "";
  try {
    const models = await scanOpenAIModels(baseURL, apiKey);
    if (!Array.isArray(models) || models.length === 0) {
      scanError.value = "No models found";
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
        tooltipData: "Notes",
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
    expandedGroupID.value = "";
    showEndpointForm.value = false;
    scanBaseURL.value = "";
    scanAPIKey.value = "";
    await showModal({
      title: "Scan Completed",
      content: newAdapters.length > 0 ? `Added ${newAdapters.length} models` : "No new models added, list is up to date",
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
	if (
		adapter.type === "codex" &&
		!adapter.active &&
		(!appState.codexRuntime.installed || !appState.codexRuntime.authenticated)
	) {
    await showActionError("Codex 尚未就绪", "请先安装并登录 Codex，再激活此模型。");
    return;
  }
  const nextActive = !adapter.active;
  const current = appState.modelAdapters.map((item) => normalizeModelAdapter(item));
  if (adapter.type === "openai") {
    const groupID = endpointGroupIDOf(adapter);
    for (const item of current) {
      if (item.type === "openai" && endpointGroupIDOf(item) === groupID) {
        // At most one adapter can be active within the same endpoint group (mutually exclusive).
        item.active = item.modelID === adapter.modelID && nextActive;
      }
    }
  } else {
    for (const item of current) {
      if (item.type === adapter.type && item.modelID === adapter.modelID && item.baseURL === adapter.baseURL && item.apiKey === adapter.apiKey) {
        item.active = nextActive;
      }
    }
  }
  const result = await saveModelAdapters(current);
  if (!result.ok) {
    await showActionError("Save failed", result.error);
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
          <Button variant="primary" :disabled="appState.configSaving || batchTesting" @click="activeType === 'openai' && expandedGroup ? openEditorForSelectedEndpoint() : openEditor()">Add Model</Button>
        </div>
      </div>
    </div>

    <div class="min-h-0 flex-1 overflow-y-auto pr-1">
      <div v-if="activeType === 'openai' && (showEndpointForm || openaiGroups.length === 0)" class="mb-3 rounded-[8px] border border-[#343434] bg-[#232323] p-3">
        <div class="mb-2 flex items-center justify-between gap-3">
          <div class="text-sm font-medium text-white">Add OpenAI Compatible Endpoint</div>
          <Button v-if="openaiGroups.length > 0" variant="text" :disabled="appState.configSaving || scanning" @click="showEndpointForm = false">Close</Button>
        </div>
        <div class="grid grid-cols-2 gap-2">
          <Input v-model="scanBaseURL" placeholder="https://api.openai.com/v1" />
          <Input
            v-model="scanAPIKey"
            type="password"
            allow-visibility-toggle
            placeholder="API Key"
          />
        </div>
        <div class="mt-2 flex items-center gap-3">
          <Button
            variant="primary"
            :disabled="scanning || appState.configSaving"
            @click="handleScanOpenAI"
          >
            {{ scanning ? "Scanning..." : "Scan Models" }}
          </Button>
          <span v-if="scanError" class="min-w-0 truncate text-xs text-[#e06c75]">{{ scanError }}</span>
        </div>
      </div>

      <template v-if="activeType === 'anthropic'">
        <div
          v-if="filteredAdapters.length === 0"
          class="flex h-full min-h-[220px] items-center justify-center rounded-[8px] border border-dashed border-[#3a3a3a] bg-[#232323] px-4 text-sm text-[#a3a3a3]"
        >
          No {{ typeLabel(activeType) }} models configured yet.
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
                  title="Test"
                  empty-text="Not tested"
                  :result="getAdapterTestResult(adapter)"
                />
              </div>

              <div class="center-row flex-wrap justify-end gap-2 border-t border-[#343434] pt-3">
                <Button
                  variant="default"
                  :disabled="appState.configSaving || batchTesting || isAdapterTesting(adapter)"
                  @click="handleTestModelAdapter(adapter)"
                >
                  {{ isAdapterTesting(adapter) ? "Testing..." : "Test" }}
                </Button>
                <Button variant="default" :disabled="appState.configSaving" @click="openEditor(appState.modelAdapters.indexOf(adapter))">Edit</Button>
                <Button variant="default" :disabled="appState.configSaving" @click="handleDuplicateModelAdapter(appState.modelAdapters.indexOf(adapter))">Duplicate</Button>
                <Button variant="text" :disabled="appState.configSaving"
                  @click="handleDeleteModelAdapter(appState.modelAdapters.indexOf(adapter))">Delete</Button>
              </div>
            </div>
          </Card>
        </div>
      </template>

      <template v-else-if="activeType === 'codex'">
        <div
          v-if="filteredAdapters.length === 0"
          class="flex h-full min-h-[220px] items-center justify-center rounded-[8px] border border-dashed border-[#3a3a3a] bg-[#232323] px-4 text-sm text-[#a3a3a3]"
        >
          尚未配置 Codex 模型。
        </div>
        <div v-else class="grid gap-3 pb-1 [grid-template-columns:repeat(auto-fill,minmax(250px,1fr))]">
          <Card v-for="adapter in filteredAdapters" :key="adapter.id || adapter.modelID">
            <div class="flex h-full min-h-[180px] flex-col justify-between gap-3">
              <div class="flex flex-col gap-2.5">
                <div class="flex items-start justify-between gap-3">
                  <div class="min-w-0 flex-1">
                    <div class="truncate text-base font-medium text-white">{{ adapter.displayName }}</div>
                    <div class="mt-1 truncate text-sm text-[#8f8f8f]">{{ adapter.modelID }}</div>
                  </div>
                  <span class="center-row shrink-0 gap-1 rounded-[999px] border border-[#3f3f3f] px-[7px] py-[4px] text-[11px] font-medium text-[#cfcfcf]">
                    <span class="icon-[mdi--robot-outline] text-[14px]"></span>
                    <span>Codex</span>
                  </span>
                </div>
                <div class="rounded-[8px] bg-[#232323] px-3 py-2 text-sm text-[#a3a3a3]">
                    <div class="text-[11px] uppercase tracking-[0.08em] text-[#666]">运行环境</div>
                  <div class="mt-1 text-[#d4d4d4]">
                    {{ appState.codexRuntime.installed && appState.codexRuntime.authenticated ? `已就绪 ${appState.codexRuntime.version}` : "需要安装并登录" }}
                  </div>
                </div>
                <div class="text-xs text-[#e0a458]">Codex 管理 Shell 和文件工具。审批策略：never。</div>
                <ModelContextWindowControl
                  :value="adapter.contextWindowTokens"
                  :disabled="appState.configSaving || batchTesting"
                  :saving="isContextWindowSaving(adapter)"
                  :error="contextWindowError(adapter)"
                  @save="handleSaveContextWindow(adapter, $event)"
                />
              </div>
              <div class="center-row flex-wrap justify-end gap-2 border-t border-[#343434] pt-3">
                <Button variant="default" :disabled="appState.configSaving || batchTesting || isAdapterTesting(adapter)" @click="handleTestModelAdapter(adapter)">
                  {{ isAdapterTesting(adapter) ? "Testing..." : "Test" }}
                </Button>
                <Button variant="default" :disabled="appState.configSaving" @click="openEditor(appState.modelAdapters.indexOf(adapter))">Edit</Button>
                <Button variant="default" :disabled="appState.configSaving || batchTesting" @click="handleToggleActive(adapter)">
                  {{ adapter.active ? "Deactivate" : "Activate" }}
                </Button>
                <Button variant="text" :disabled="appState.configSaving" @click="handleDeleteModelAdapter(appState.modelAdapters.indexOf(adapter))">Delete</Button>
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
          No OpenAI models configured yet. You can use the scan feature above to import models in batch.
        </div>

        <div v-else>
          <div class="mb-2 flex items-end justify-between gap-3">
            <div>
              <div class="text-[11px] font-medium uppercase tracking-[0.12em] text-[#737373]">Endpoints</div>
              <div class="mt-1 text-xs text-[#8f8f8f]">Choose an endpoint to manage its models.</div>
            </div>
            <Button variant="default" :disabled="appState.configSaving || batchTesting" @click="showEndpointForm = true">+ Add Endpoint</Button>
          </div>
          <div class="grid gap-3 pb-1 [grid-template-columns:repeat(auto-fill,minmax(250px,1fr))]">
            <Card
              v-for="(group, groupIndex) in openaiGroups"
              :key="group.groupID || group.adapters[0]?.id || `manual-${groupIndex}`"
            >
              <div
                class="flex h-full min-h-[154px] cursor-pointer flex-col justify-between gap-3 rounded-[7px] border border-transparent p-1 transition-colors duration-150 hover:border-[#4a4a4a]"
                :class="expandedGroupID === group.groupID ? 'border-[#1ca35a] bg-[#123322]/35' : ''"
                role="button"
                tabindex="0"
                @click="expandedGroupID = expandedGroupID === group.groupID ? '' : group.groupID"
                @keydown.enter="expandedGroupID = expandedGroupID === group.groupID ? '' : group.groupID"
                @keydown.space.prevent="expandedGroupID = expandedGroupID === group.groupID ? '' : group.groupID"
              >
                <div class="flex flex-col gap-2.5">
                  <div
                    v-if="group.groupID"
                    class="flex items-start justify-between gap-3"
                  >
                    <div class="min-w-0 flex-1">
                      <div class="truncate text-base font-medium text-white">{{ formatHost(group.adapters[0]?.baseURL) }}</div>
                      <div class="mt-1 truncate text-sm text-[#8f8f8f]">{{ group.adapters.length }} models</div>
                      <div class="mt-0.5 truncate text-xs text-[#737373]">
                        {{ activeAdapterOf(group) ? `Active: ${activeAdapterOf(group).modelID}` : "Inactive" }}
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
                    <Button variant="default" :disabled="appState.configSaving" @click.stop="expandedGroupID = group.groupID">
                      {{ expandedGroupID === group.groupID ? "Collapse" : "Expand" }}
                    </Button>
                  </template>
                  <template v-else>
                    <Button
                      variant="default"
                      :disabled="appState.configSaving || batchTesting || isAdapterTesting(group.adapters[0])"
                      @click="handleTestModelAdapter(group.adapters[0])"
                    >
                      {{ isAdapterTesting(group.adapters[0]) ? "Testing..." : "Test" }}
                    </Button>
                    <Button variant="default" :disabled="appState.configSaving" @click="openEditor(appState.modelAdapters.indexOf(group.adapters[0]))">Edit</Button>
                    <Button variant="default" :disabled="appState.configSaving" @click="handleDuplicateModelAdapter(appState.modelAdapters.indexOf(group.adapters[0]))">Duplicate</Button>
                    <Button variant="text" :disabled="appState.configSaving"
                      @click="handleDeleteModelAdapter(appState.modelAdapters.indexOf(group.adapters[0]))">Delete</Button>
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
                <div class="mt-0.5 truncate text-xs text-[#737373]">{{ expandedGroup.adapters.length }} models</div>
              </div>
              <Button variant="text" :disabled="appState.configSaving" @click="expandedGroupID = ''">Collapse</Button>
            </div>

            <div class="mb-4">
              <div class="mb-2 flex items-end justify-between gap-3">
                <div>
                  <div class="text-sm font-medium text-white">Active models</div>
                  <div class="mt-0.5 text-xs text-[#737373]">Cursor can see and use these models.</div>
                </div>
                <span class="rounded-full border border-[#1ca35a] bg-[#123322] px-2 py-1 text-[11px] text-[#10AD5D]">
                  {{ expandedGroup.adapters.filter((adapter) => adapter.active).length }} active
                </span>
              </div>
              <div
                v-if="expandedGroup.adapters.filter((adapter) => adapter.active).length === 0"
                class="flex h-[84px] items-center justify-center rounded-[8px] border border-dashed border-[#3a3a3a] bg-[#1f1f1f] px-4 text-center text-xs text-[#737373]"
              >
                No active model. Activate one from the available models below.
              </div>
              <div v-else class="grid gap-3 [grid-template-columns:repeat(auto-fill,minmax(220px,1fr))]">
                <Card
                  v-for="adapter in expandedGroup.adapters.filter((item) => item.active)"
                  :key="adapter.id || `${adapter.baseURL}-${adapter.modelID}-active`"
                >
                  <div class="flex min-h-[138px] flex-col justify-between gap-3">
                    <div>
                      <div class="truncate text-sm font-medium text-white">{{ adapter.displayName }}</div>
                      <div class="mt-1 truncate font-mono text-xs text-[#8f8f8f]">{{ adapter.modelID }}</div>
                    </div>
                    <Switch
                      compact
                      label="Visible to Cursor"
                      enabled-text="Active"
                      disabled-text="Available"
                      :enabled="adapter.active"
                      :disabled="appState.configSaving || batchTesting"
                      @change="handleToggleActive(adapter)"
                    />
                    <div class="center-row flex-wrap justify-end gap-2 border-t border-[#343434] pt-2.5">
                      <Button
                        variant="default"
                        :disabled="appState.configSaving || batchTesting || isAdapterTesting(adapter)"
                        @click="handleTestModelAdapter(adapter)"
                      >
                        {{ isAdapterTesting(adapter) ? "Testing..." : "Test" }}
                      </Button>
                      <Button variant="default" :disabled="appState.configSaving" @click="openEditor(appState.modelAdapters.indexOf(adapter))">Edit</Button>
                      <Button variant="text" :disabled="appState.configSaving" @click="handleDeleteModelAdapter(appState.modelAdapters.indexOf(adapter))">Delete</Button>
                    </div>
                  </div>
                </Card>
              </div>
            </div>

            <div>
              <div class="mb-2 flex items-end justify-between gap-3">
                <div>
                  <div class="text-sm font-medium text-white">Available models</div>
                  <div class="mt-0.5 text-xs text-[#737373]">Configured here, but hidden from Cursor until activated.</div>
                </div>
                <span class="rounded-full border border-[#3f3f3f] px-2 py-1 text-[11px] text-[#8f8f8f]">
                  {{ expandedGroup.adapters.filter((adapter) => !adapter.active).length }} available
                </span>
              </div>
              <div
                v-if="expandedGroup.adapters.filter((adapter) => !adapter.active).length === 0"
                class="flex h-[84px] items-center justify-center rounded-[8px] border border-dashed border-[#3a3a3a] bg-[#1f1f1f] px-4 text-center text-xs text-[#737373]"
              >
                All models for this endpoint are active.
              </div>
              <div v-else class="grid gap-2 [grid-template-columns:repeat(auto-fill,minmax(220px,1fr))]">
                <Card
                  v-for="adapter in expandedGroup.adapters"
                  :key="adapter.id || `${adapter.baseURL}-${adapter.modelID}`"
                  v-show="!adapter.active"
                >
                  <div class="flex flex-col gap-2.5">
                    <div>
                      <div class="truncate text-sm font-medium text-white">{{ adapter.displayName }}</div>
                      <div class="mt-0.5 truncate text-xs text-[#8f8f8f]">{{ adapter.modelID }}</div>
                    </div>
                    <div class="flex items-center justify-between gap-2">
                      <Switch
                        compact
                        label="Cursor visibility"
                        enabled-text="Active"
                        disabled-text="Available"
                        :enabled="adapter.active"
                        :disabled="appState.configSaving || batchTesting"
                        @change="handleToggleActive(adapter)"
                      />
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
                        {{ isAdapterTesting(adapter) ? "Testing..." : "Test" }}
                      </Button>
                      <Button variant="default" :disabled="appState.configSaving" @click="openEditor(appState.modelAdapters.indexOf(adapter))">Edit</Button>
                      <Button variant="text" :disabled="appState.configSaving"
                        @click="handleDeleteModelAdapter(appState.modelAdapters.indexOf(adapter))">Delete</Button>
                    </div>
                  </div>
                </Card>
              </div>
            </div>
          </div>
        </div>
      </template>
    </div>
  </div>
</template>
