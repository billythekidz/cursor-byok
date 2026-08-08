<script setup>
import Button from "@/components/ui/Button.vue";
import Input from "@/components/ui/Input.vue";
import ModelAdapterTestCard from "@/components/ModelAdapterTestCard.vue";
import Select from "@/components/ui/Select.vue";
import Tooltip from "@/components/ui/Tooltip.vue";
import { getModelEditorContext } from "@/services/clientApi";
import {
  ANTHROPIC_THINKING_EFFORT_DEFAULT,
  appState,
  buildOpenAIEndpointGroupKey,
  buildModelAdapterTestRequestHash,
  createEmptyModelAdapter,
  CUSTOM_HEADERS_DEFAULT_JSON,
  EXTRA_PARAMS_DEFAULT_JSON,
  getModelAdapterTestResult,
  getModelAdapterTestResultByID,
  isModelAdapterTestResultStale,
  normalizeModelAdapter,
  OPENAI_ENDPOINT_CHAT_COMPLETIONS,
  OPENAI_ENDPOINT_CUSTOM,
  OPENAI_ENDPOINT_RESPONSES,
  OPENAI_EXTRA_PARAMS_DEFAULT_JSON,
  runModelAdapterTest,
  saveModelAdapterAt,
  toUserError,
  validateModelAdapters,
} from "@/state/appState";
import { Window } from "@wailsio/runtime";
import { computed, onMounted, reactive, ref, watch } from "vue";

const modelTypeTabs = [
  { label: "OpenAI", value: "openai", icon: "icon-[bxl--openai]" },
  { label: "Anthropic", value: "anthropic", icon: "icon-[logos--claude-icon]" },
  { label: "Codex", value: "codex", icon: "icon-[mdi--robot-outline]" },
];

const reasoningEffortOptions = [
  { label: "Low", value: "low", icon: "icon-[mdi--head-outline]" },
  { label: "Medium", value: "medium", icon: "icon-[mdi--head-lightbulb-outline]" },
  { label: "High", value: "high", icon: "icon-[mdi--brain]" },
  { label: "Extra High", value: "xhigh", icon: "icon-[mdi--head-cog-outline]" },
  { label: "Max", value: "max", icon: "icon-[mdi--brain]" },
];

const anthropicThinkingEffortOptions = [
  { label: "Low", value: "low", icon: "icon-[mdi--head-outline]" },
  { label: "Medium", value: "medium", icon: "icon-[mdi--head-lightbulb-outline]" },
  { label: "High", value: "high", icon: "icon-[mdi--brain]" },
  { label: "Extra High", value: "xhigh", icon: "icon-[mdi--head-cog-outline]" },
  { label: "Max", value: "max", icon: "icon-[mdi--brain]" },
];

const openAIEndpointOptions = [
  { label: "/v1/responses", value: OPENAI_ENDPOINT_RESPONSES, icon: "icon-[mdi--api]" },
  { label: "/v1/chat/completions", value: OPENAI_ENDPOINT_CHAT_COMPLETIONS, icon: "icon-[mdi--message-text-outline]" },
  { label: "Custom path (enter full request URL)", value: OPENAI_ENDPOINT_CUSTOM, icon: "icon-[mdi--pencil-outline]" },
];

const editorIndex = ref(-1);
const draft = reactive(createEmptyModelAdapter());
const errorMessage = ref("");
const loading = ref(true);
const lastTestAdapterID = ref("");
const localTestFailure = ref("");
const selectedOpenAIEndpointGroupID = ref("");

function createOptionalPositiveIntegerModel(key) {
  return computed({
    get() {
      return draft[key] > 0 ? String(draft[key]) : "";
    },
    set(value) {
      const text = String(value || "").trim();
      draft[key] = /^\d+$/.test(text) && Number(text) > 0 ? Number(text) : 0;
    },
  });
}

const maxCompletionTokensInput = createOptionalPositiveIntegerModel("maxCompletionTokens");
const anthropicMaxTokensInput = createOptionalPositiveIntegerModel("anthropicMaxTokens");
const contextWindowTokensInput = createOptionalPositiveIntegerModel("contextWindowTokens");
const interfacePlaceholder = computed(() =>
  draft.type === "anthropic" ? "e.g., https://api.anthropic.com" : "e.g., https://api.openai.com/v1",
);
const currentRequestHash = computed(() => buildModelAdapterTestRequestHash(draft));
const directModelTestResult = computed(() => getModelAdapterTestResult(draft));
const rememberedModelTestResult = computed(() =>
  lastTestAdapterID.value ? getModelAdapterTestResultByID(lastTestAdapterID.value) : null,
);
const activeModelTestResult = computed(() => directModelTestResult.value || rememberedModelTestResult.value);
const modelTestResultStale = computed(() =>
  isModelAdapterTestResultStale(draft, activeModelTestResult.value),
);
const isCurrentConfigTesting = computed(() => directModelTestResult.value?.status === "running");
const modelTestSummary = computed(() => {
  if (localTestFailure.value) {
    return localTestFailure.value;
  }
  return activeModelTestResult.value?.summaryText || "Not tested";
});

const title = computed(() => (editorIndex.value >= 0 ? "Edit Model Configuration" : "Add Model Configuration"));
const isAddingOpenAIModel = computed(() => editorIndex.value < 0 && draft.type === "openai");

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

function formatEndpointHost(value) {
  const text = String(value || "").trim();
  if (!text) {
    return "-";
  }
  try {
    return new URL(text).host || text;
  } catch {
    return text.replace(/^https?:\/\//, "");
  }
}

const configuredOpenAIEndpointOptions = computed(() => {
  const seen = new Set();
  const options = [];
  for (const rawAdapter of appState.modelAdapters) {
    const adapter = normalizeModelAdapter(rawAdapter);
    if (adapter.type !== "openai" || !adapter.baseURL || !adapter.apiKey) {
      continue;
    }
    const groupID = buildOpenAIEndpointGroupKey(adapter.baseURL, adapter.apiKey);
    if (seen.has(groupID)) {
      continue;
    }
    seen.add(groupID);
    options.push({
      value: groupID,
      label: `${formatEndpointHost(adapter.baseURL)} · ${maskSecret(adapter.apiKey)}`,
      icon: "icon-[mdi--server-network]",
      baseURL: adapter.baseURL,
      apiKey: adapter.apiKey,
      openAIEndpoint: adapter.openAIEndpoint || OPENAI_ENDPOINT_RESPONSES,
    });
  }
  return options;
});

function applySelectedOpenAIEndpoint(groupID) {
  const endpoint = configuredOpenAIEndpointOptions.value.find((option) => option.value === groupID);
  if (!endpoint) {
    draft.baseURL = "";
    draft.apiKey = "";
    draft.openAIEndpointGroupID = "";
    return;
  }
  draft.baseURL = endpoint.baseURL;
  draft.apiKey = endpoint.apiKey;
  draft.openAIEndpoint = endpoint.openAIEndpoint;
  draft.openAIEndpointGroupID = endpoint.value;
}

function initializeNewOpenAIEndpoint() {
  if (!isAddingOpenAIModel.value) {
    return;
  }
  const currentGroupID = draft.baseURL && draft.apiKey
    ? buildOpenAIEndpointGroupKey(draft.baseURL, draft.apiKey)
    : "";
  const selected = configuredOpenAIEndpointOptions.value.some((option) => option.value === currentGroupID)
    ? currentGroupID
    : configuredOpenAIEndpointOptions.value[0]?.value || "";
  selectedOpenAIEndpointGroupID.value = selected;
  applySelectedOpenAIEndpoint(selected);
}

function ensureOpenAIExtraParamsJSON() {
  if (!String(draft.openAIExtraParamsJSON || "").trim()) {
    draft.openAIExtraParamsJSON = OPENAI_EXTRA_PARAMS_DEFAULT_JSON;
  }
}

function ensureCustomHeadersJSON() {
  if (!String(draft.customHeadersJSON || "").trim()) {
    draft.customHeadersJSON = CUSTOM_HEADERS_DEFAULT_JSON;
  }
}

function ensureAnthropicExtraParamsJSON() {
  if (!String(draft.anthropicExtraParamsJSON || "").trim()) {
    draft.anthropicExtraParamsJSON = EXTRA_PARAMS_DEFAULT_JSON;
  }
}

function ensureAnthropicThinkingEffort() {
  if (!String(draft.anthropicThinkingEffort || "").trim()) {
    draft.anthropicThinkingEffort = ANTHROPIC_THINKING_EFFORT_DEFAULT;
  }
}

const fieldTips = {
  displayName: "Display name for UI identification.",
  modelID: "The actual model name sent to the server, e.g. gpt-4.1 or claude-sonnet.",
  baseURL: "Base API URL for the model service, typically compatible with OpenAI or Anthropic.",
  apiKey: "API key used to call the model service.",
  contextWindowTokens: "Maximum context tokens accepted by the model. Leave empty for default.",
  reasoningEffort: "Reasoning effort only applies to models supporting reasoning_effort. Higher value is more stable but may be slower.",
  maxCompletionTokens: "Maximum completion tokens generated per response. Leave empty for default.",
  openAIEndpoint: "Select protocol endpoint. When 'Custom path' is selected, please enter full request URL.",
  openAIExtraParams: "When enabled, overrides the OpenAI request body with this JSON object.",
  customHeaders: "When enabled, overrides final request headers with this JSON object. Values must be strings.",
  anthropicExtraParams: "When enabled, overrides the Anthropic request body with this JSON object.",
  anthropicMaxTokens: "Maximum completion tokens generated per response for Anthropic models.",
  anthropicThinkingEffort: "Anthropic adaptive thinking effort.",
  tooltipData: "Notes displayed when hovering over the model in the list.",
};

async function loadContext() {
  try {
    const ctx = await getModelEditorContext();
    editorIndex.value = typeof ctx.index === "number" ? ctx.index : -1;
    const parsed = JSON.parse(ctx.adapterJSON || "{}");
    Object.assign(draft, normalizeModelAdapter(parsed));
    if (!draft.type) {
      draft.type = "openai";
    }
    initializeNewOpenAIEndpoint();
  } catch (_error) {
    Object.assign(draft, createEmptyModelAdapter());
    draft.type = "openai";
    initializeNewOpenAIEndpoint();
  } finally {
    loading.value = false;
  }
}

async function persistDraft() {
  const adapter = normalizeModelAdapter(draft);
	if (
		adapter.type === "codex" &&
		adapter.active &&
		(!appState.codexRuntime.installed || !appState.codexRuntime.authenticated)
	) {
		adapter.active = false;
		draft.active = false;
	}

  const singleCheck = validateModelAdapters([adapter]);
  if (singleCheck) {
    errorMessage.value = singleCheck;
    return { ok: false, error: singleCheck, adapter: null };
  }

  const result = await saveModelAdapterAt(editorIndex.value, adapter);
  if (!result.ok) {
    errorMessage.value = result.error;
    return { ok: false, error: result.error, adapter: null };
  }

  if (typeof result.index === "number") {
    editorIndex.value = result.index;
  }
  if (result.adapter) {
    Object.assign(draft, normalizeModelAdapter(result.adapter));
  }
  errorMessage.value = "";
  return {
    ok: true,
    error: "",
    adapter: result.adapter ? normalizeModelAdapter(result.adapter) : normalizeModelAdapter(draft),
  };
}

async function handleSave() {
  const result = await persistDraft();
  if (!result.ok) {
    return;
  }
  await Window.Close();
}

async function handleCancel() {
  await Window.Close();
}

function handleModelTypeChange(type) {
  draft.type = type;
  if (type === "openai" && !draft.openAIEndpoint) {
    draft.openAIEndpoint = OPENAI_ENDPOINT_RESPONSES;
  } else if (type === "anthropic") {
    ensureAnthropicThinkingEffort();
  }
  initializeNewOpenAIEndpoint();
}

async function handleTest() {
  localTestFailure.value = "";
  try {
    const saved = await persistDraft();
    if (!saved.ok || !saved.adapter) {
      return;
    }
    const result = await runModelAdapterTest(saved.adapter);
    if (result?.adapterID) {
      lastTestAdapterID.value = result.adapterID;
    }
  } catch (error) {
    const latest = getModelAdapterTestResult(draft);
    if (latest?.adapterID) {
      lastTestAdapterID.value = latest.adapterID;
      return;
    }
    localTestFailure.value = toUserError(error);
  }
}

watch(
  directModelTestResult,
  (result) => {
    if (!result?.adapterID) {
      return;
    }
    lastTestAdapterID.value = result.adapterID;
    if (result.status !== "running") {
      localTestFailure.value = "";
    }
  },
  { immediate: true },
);

watch(currentRequestHash, () => {
  localTestFailure.value = "";
});

watch(
  () => draft.openAIExtraParamsEnabled,
  (enabled) => {
    if (enabled) {
      ensureOpenAIExtraParamsJSON();
    }
  },
);

watch(
  () => draft.customHeadersEnabled,
  (enabled) => {
    if (enabled) {
      ensureCustomHeadersJSON();
    }
  },
);

watch(
  () => draft.anthropicExtraParamsEnabled,
  (enabled) => {
    if (enabled) {
      ensureAnthropicExtraParamsJSON();
    }
  },
);

watch(
  configuredOpenAIEndpointOptions,
  () => {
    initializeNewOpenAIEndpoint();
  },
  { deep: true },
);

onMounted(async () => {
  await loadContext();
});
</script>

<template>
  <div class="flex h-full flex-col text-[#e5e5e5]">
    <div class="flex shrink-0 items-center justify-between px-4 pb-2">
      <h2 class="text-base font-medium text-white">{{ title }}</h2>
      <div class="flex items-center gap-2">
        <Button variant="default" @click="handleCancel">Cancel</Button>
        <Button variant="default" :disabled="isCurrentConfigTesting || appState.configSaving || (isAddingOpenAIModel && !selectedOpenAIEndpointGroupID)" @click="handleTest">
          {{ isCurrentConfigTesting ? "Testing..." : "Save & Test" }}
        </Button>
        <Button variant="primary" :disabled="appState.configSaving || (isAddingOpenAIModel && !selectedOpenAIEndpointGroupID)" @click="handleSave">
          {{ appState.configSaving ? "Saving..." : "Save" }}
        </Button>
      </div>
    </div>

    <div v-if="loading" class="flex flex-1 items-center justify-center text-sm text-[#a3a3a3]">
      Loading...
    </div>

    <div v-else class="flex-1 overflow-y-auto min-h-0 px-4 pb-4">
      <div class="flex flex-col gap-4">
        <div class="center-row gap-2">
          <button
            v-for="tab in modelTypeTabs"
            :key="tab.value"
            type="button"
            class="center-row gap-2 rounded-[8px] border px-3 py-2 text-sm transition-colors duration-150"
            :class="draft.type === tab.value
              ? 'border-[#1ca35a] bg-[#123322] text-white'
              : 'border-[#343434] bg-[#252525] text-[#a3a3a3] hover:border-[#4a4a4a] hover:text-[#e5e5e5]'"
            @click="handleModelTypeChange(tab.value)"
          >
            <span :class="[tab.icon, 'text-[16px]']"></span>
            <span>{{ tab.label }}</span>
          </button>
        </div>

        <div class="grid grid-cols-1 gap-3 md:grid-cols-2">
          <label class="flex flex-col gap-1">
            <span class="center-row justify-start gap-1.5 text-sm text-[#d4d4d4]">
              <Tooltip :content="fieldTips.displayName" />
              <span>Display Name</span>
            </span>
            <input
              v-model="draft.displayName"
              type="text"
              placeholder="e.g., OpenAI - GPT-4.1"
              class="h-9 rounded-[6px] border border-[#3f3f3f] bg-[#232323] px-3 text-sm text-[#e5e5e5] outline-none focus:border-[#10AD5D]"
            />
          </label>

          <label class="flex flex-col gap-1">
            <span class="center-row justify-start gap-1.5 text-sm text-[#d4d4d4]">
              <Tooltip :content="fieldTips.modelID" />
              <span>Model ID</span>
            </span>
            <input
              v-model="draft.modelID"
              type="text"
              placeholder="e.g., gpt-4.1"
              class="h-9 rounded-[6px] border border-[#3f3f3f] bg-[#232323] px-3 text-sm text-[#e5e5e5] outline-none focus:border-[#10AD5D]"
            />
          </label>

          <label v-if="draft.type !== 'codex' && !isAddingOpenAIModel" class="flex flex-col gap-1">
            <span class="center-row justify-start gap-1.5 text-sm text-[#d4d4d4]">
              <Tooltip :content="fieldTips.apiKey" />
              <span>API Key</span>
            </span>
            <Input
              v-model="draft.apiKey"
              type="password"
              allow-visibility-toggle
              placeholder="e.g., sk-xxxxxx"
              autocomplete="off"
            />
          </label>

          <label v-if="isAddingOpenAIModel" class="flex flex-col gap-1">
            <span class="text-sm text-[#d4d4d4]">Endpoint</span>
            <Select
              v-model="selectedOpenAIEndpointGroupID"
              :options="configuredOpenAIEndpointOptions"
              :disabled="configuredOpenAIEndpointOptions.length === 0"
              placeholder="Select configured endpoint"
              @change="applySelectedOpenAIEndpoint"
            />
            <span v-if="configuredOpenAIEndpointOptions.length === 0" class="text-xs text-[#e0a458]">No configured endpoint. Add an endpoint first.</span>
          </label>

          <label v-if="draft.type !== 'codex' && !isAddingOpenAIModel" class="flex flex-col gap-1">
            <span class="center-row justify-start gap-1.5 text-sm text-[#d4d4d4]">
              <Tooltip :content="fieldTips.baseURL" />
              <span>Base URL</span>
            </span>
            <input
              v-model="draft.baseURL"
              type="text"
              :placeholder="interfacePlaceholder"
              class="h-9 rounded-[6px] border border-[#3f3f3f] bg-[#232323] px-3 text-sm text-[#e5e5e5] outline-none focus:border-[#10AD5D]"
            />
          </label>

          <label class="flex flex-col gap-1">
            <span class="center-row justify-start gap-1.5 text-sm text-[#d4d4d4]">
              <Tooltip :content="fieldTips.contextWindowTokens" />
              <span>Context Window</span>
            </span>
            <input
              v-model="contextWindowTokensInput"
              type="text"
              inputmode="numeric"
              placeholder="e.g., 256000 (leave blank for default)"
              class="h-9 rounded-[6px] border border-[#3f3f3f] bg-[#232323] px-3 text-sm text-[#e5e5e5] outline-none focus:border-[#10AD5D]"
            />
          </label>

          <label v-if="draft.type === 'openai' || draft.type === 'codex'" class="flex flex-col gap-1">
            <span class="center-row justify-start gap-1.5 text-sm text-[#d4d4d4]">
              <Tooltip :content="fieldTips.reasoningEffort" />
              <span>Reasoning Effort</span>
            </span>
            <Select
              v-model="draft.reasoningEffort"
              :options="reasoningEffortOptions"
            />
          </label>

          <label v-if="draft.type === 'anthropic'" class="flex flex-col gap-1">
            <span class="center-row justify-start gap-1.5 text-sm text-[#d4d4d4]">
              <Tooltip :content="fieldTips.anthropicMaxTokens" />
              <span>Max Completion Tokens</span>
            </span>
            <input
              v-model="anthropicMaxTokensInput"
              type="text"
              inputmode="numeric"
              placeholder="e.g., 65536 (leave blank for default)"
              class="h-9 rounded-[6px] border border-[#3f3f3f] bg-[#232323] px-3 text-sm text-[#e5e5e5] outline-none focus:border-[#10AD5D]"
            />
          </label>

          <label v-if="draft.type === 'anthropic'" class="flex flex-col gap-1">
            <span class="center-row justify-start gap-1.5 text-sm text-[#d4d4d4]">
              <Tooltip :content="fieldTips.anthropicThinkingEffort" />
              <span>Thinking Effort</span>
            </span>
            <Select
              v-model="draft.anthropicThinkingEffort"
              :options="anthropicThinkingEffortOptions"
            />
          </label>

        </div>

        <div v-if="draft.type === 'openai'" class="grid grid-cols-1 gap-3 md:grid-cols-2">
          <label class="flex flex-col gap-1">
            <span class="center-row justify-start gap-1.5 text-sm text-[#d4d4d4]">
              <Tooltip :content="fieldTips.maxCompletionTokens" />
              <span>Max Completion Tokens</span>
            </span>
            <input
              v-model="maxCompletionTokensInput"
              type="text"
              inputmode="numeric"
              placeholder="e.g., 65536 (leave blank for default)"
              class="h-9 rounded-[6px] border border-[#3f3f3f] bg-[#232323] px-3 text-sm text-[#e5e5e5] outline-none focus:border-[#10AD5D]"
            />
          </label>

          <label class="flex flex-col gap-1">
            <span class="center-row justify-start gap-1.5 text-sm text-[#d4d4d4]">
              <Tooltip :content="fieldTips.openAIEndpoint" />
              <span>Protocol Endpoint</span>
            </span>
            <Select
              v-model="draft.openAIEndpoint"
              :options="openAIEndpointOptions"
            />
          </label>
        </div>

        <div v-if="draft.type === 'openai'" class="rounded-[8px] border border-[#343434] bg-[#252525] p-3">
          <div class="flex items-center justify-between gap-3">
            <span class="center-row justify-start gap-1.5 text-sm text-[#d4d4d4]">
              <Tooltip :content="fieldTips.openAIExtraParams" />
              <span>Extra Params JSON</span>
            </span>
            <label class="center-row gap-2 text-xs text-[#d4d4d4]">
              <input
                v-model="draft.openAIExtraParamsEnabled"
                type="checkbox"
                class="size-4 accent-[#10AD5D]"
              />
              <span>Enable</span>
            </label>
          </div>
          <textarea
            v-if="draft.openAIExtraParamsEnabled"
            v-model="draft.openAIExtraParamsJSON"
            rows="5"
            spellcheck="false"
            class="mt-3 min-h-[120px] w-full resize-none rounded-[6px] border border-[#3f3f3f] bg-[#1f1f1f] px-3 py-2 font-mono text-xs text-[#e5e5e5] outline-none focus:border-[#10AD5D]"
          />
        </div>

        <div v-if="draft.type === 'anthropic'" class="rounded-[8px] border border-[#343434] bg-[#252525] p-3">
          <div class="flex items-center justify-between gap-3">
            <span class="center-row justify-start gap-1.5 text-sm text-[#d4d4d4]">
              <Tooltip :content="fieldTips.anthropicExtraParams" />
              <span>Anthropic Extra Params JSON</span>
            </span>
            <label class="center-row gap-2 text-xs text-[#d4d4d4]">
              <input
                v-model="draft.anthropicExtraParamsEnabled"
                type="checkbox"
                class="size-4 accent-[#10AD5D]"
              />
              <span>Enable</span>
            </label>
          </div>
          <textarea
            v-if="draft.anthropicExtraParamsEnabled"
            v-model="draft.anthropicExtraParamsJSON"
            rows="5"
            spellcheck="false"
            class="mt-3 min-h-[120px] w-full resize-none rounded-[6px] border border-[#3f3f3f] bg-[#1f1f1f] px-3 py-2 font-mono text-xs text-[#e5e5e5] outline-none focus:border-[#10AD5D]"
          />
        </div>

        <div v-if="draft.type === 'codex'" class="rounded-[8px] border border-[#6b5428] bg-[#2a2418] p-3 text-sm text-[#e0c58a]">
			<div class="font-medium text-[#f0d59a]">Codex 运行环境</div>
			<div class="mt-1">Codex CLI 管理 ChatGPT 登录。Shell 和文件工具将在选定工作区运行，审批策略为 <code>never</code>。</div>
			<div v-if="!appState.codexRuntime.installed || !appState.codexRuntime.authenticated" class="mt-2">可以保存为草稿，但激活前必须安装并登录 Codex。</div>
		</div>

        <div v-if="draft.type !== 'codex'" class="rounded-[8px] border border-[#343434] bg-[#252525] p-3">
          <div class="flex items-center justify-between gap-3">
            <span class="center-row justify-start gap-1.5 text-sm text-[#d4d4d4]">
              <Tooltip :content="fieldTips.customHeaders" />
              <span>Custom Headers JSON</span>
            </span>
            <label class="center-row gap-2 text-xs text-[#d4d4d4]">
              <input
                v-model="draft.customHeadersEnabled"
                type="checkbox"
                class="size-4 accent-[#10AD5D]"
              />
              <span>Enable</span>
            </label>
          </div>
          <textarea
            v-if="draft.customHeadersEnabled"
            v-model="draft.customHeadersJSON"
            rows="5"
            spellcheck="false"
            class="mt-3 min-h-[120px] w-full resize-none rounded-[6px] border border-[#3f3f3f] bg-[#1f1f1f] px-3 py-2 font-mono text-xs text-[#e5e5e5] outline-none focus:border-[#10AD5D]"
          />
        </div>

        <label class="flex flex-col gap-1">
          <span class="center-row justify-start gap-1.5 text-sm text-[#d4d4d4]">
            <Tooltip :content="fieldTips.tooltipData" />
            <span>Notes</span>
          </span>
          <textarea
            v-model="draft.tooltipData"
            rows="3"
            placeholder="e.g., Used for daily coding completion and Q&A"
            class="min-h-[96px] resize-none rounded-[6px] border border-[#3f3f3f] bg-[#232323] px-3 py-2 text-sm text-[#e5e5e5] outline-none focus:border-[#10AD5D]"
          />
        </label>

        <ModelAdapterTestCard
          :result="localTestFailure ? { status: 'error', error: 'Test failed', summaryText: 'Test failed', rawResponse: modelTestSummary } : activeModelTestResult"
          :stale="modelTestResultStale"
          :show-metrics="true"
        />

        <div
          v-if="errorMessage"
          class="rounded-[8px] border border-[#4b1d1d] bg-[#2a1313] px-3 py-2 text-sm text-[#fca5a5]"
        >
          {{ errorMessage }}
        </div>
      </div>
    </div>
  </div>
</template>
