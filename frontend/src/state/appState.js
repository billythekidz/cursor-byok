import { computed, reactive, watchSyncEffect } from "vue";
import { Events } from "@wailsio/runtime";
import dayjs from "dayjs";
import {
  checkForUpdates,
  getAppVersion,
  getHomeMetricsSummary,
  getModelAdapterTestResults,
  installReadyUpdate,
  getProxyState,
  openConfigWindow as openConfig,
  loadUserConfig,
  openLogsDirectory,
  openModelConfig,
  openModelEditor,
  saveUserConfig,
  scanOpenAIModels,
  startProxyService,
  stopProxyService,
  testModelAdapter,
} from "@/services/clientApi";

const APP_STATE_STORAGE_KEY = "cursor-client:runtime-state:v2";
const GENERIC_SERVICE_ERROR = "Service error";
const SUPPORTED_MODEL_ADAPTER_TYPES = new Set(["openai", "anthropic"]);
const SUPPORTED_REASONING_EFFORTS = new Set(["low", "medium", "high", "xhigh", "max"]);
const SUPPORTED_ANTHROPIC_THINKING_EFFORTS = new Set(["low", "medium", "high", "xhigh", "max"]);
export const ANTHROPIC_THINKING_EFFORT_DEFAULT = "xhigh";
export const OPENAI_ENDPOINT_RESPONSES = "/v1/responses";
export const OPENAI_ENDPOINT_CHAT_COMPLETIONS = "/v1/chat/completions";
export const OPENAI_ENDPOINT_CUSTOM = "/custom";
export const OPENAI_EXTRA_PARAMS_DEFAULT_JSON = `{
  "service_tier": "priority"
}`;
export const EXTRA_PARAMS_DEFAULT_JSON = `{
}`;
export const CUSTOM_HEADERS_DEFAULT_JSON = `{
}`;
const SUPPORTED_OPENAI_ENDPOINTS = new Set([OPENAI_ENDPOINT_RESPONSES, OPENAI_ENDPOINT_CHAT_COMPLETIONS, OPENAI_ENDPOINT_CUSTOM]);
const SUPPORTED_ROUTE_MODES = new Set(["local", "upstream"]);
const PROXY_STATE_EVENT = "proxy:state";
const USER_CONFIG_CHANGED_EVENT = "user-config:changed";
const UPDATE_STATE_EVENT = "update:state";
const UPDATE_PROGRESS_EVENT = "update:progress";
const UPDATE_READY_EVENT = "update:ready";
const UPDATE_ERROR_EVENT = "update:error";
const MODEL_ADAPTER_TEST_UPDATED_EVENT = "model-adapter-test:updated";
const SUPPORTED_MODEL_ADAPTER_TEST_STATUSES = new Set(["idle", "running", "success", "error"]);
const HOME_METRICS_MIN_LOADING_MS = 600;

export const ROUTE_MODE_OPTIONS = [
  { label: "Local Service Mode", value: "local" },
  { label: "Direct Cursor Mode", value: "upstream" },
];

function asString(value) {
  if (typeof value === "string") {
    return value.trim();
  }
  if (value instanceof String) {
    return value.toString().trim();
  }
  if (typeof value === "number" || typeof value === "boolean") {
    return String(value);
  }
  return "";
}

function asBoolean(value, fallback = false) {
  if (typeof value === "boolean") {
    return value;
  }
  if (typeof value === "number") {
    return value !== 0;
  }
  const normalized = asString(value).toLowerCase();
  if (!normalized) {
    return fallback;
  }
  return normalized === "true" || normalized === "1" || normalized === "yes";
}

function asArray(value) {
  return Array.isArray(value) ? value : [];
}

function asPositiveIntegerString(value) {
  const text = asString(value);
  if (!text) {
    return "";
  }
  if (!/^\d+$/.test(text)) {
    return "";
  }
  return Number(text) > 0 ? text : "";
}

function asPositiveInteger(value) {
  const text = asPositiveIntegerString(value);
  if (!text) {
    return 0;
  }
  return Number(text);
}

function asNumber(value, fallback = 0) {
  if (typeof value === "number" && Number.isFinite(value)) {
    return value;
  }
  const text = asString(value);
  if (!text) {
    return fallback;
  }
  const parsed = Number(text);
  return Number.isFinite(parsed) ? parsed : fallback;
}

function formatReleaseDate(value) {
  const text = asString(value);
  if (!text) {
    return "Unknown";
  }
  const parsed = dayjs(text);
  if (!parsed.isValid()) {
    return text;
  }
  return parsed.format("YYYY-MM-DD HH:mm");
}

function normalizeRouteMode(value, fallback = "local") {
  const text = asString(value).toLowerCase();
  if (SUPPORTED_ROUTE_MODES.has(text)) {
    return text;
  }
  return fallback;
}

function normalizeBaseURL(value) {
  const text = asString(value);
  if (!text) {
    return "";
  }
  try {
    const parsed = new URL(text);
    if (parsed.protocol !== "http:" && parsed.protocol !== "https:") {
      return "";
    }
    parsed.protocol = parsed.protocol.toLowerCase();
    parsed.hostname = parsed.hostname.toLowerCase();
    const normalized = parsed.toString().replace(/\/+$/, "");
    return normalized || parsed.toString();
  } catch (_error) {
    return text;
  }
}

function buildModelAdapterIdentityKey(adapter) {
  return [
    normalizeBaseURL(adapter.baseURL),
    asString(adapter.modelID),
    asString(adapter.apiKey),
    asString(adapter.displayName),
    adapter.type === "openai" ? normalizeOpenAIEndpoint(adapter.openAIEndpoint) : "",
  ].join("\n");
}

function hashStringFNV32a(value) {
  let hash = 0x811c9dc5;
  for (let index = 0; index < value.length; index += 1) {
    hash ^= value.charCodeAt(index);
    hash = Math.imul(hash, 0x01000193) >>> 0;
  }
  return hash.toString(16).padStart(8, "0");
}

// buildOpenAIEndpointGroupKey generates an OpenAI endpoint group key (FNV32a of baseURL\napiKey).
// Keep the grouping semantics aligned with the Go side: the same baseURL + apiKey maps to the same endpoint group.
export function buildOpenAIEndpointGroupKey(baseURL, apiKey) {
  return hashStringFNV32a([normalizeBaseURL(baseURL), asString(apiKey)].join("\n"));
}

export function buildModelAdapterTestRequestHash(source) {  const adapter = normalizeModelAdapter(source);
  return hashStringFNV32a([
    asString(adapter.type),
    normalizeBaseURL(adapter.baseURL),
    asString(adapter.apiKey),
    asString(adapter.modelID),
    adapter.type === "openai" ? asString(adapter.reasoningEffort || "medium") : "",
    adapter.type === "openai" ? normalizeOpenAIEndpoint(adapter.openAIEndpoint) : "",
    adapter.type === "openai" ? String(Boolean(adapter.openAIExtraParamsEnabled)) : "false",
    adapter.type === "openai" && adapter.openAIExtraParamsEnabled ? asString(adapter.openAIExtraParamsJSON) : "",
    String(Boolean(adapter.customHeadersEnabled)),
    adapter.customHeadersEnabled ? asString(adapter.customHeadersJSON) : "",
    adapter.type === "anthropic" ? String(Boolean(adapter.anthropicExtraParamsEnabled)) : "false",
    adapter.type === "anthropic" && adapter.anthropicExtraParamsEnabled ? asString(adapter.anthropicExtraParamsJSON) : "",
    String(asPositiveInteger(adapter.contextWindowTokens)),
    String(asPositiveInteger(adapter.maxCompletionTokens)),
    String(asPositiveInteger(adapter.anthropicMaxTokens)),
    adapter.type === "anthropic" ? asString(adapter.anthropicThinkingEffort || ANTHROPIC_THINKING_EFFORT_DEFAULT) : "",
  ].join("\n"));
}

export function formatDuration(value) {
  const durationMS = Math.max(0, Math.round(asNumber(value)));
  if (durationMS < 1000) {
    return `${durationMS} ms`;
  }
  return `${(durationMS / 1000).toFixed(1)} s`;
}

function normalizeModelAdapterTestStatus(value) {
  const text = asString(value).toLowerCase();
  return SUPPORTED_MODEL_ADAPTER_TEST_STATUSES.has(text) ? text : "idle";
}

export function formatModelAdapterTestSummary(source) {
  const result = source && typeof source === "object" ? source : {};
  const status = normalizeModelAdapterTestStatus(result.status);
  if (status === "running") {
    return "Testing...";
  }
  if (status === "error") {
    return asString(result.error) || "Model test failed";
  }
  if (status !== "success") {
    return "";
  }
  const roundedTPS = Math.max(0, Math.round(asNumber(result.tokensPerSecond)));
  return `${roundedTPS} t/s | First Token ${formatDuration(result.firstTextTokenMS)}`;
}

function normalizeModelAdapterTestResult(source) {
  const raw = source && typeof source === "object" ? source : {};
  const status = normalizeModelAdapterTestStatus(raw.status);
  const normalized = {
    adapterID: asString(raw.adapterID),
    requestHash: asString(raw.requestHash),
    status,
    tokensPerSecond: Math.max(0, asNumber(raw.tokensPerSecond)),
    firstTextTokenMS: Math.max(0, Math.round(asNumber(raw.firstTextTokenMS))),
    totalDurationMS: Math.max(0, Math.round(asNumber(raw.totalDurationMS))),
    outputTokens: Math.max(0, Math.round(asNumber(raw.outputTokens))),
    tokensEstimated: asBoolean(raw.tokensEstimated),
    summaryText: asString(raw.summaryText),
    error: asString(raw.error),
    rawResponse: asString(raw.rawResponse),
    testedAt: asString(raw.testedAt),
  };
  if (!normalized.summaryText) {
    normalized.summaryText = formatModelAdapterTestSummary(normalized);
  }
  if (status === "error" && !normalized.summaryText) {
    normalized.summaryText = normalized.error || "Model test failed";
  }
  return normalized;
}

function normalizeModelAdapterTestResults(source) {
  const raw = source && typeof source === "object" && !Array.isArray(source)
    ? source.results
    : source;
  return asArray(raw)
    .map((item) => normalizeModelAdapterTestResult(item))
    .filter((item) => item.adapterID);
}

export function createEmptyModelAdapter() {
  return {
    id: "",
    displayName: "",
    type: "openai",
    baseURL: "",
    apiKey: "",
    tooltipData: "Notes",
    modelID: "",
    reasoningEffort: "medium",
    openAIEndpoint: OPENAI_ENDPOINT_RESPONSES,
    openAIExtraParamsEnabled: false,
    openAIExtraParamsJSON: OPENAI_EXTRA_PARAMS_DEFAULT_JSON,
    customHeadersEnabled: false,
    customHeadersJSON: CUSTOM_HEADERS_DEFAULT_JSON,
    anthropicExtraParamsEnabled: false,
    anthropicExtraParamsJSON: EXTRA_PARAMS_DEFAULT_JSON,
    contextWindowTokens: 0,
    maxCompletionTokens: 0,
    anthropicMaxTokens: 0,
    anthropicThinkingEffort: ANTHROPIC_THINKING_EFFORT_DEFAULT,
    thinkingBudgetTokens: 0,
    openAIEndpointGroupID: "",
    active: false,
  };
}

// normalizeOpenAIEndpoint normalizes the endpoint path.
// Supports three presets: /v1/responses, /v1/chat/completions, and /custom (custom path).
// When /custom is chosen, the user must fill in the full request URL in the endpoint address field.
function normalizeOpenAIEndpoint(value) {
  const text = asString(value).toLowerCase();
  if (!text) {
    return OPENAI_ENDPOINT_RESPONSES;
  }
  return SUPPORTED_OPENAI_ENDPOINTS.has(text) ? text : "";
}

function isValidOpenAIEndpoint(value) {
  return normalizeOpenAIEndpoint(value) !== "";
}

function validateJSONObject(value, label) {
  const text = asString(value);
  if (!text) {
    return `${label} cannot be empty`;
  }
  try {
    const parsed = JSON.parse(text);
    if (!parsed || typeof parsed !== "object" || Array.isArray(parsed)) {
      return `${label} must be a JSON object`;
    }
  } catch (_error) {
    return `${label} must be a valid JSON object`;
  }
  return "";
}

function validateHeadersJSON(value) {
  const objectError = validateJSONObject(value, "Custom Headers JSON");
  if (objectError) {
    return objectError;
  }
  const parsed = JSON.parse(asString(value));
  for (const [key, item] of Object.entries(parsed)) {
    if (!asString(key)) {
      return "Custom header name cannot be empty";
    }
    if (typeof item !== "string") {
      return `Custom header ${key} value must be a string`;
    }
  }
  return "";
}

function validateOpenAIExtraParamsJSON(value) {
  return validateJSONObject(value, "Extra Params JSON");
}

function validateAnthropicExtraParamsJSON(value) {
  return validateJSONObject(value, "Anthropic Extra Params JSON");
}

export function normalizeModelAdapter(source) {
  const raw = source && typeof source === "object" ? source : {};
  const normalizedType = asString(raw.type).toLowerCase();
  const normalizedReasoningEffort = asString(raw.reasoningEffort || raw.reasoning_effort).toLowerCase();
  const normalizedAnthropicThinkingEffort = asString(
    raw.anthropicThinkingEffort
      ?? raw.anthropic_thinking_effort
      ?? raw.outputConfigEffort
      ?? raw.output_config_effort,
  ).toLowerCase();
  const normalizedOpenAIEndpoint = normalizeOpenAIEndpoint(
    raw.openAIEndpoint ?? raw.openaiEndpoint ?? raw.open_ai_endpoint ?? raw.endpoint,
  );
  const openAIExtraParamsEnabled = normalizedType === "openai"
    ? asBoolean(raw.openAIExtraParamsEnabled ?? raw.openaiExtraParamsEnabled ?? raw.open_ai_extra_params_enabled)
    : false;
  const openAIExtraParamsJSON = normalizedType === "openai"
    ? asString(raw.openAIExtraParamsJSON ?? raw.openaiExtraParamsJSON ?? raw.open_ai_extra_params_json) || OPENAI_EXTRA_PARAMS_DEFAULT_JSON
    : "";
  const customHeadersEnabled = asBoolean(raw.customHeadersEnabled ?? raw.custom_headers_enabled);
  const customHeadersJSON = asString(raw.customHeadersJSON ?? raw.custom_headers_json) || CUSTOM_HEADERS_DEFAULT_JSON;
  const anthropicExtraParamsEnabled = normalizedType === "anthropic"
    ? asBoolean(raw.anthropicExtraParamsEnabled ?? raw.anthropic_extra_params_enabled)
    : false;
  const anthropicExtraParamsJSON = normalizedType === "anthropic"
    ? asString(raw.anthropicExtraParamsJSON ?? raw.anthropic_extra_params_json) || EXTRA_PARAMS_DEFAULT_JSON
    : "";
  return {
    id: asString(raw.id),
    displayName: asString(raw.displayName || raw.name),
    type: SUPPORTED_MODEL_ADAPTER_TYPES.has(normalizedType) ? normalizedType : "",
    baseURL: normalizeBaseURL(raw.baseURL || raw.url),
    apiKey: asString(raw.apiKey || raw.key),
    tooltipData: asString(raw.tooltipData),
    modelID: asString(raw.modelID),
    reasoningEffort: SUPPORTED_REASONING_EFFORTS.has(normalizedReasoningEffort)
      ? normalizedReasoningEffort
      : "medium",
    openAIEndpoint: normalizedType === "openai" ? normalizedOpenAIEndpoint : "",
    openAIExtraParamsEnabled,
    openAIExtraParamsJSON,
    customHeadersEnabled,
    customHeadersJSON,
    anthropicExtraParamsEnabled,
    anthropicExtraParamsJSON,
    contextWindowTokens: asPositiveInteger(
      raw.contextWindowTokens ?? raw.context_window_tokens ?? raw.maxInputTokens ?? raw.max_input_tokens,
    ),
    maxCompletionTokens: asPositiveInteger(
      raw.maxCompletionTokens ?? raw.max_completion_tokens ?? raw.max_tokens ?? raw.max_token,
    ),
    anthropicMaxTokens: asPositiveInteger(
      raw.anthropicMaxTokens ?? raw.anthropic_max_tokens ?? raw.max_tokens,
    ),
    anthropicThinkingEffort: normalizedType === "anthropic"
      ? (SUPPORTED_ANTHROPIC_THINKING_EFFORTS.has(normalizedAnthropicThinkingEffort)
        ? normalizedAnthropicThinkingEffort
        : ANTHROPIC_THINKING_EFFORT_DEFAULT)
      : "",
    thinkingBudgetTokens: asPositiveInteger(
      raw.thinkingBudgetTokens ?? raw.thinking_budget_tokens,
    ),
    openAIEndpointGroupID: asString(raw.openAIEndpointGroupID ?? raw.openAIEndpointGroupId ?? raw.open_ai_endpoint_group_id),
    active: asBoolean(raw.active),
  };
}

export function normalizeModelAdapters(source) {
  return asArray(source).map((item) => normalizeModelAdapter(item));
}

export function validateModelAdapters(source) {
  const adapters = normalizeModelAdapters(source);
  const seenIdentityKeys = new Set();
  for (const [index, adapter] of adapters.entries()) {
    const prefix = `Model ${index + 1}`;
    if (!adapter.displayName) {
      return `${prefix} display name cannot be empty`;
    }
    if (!SUPPORTED_MODEL_ADAPTER_TYPES.has(adapter.type)) {
      return `${prefix} type only supports OpenAI or Anthropic`;
    }
    if (!adapter.baseURL) {
      return `${prefix} endpoint URL cannot be empty`;
    }
    if (!adapter.apiKey) {
      return `${prefix} API key cannot be empty`;
    }
    if (!adapter.tooltipData) {
      return `${prefix} tooltip cannot be empty`;
    }
    if (!adapter.modelID) {
      return `${prefix} model ID cannot be empty`;
    }
    if (adapter.type === "openai" && !SUPPORTED_REASONING_EFFORTS.has(adapter.reasoningEffort)) {
      return `${prefix} reasoning effort only supports low, medium, high, xhigh, max`;
    }
    if (adapter.type === "openai" && !isValidOpenAIEndpoint(adapter.openAIEndpoint)) {
      return `${prefix} OpenAI endpoint only supports /v1/responses, /v1/chat/completions, or custom path starting with /`;
    }
    if (adapter.type === "openai" && adapter.openAIExtraParamsEnabled) {
      const extraParamsError = validateOpenAIExtraParamsJSON(adapter.openAIExtraParamsJSON);
      if (extraParamsError) {
        return `${prefix} ${extraParamsError}`;
      }
    }
    if (adapter.customHeadersEnabled) {
      const customHeadersError = validateHeadersJSON(adapter.customHeadersJSON);
      if (customHeadersError) {
        return `${prefix} ${customHeadersError}`;
      }
    }
    if (adapter.type === "anthropic" && adapter.anthropicExtraParamsEnabled) {
      const extraParamsError = validateAnthropicExtraParamsJSON(adapter.anthropicExtraParamsJSON);
      if (extraParamsError) {
        return `${prefix} ${extraParamsError}`;
      }
    }
    if (adapter.type === "anthropic" && !SUPPORTED_ANTHROPIC_THINKING_EFFORTS.has(adapter.anthropicThinkingEffort)) {
      return `${prefix} Anthropic thinking effort only supports low, medium, high, xhigh, max`;
    }
    if (adapter.contextWindowTokens && (!Number.isInteger(adapter.contextWindowTokens) || adapter.contextWindowTokens <= 0)) {
      return `${prefix} context window tokens must be a positive integer`;
    }
    if (adapter.maxCompletionTokens && (!Number.isInteger(adapter.maxCompletionTokens) || adapter.maxCompletionTokens <= 0)) {
      return `${prefix} max completion tokens must be a positive integer`;
    }
    if (adapter.anthropicMaxTokens && (!Number.isInteger(adapter.anthropicMaxTokens) || adapter.anthropicMaxTokens <= 0)) {
      return `${prefix} max completion tokens must be a positive integer`;
    }
    if (adapter.thinkingBudgetTokens && (!Number.isInteger(adapter.thinkingBudgetTokens) || adapter.thinkingBudgetTokens <= 0)) {
      return `${prefix} thinking budget tokens must be a positive integer`;
    }
    const dedupeKey = buildModelAdapterIdentityKey(adapter);
    if (seenIdentityKeys.has(dedupeKey)) {
      return `Duplicate model channel, please check url, modelID, apiKey, displayName, endpoint combination`;
    }
    seenIdentityKeys.add(dedupeKey);
  }
  return "";
}

function validateConfigPayload(payload) {
  if (!SUPPORTED_ROUTE_MODES.has(normalizeRouteMode(payload?.routing?.mode, ""))) {
    return "Routing mode only supports local or upstream";
  }
  return "";
}

function canUseLocalStorage() {
  return typeof window !== "undefined" && typeof window.localStorage !== "undefined";
}

function delay(ms) {
  return new Promise((resolve) => {
    window.setTimeout(resolve, Math.max(0, ms));
  });
}

function createEmptyHomeMetrics() {
  return {
    turnsTotal: 0,
    validTurnsTotal: 0,
    invalidTurnsTotal: 0,
    requestTokensTotal: 0,
    promptTokensTotal: 0,
    cacheReadTokens: 0,
    cacheWriteTokens: 0,
    cacheHitRate: null,
  };
}

function loadCachedState() {
  if (!canUseLocalStorage()) {
    return {};
  }

  try {
    const raw = window.localStorage.getItem(APP_STATE_STORAGE_KEY);
    if (!raw) {
      return {};
    }
    const parsed = JSON.parse(raw);
    if (!parsed || typeof parsed !== "object") {
      return {};
    }
    return parsed;
  } catch (_error) {
    return {};
  }
}

function normalizeConfig(source) {
  const raw = source && typeof source === "object" ? source : {};
  const routing = raw.routing && typeof raw.routing === "object" ? raw.routing : {};
  const homeMetrics = raw.homeMetrics && typeof raw.homeMetrics === "object" ? raw.homeMetrics : {};
  return {
    log: asBoolean(raw.log),
    providerStreamIdleTimeout: asPositiveInteger(raw.providerStreamIdleTimeout),
    backendListenAddr: asString(raw.configBackendListenAddr) || asString(raw.backendListenAddr),
    proxyListenAddr: asString(raw.configProxyListenAddr) || asString(raw.proxyListenAddr),
    modelAdapters: normalizeModelAdapters(raw.modelAdapters),
    routing: {
      mode: normalizeRouteMode(routing.mode),
    },
    homeMetrics: {
      includeCacheWriteInHitRate: asBoolean(homeMetrics.includeCacheWriteInHitRate),
    },
    lastAgentModelHash: asString(raw.lastAgentModelHash),
  };
}

function asNullableRate(value) {
  if (typeof value === "number" && Number.isFinite(value)) {
    return value;
  }
  return null;
}

function normalizeHomeMetrics(source) {
  const raw = source && typeof source === "object" ? source : {};
  const providerCallsTotal = asPositiveInteger(raw.providerCallsTotal ?? raw.turnsTotal);
  return {
    turnsTotal: providerCallsTotal,
    validTurnsTotal: providerCallsTotal,
    invalidTurnsTotal: 0,
    requestTokensTotal: asPositiveInteger(raw.requestTokensTotal),
    promptTokensTotal: asPositiveInteger(raw.promptTokensTotal),
    cacheReadTokens: asPositiveInteger(raw.cacheReadTokens),
    cacheWriteTokens: asPositiveInteger(raw.cacheWriteTokens),
    cacheHitRate: asNullableRate(raw.cacheHitRate),
  };
}

function applyHomeMetrics(raw) {
  appState.homeMetrics = normalizeHomeMetrics(raw);
  appState.homeMetricsError = "";
}

function buildConfigPayload(source = appState) {
  const normalized = normalizeConfig(source);
  return {
    log: normalized.log,
    providerStreamIdleTimeout: normalized.providerStreamIdleTimeout,
    backendListenAddr: normalized.backendListenAddr,
    proxyListenAddr: normalized.proxyListenAddr,
    modelAdapters: normalized.modelAdapters.map(({ id, ...adapter }) => adapter),
    routing: normalized.routing,
    homeMetrics: normalized.homeMetrics,
    lastAgentModelHash: normalized.lastAgentModelHash,
  };
}

function applyConfigToState(config, { modelAdaptersOnly = false } = {}) {
  const normalized = normalizeConfig(config);
  if (modelAdaptersOnly) {
    appState.modelAdapters = normalized.modelAdapters;
    return normalized;
  }
  appState.modelAdapters = normalized.modelAdapters;
  appState.configBackendListenAddr = normalized.backendListenAddr;
  appState.configProxyListenAddr = normalized.proxyListenAddr;
  appState.routingMode = normalized.routing.mode;
  appState.debugLogEnabled = normalized.log;
  appState.includeCacheWriteInHitRate = normalized.homeMetrics.includeCacheWriteInHitRate;
  return normalized;
}

async function loadPersistedUserConfig() {
  return normalizeConfig(await loadUserConfig());
}

async function persistConfigPayload(config, { modelAdaptersOnly = false } = {}) {
  const payload = buildConfigPayload(config);
  const configValidationError = validateConfigPayload(payload);
  if (configValidationError) {
    return {
      ok: false,
      error: configValidationError,
    };
  }
  const validationError = validateModelAdapters(payload.modelAdapters);
  if (validationError) {
    return {
      ok: false,
      error: validationError,
    };
  }

  appState.configSaving = true;
  try {
    await saveUserConfig(payload);
    const persisted = await loadPersistedUserConfig();
    applyConfigToState(persisted, { modelAdaptersOnly });
    return {
      ok: true,
      error: "",
    };
  } catch (error) {
    return {
      ok: false,
      error: toUserError(error),
    };
  } finally {
    appState.configSaving = false;
  }
}

function applyProxyState(raw) {
  const state = raw && typeof raw === "object" ? raw : {};
  appState.backendRunning = asBoolean(state.backendRunning);
  appState.proxyRunning = asBoolean(state.proxyRunning ?? state.running);
  appState.serviceRunning = appState.proxyRunning;
  appState.serviceLastError = asString(state.lastError);
  appState.backendListenAddr = asString(state.backendListenAddr);
  appState.proxyListenAddr = asString(state.proxyListenAddr || state.listenAddr);
  appState.serviceListenAddr = appState.proxyListenAddr;
  appState.cursorSettingsApplied = asBoolean(state.cursorSettingsApplied);
  appState.netProxySource = asString(state.netProxySource);
  appState.netProxyActive = asBoolean(state.netProxyActive);
  appState.netProxyUsingSystem = asBoolean(state.netProxyUsingSystem);
  appState.netProxyUsingEnv = asBoolean(state.netProxyUsingEnv);
  appState.netProxyHttp = asString(state.netProxyHttp);
  appState.netProxyHttps = asString(state.netProxyHttps);
  appState.netProxyPacIgnored = asBoolean(state.netProxyPacIgnored);
  appState.netProxyDescription = asString(state.netProxyDescription);
}

function handleProxyStateEvent(event) {
  if (event?.data && typeof event.data === "object") {
    applyProxyState(event.data);
    return;
  }
  void syncServiceState().catch(() => {});
}

function handleUserConfigChangedEvent(event) {
  if (event?.data && typeof event.data === "object") {
    applyConfigToState(event.data);
    return;
  }
  void reloadUserConfig().catch(() => {});
}

function applyModelAdapterTestResults(source) {
  const next = {};
  for (const result of normalizeModelAdapterTestResults(source)) {
    next[result.adapterID] = result;
  }
  appState.modelAdapterTestResults = next;
  return next;
}

function handleModelAdapterTestUpdatedEvent(event) {
  if (event?.data) {
    applyModelAdapterTestResults(event.data);
    return;
  }
  void refreshModelAdapterTestResults().catch(() => {});
}

function normalizeUpdateState(value) {
  const text = asString(value).toLowerCase();
  if (["idle", "checking", "downloading", "ready", "installing", "error"].includes(text)) {
    return text;
  }
  return "idle";
}

function applyUpdateSnapshot(raw) {
  const data = raw && typeof raw === "object" ? raw : {};
  const nextState = normalizeUpdateState(data.state ?? appState.updateState);
  appState.updateState = nextState;

  const version = asString(data.version);
  if (version) {
    appState.updateVersion = version;
  } else if (nextState === "idle") {
    appState.updateVersion = "";
  }

  const releaseDate = asString(data.releaseDate);
  if (releaseDate) {
    appState.updateReleaseDate = releaseDate;
  } else if (nextState === "idle") {
    appState.updateReleaseDate = "";
  }

  if (typeof data.releaseNotes === "string") {
    appState.updateReleaseNotes = data.releaseNotes.replace(/\r\n/g, "\n");
  } else if (nextState === "idle") {
    appState.updateReleaseNotes = "";
  }

  if (typeof data.error === "string") {
    appState.updateError = data.error.trim();
  } else if (nextState !== "error") {
    appState.updateError = "";
  }

  if (typeof data.message === "string") {
    appState.updateMessage = data.message.trim();
  } else if (!data.prompt) {
    appState.updateMessage = "";
  }

  if (typeof data.downloaded === "number") {
    appState.updateProgressDownloaded = data.downloaded;
  } else if (nextState !== "downloading") {
    appState.updateProgressDownloaded = 0;
  }

  if (typeof data.total === "number") {
    appState.updateProgressTotal = data.total;
  } else if (nextState !== "downloading") {
    appState.updateProgressTotal = 0;
  }

  if (typeof data.percentage === "number") {
    appState.updateProgressPercent = Math.max(0, Math.min(100, data.percentage));
  } else if (nextState !== "downloading") {
    appState.updateProgressPercent = 0;
  }
}

function openUpdatePrompt(kind, payload = {}) {
  appState.updatePromptKind = asString(kind) || "idle";
  appState.updatePromptVisible = true;
  appState.updatePromptBusy = false;
  if (typeof payload.message === "string") {
    appState.updateMessage = payload.message.trim();
  }
  if (typeof payload.error === "string") {
    appState.updateError = payload.error.trim();
  }
}

function handleUpdateStateEvent(event) {
  const data = event?.data && typeof event.data === "object" ? event.data : {};
  applyUpdateSnapshot(data);
  if (asBoolean(data.prompt)) {
    openUpdatePrompt(asString(data.promptKind) || "idle", data);
  }
}

function handleUpdateProgressEvent(event) {
  const data = event?.data && typeof event.data === "object" ? event.data : {};
  applyUpdateSnapshot({
    ...data,
    state: "downloading",
  });
}

function handleUpdateReadyEvent(event) {
  const data = event?.data && typeof event.data === "object" ? event.data : {};
  applyUpdateSnapshot({
    ...data,
    state: "ready",
  });
  if (data.prompt !== false) {
    openUpdatePrompt("ready", data);
  }
}

function handleUpdateErrorEvent(event) {
  const data = event?.data && typeof event.data === "object" ? event.data : {};
  applyUpdateSnapshot({
    ...data,
    state: "error",
  });
  if (asBoolean(data.prompt)) {
    openUpdatePrompt("error", data);
  }
}

function extractErrorMessage(error) {
  if (typeof error === "string") {
    return error.trim();
  }
  if (error && typeof error === "object") {
    return asString(error.message) || asString(error.error);
  }
  return "";
}

const cachedState = loadCachedState();
const cachedConfig = normalizeConfig(cachedState);

export const appState = reactive({
  appVersion: "",
  modelAdapters: cachedConfig.modelAdapters,
  modelAdapterTestResults: {},
  configBackendListenAddr: cachedConfig.backendListenAddr,
  configProxyListenAddr: cachedConfig.proxyListenAddr,
  routingMode: cachedConfig.routing.mode,
  debugLogEnabled: cachedConfig.log,
  includeCacheWriteInHitRate: cachedConfig.homeMetrics.includeCacheWriteInHitRate,

  serviceRunning: asBoolean(cachedState.serviceRunning),
  backendRunning: asBoolean(cachedState.backendRunning),
  proxyRunning: asBoolean(cachedState.proxyRunning),
  serviceBusy: false,
  serviceLastError: asString(cachedState.serviceLastError),
  serviceListenAddr: asString(cachedState.serviceListenAddr),
  backendListenAddr: asString(cachedState.backendListenAddr),
  proxyListenAddr: asString(cachedState.proxyListenAddr),
  cursorSettingsApplied: asBoolean(cachedState.cursorSettingsApplied),
  netProxySource: asString(cachedState.netProxySource),
  netProxyActive: asBoolean(cachedState.netProxyActive),
  netProxyUsingSystem: asBoolean(cachedState.netProxyUsingSystem),
  netProxyUsingEnv: asBoolean(cachedState.netProxyUsingEnv),
  netProxyHttp: asString(cachedState.netProxyHttp),
  netProxyHttps: asString(cachedState.netProxyHttps),
  netProxyPacIgnored: asBoolean(cachedState.netProxyPacIgnored),
  netProxyDescription: asString(cachedState.netProxyDescription),

  configSaving: false,
  homeMetrics: createEmptyHomeMetrics(),
  homeMetricsLoading: false,
  homeMetricsError: "",

  updateState: "idle",
  updateVersion: "",
  updateReleaseDate: "",
  updateReleaseNotes: "",
  updateProgressDownloaded: 0,
  updateProgressTotal: 0,
  updateProgressPercent: 0,
  updateError: "",
  updateMessage: "",
  updatePromptVisible: false,
  updatePromptKind: "idle",
  updatePromptBusy: false,
});

watchSyncEffect(() => {
  if (!canUseLocalStorage()) {
    return;
  }
  try {
    window.localStorage.setItem(
      APP_STATE_STORAGE_KEY,
      JSON.stringify({
        ...buildConfigPayload(),
        serviceRunning: appState.serviceRunning,
        backendRunning: appState.backendRunning,
        proxyRunning: appState.proxyRunning,
        serviceLastError: appState.serviceLastError,
        serviceListenAddr: appState.serviceListenAddr,
        configBackendListenAddr: appState.configBackendListenAddr,
        configProxyListenAddr: appState.configProxyListenAddr,
        backendListenAddr: appState.backendListenAddr,
        proxyListenAddr: appState.proxyListenAddr,
        cursorSettingsApplied: appState.cursorSettingsApplied,
        netProxySource: appState.netProxySource,
        netProxyActive: appState.netProxyActive,
        netProxyUsingSystem: appState.netProxyUsingSystem,
        netProxyUsingEnv: appState.netProxyUsingEnv,
        netProxyHttp: appState.netProxyHttp,
        netProxyHttps: appState.netProxyHttps,
        netProxyPacIgnored: appState.netProxyPacIgnored,
        netProxyDescription: appState.netProxyDescription,
      }),
    );
  } catch (_error) {
    // ignore local persistence failures
  }
});

watchSyncEffect((onCleanup) => {
  if (typeof window === "undefined") {
    return;
  }
  const unsubscribe = Events.On(PROXY_STATE_EVENT, handleProxyStateEvent);
  onCleanup(() => {
    unsubscribe();
  });
});

watchSyncEffect((onCleanup) => {
  if (typeof window === "undefined") {
    return;
  }
  const unsubscribe = Events.On(USER_CONFIG_CHANGED_EVENT, handleUserConfigChangedEvent);
  onCleanup(() => {
    unsubscribe();
  });
});

watchSyncEffect((onCleanup) => {
  if (typeof window === "undefined") {
    return;
  }
  const unsubscribe = Events.On(MODEL_ADAPTER_TEST_UPDATED_EVENT, handleModelAdapterTestUpdatedEvent);
  onCleanup(() => {
    unsubscribe();
  });
});

watchSyncEffect((onCleanup) => {
  if (typeof window === "undefined") {
    return;
  }
  const unsubscribe = Events.On(UPDATE_STATE_EVENT, handleUpdateStateEvent);
  onCleanup(() => {
    unsubscribe();
  });
});

watchSyncEffect((onCleanup) => {
  if (typeof window === "undefined") {
    return;
  }
  const unsubscribe = Events.On(UPDATE_PROGRESS_EVENT, handleUpdateProgressEvent);
  onCleanup(() => {
    unsubscribe();
  });
});

watchSyncEffect((onCleanup) => {
  if (typeof window === "undefined") {
    return;
  }
  const unsubscribe = Events.On(UPDATE_READY_EVENT, handleUpdateReadyEvent);
  onCleanup(() => {
    unsubscribe();
  });
});

watchSyncEffect((onCleanup) => {
  if (typeof window === "undefined") {
    return;
  }
  const unsubscribe = Events.On(UPDATE_ERROR_EVENT, handleUpdateErrorEvent);
  onCleanup(() => {
    unsubscribe();
  });
});

export const appViewState = reactive({
  serviceStatusText: computed(() => {
    if (appState.proxyRunning && appState.backendRunning) {
      return "Service Running";
    }
    if (appState.backendRunning) {
      return "Backend Started, Proxy Stopped";
    }
    return "Service Stopped";
  }),
  serviceStatusClass: computed(() =>
    appState.serviceRunning ? "text-[#22c55e]" : "text-[#f59e0b]",
  ),
  serviceButtonText: computed(() => {
    if (appState.serviceBusy) {
      return appState.serviceRunning ? "Stopping..." : "Starting...";
    }
    return appState.serviceRunning ? "Stop Service" : "Start Service";
  }),
});

function localizeUpdateMessage(msg) {
  if (!msg) return "";
  if (/(?:already using the latest version|当前已是最新版本)/i.test(msg)) {
    const match = msg.match(/v?([0-9]+\.[0-9]+\.[0-9]+)/);
    const version = match ? match[1] : appState.appVersion || "...";
    return `Already using the latest version (v${version}).`;
  }
  return msg;
}

function localizeReadyContent() {
  const version = appState.updateVersion || appState.appVersion || "...";
  const date = formatReleaseDate(appState.updateReleaseDate);
  const notes = appState.updateReleaseNotes || "";

  return [
    `Version: v${version}`,
    `Release Date: ${date}`,
    "",
    notes || "No release notes",
  ].join("\n");
}

export const updateViewState = reactive({
  footerDownloading: computed(() => appState.updateState === "downloading"),
  footerBusy: computed(() => ["checking", "installing"].includes(appState.updateState)),
  footerVersionLabel: computed(() => `v${appState.appVersion || "..."}`),
  footerProgressText: computed(() => `${Math.round(appState.updateProgressPercent || 0)}%`),
  footerProgressStyle: computed(() => ({
    width: `${Math.max(0, Math.min(100, appState.updateProgressPercent || 0))}%`,
  })),
  promptTitle: computed(() => {
    switch (appState.updatePromptKind) {
      case "ready":
        return "New Version Found";
      case "error":
        return "Update Failed";
      default:
        return "Check for Updates";
    }
  }),
  promptContent: computed(() => {
    switch (appState.updatePromptKind) {
      case "ready":
        return localizeReadyContent();
      case "error":
        return appState.updateError || localizeUpdateMessage(appState.updateMessage) || GENERIC_SERVICE_ERROR;
      default:
        return localizeUpdateMessage(appState.updateMessage) || localizeUpdateMessage(`Already using the latest version (v${appState.appVersion || "..."}).`);
    }
  }),
  promptConfirmText: computed(() => {
    if (appState.updatePromptKind === "ready") {
      return "Restart and Update Now";
    }
    return "OK";
  }),
  promptCancelText: computed(() => {
    if (appState.updatePromptKind === "ready") {
      return "Later";
    }
    return "Cancel";
  }),
  promptShowCancel: computed(() => appState.updatePromptKind === "ready"),
});

export function getModelAdapterTestResultByID(adapterID) {
  const id = asString(adapterID);
  if (!id) {
    return null;
  }
  return appState.modelAdapterTestResults[id] ?? null;
}

export function getModelAdapterTestResult(adapter) {
  const normalized = normalizeModelAdapter(adapter);
  if (normalized.id && appState.modelAdapterTestResults[normalized.id]) {
    return appState.modelAdapterTestResults[normalized.id];
  }
  const requestHash = buildModelAdapterTestRequestHash(normalized);
  return Object.values(appState.modelAdapterTestResults).find((result) => result.requestHash === requestHash) ?? null;
}

export function isModelAdapterTestResultRunning(adapter) {
  return getModelAdapterTestResult(adapter)?.status === "running";
}

export function isModelAdapterTestResultStale(adapter, result) {
  if (!result || !result.requestHash) {
    return false;
  }
  return result.requestHash !== buildModelAdapterTestRequestHash(adapter);
}

export async function refreshModelAdapterTestResults() {
  const results = await getModelAdapterTestResults();
  applyModelAdapterTestResults(results);
  return Object.values(appState.modelAdapterTestResults);
}

export function startModelAdapterTest(adapter) {
  const normalized = normalizeModelAdapter(adapter);
  return testModelAdapter(normalized).then((rawResult) => {
    const result = normalizeModelAdapterTestResult(rawResult);
    if (result.adapterID) {
      appState.modelAdapterTestResults = {
        ...appState.modelAdapterTestResults,
        [result.adapterID]: result,
      };
    }
    return result;
  });
}

export async function runModelAdapterTest(adapter) {
  return startModelAdapterTest(adapter);
}

export async function persistUserConfig() {
  const currentConfig = await loadPersistedUserConfig();
  return persistConfigPayload({
    ...currentConfig,
    modelAdapters: normalizeModelAdapters(appState.modelAdapters),
    routing: {
      mode: appState.routingMode,
    },
    homeMetrics: {
      ...currentConfig.homeMetrics,
      includeCacheWriteInHitRate: appState.includeCacheWriteInHitRate,
    },
  });
}

export async function saveIncludeCacheWriteInHitRate(value) {
  const currentConfig = await loadPersistedUserConfig();
  const previousValue = appState.includeCacheWriteInHitRate;
  const nextValue = asBoolean(value);
  appState.includeCacheWriteInHitRate = nextValue;
  const result = await persistConfigPayload({
    ...currentConfig,
    homeMetrics: {
      ...currentConfig.homeMetrics,
      includeCacheWriteInHitRate: nextValue,
    },
  });
  if (!result.ok) {
    appState.includeCacheWriteInHitRate = previousValue;
  }
  return result;
}

export async function saveRoutingMode(mode) {
  const currentConfig = await loadPersistedUserConfig();
  return persistConfigPayload({
    ...currentConfig,
    routing: {
      mode: normalizeRouteMode(mode),
    },
  });
}

export async function saveDebugLogEnabled(enabled) {
  const currentConfig = await loadPersistedUserConfig();
  const previousValue = appState.debugLogEnabled;
  const nextValue = asBoolean(enabled);
  appState.debugLogEnabled = nextValue;
  const result = await persistConfigPayload({
    ...currentConfig,
    log: nextValue,
  });
  if (!result.ok) {
    appState.debugLogEnabled = previousValue;
  }
  return result;
}

export async function reloadUserConfig(options = {}) {
  const config = await loadPersistedUserConfig();
  applyConfigToState(config, options);
  return config;
}

export async function saveModelAdapterAt(index, adapter) {
  const currentConfig = await loadPersistedUserConfig();
  const nextAdapters = normalizeModelAdapters(currentConfig.modelAdapters);
  const nextAdapter = normalizeModelAdapter(adapter);
  const targetIndex = index >= 0 && index < nextAdapters.length ? index : nextAdapters.length;

  if (index >= 0 && index < nextAdapters.length) {
    nextAdapters.splice(index, 1, nextAdapter);
  } else {
    nextAdapters.push(nextAdapter);
  }

  const result = await persistConfigPayload(
    {
      ...currentConfig,
      modelAdapters: nextAdapters,
    },
    { modelAdaptersOnly: true },
  );
  if (!result.ok) {
    return result;
  }
  return {
    ...result,
    index: targetIndex,
    adapter: appState.modelAdapters[targetIndex] ?? null,
  };
}

// saveModelAdapters persists the full model adapter list in a single write (writes config only once).
// Used for bulk-change scenarios (e.g. scanning new models, toggling active) to avoid repeated disk writes.
export async function saveModelAdapters(adapters) {
  const currentConfig = await loadPersistedUserConfig();
  const nextAdapters = normalizeModelAdapters(adapters);
  return persistConfigPayload(
    {
      ...currentConfig,
      modelAdapters: nextAdapters,
    },
    { modelAdaptersOnly: true },
  );
}

export async function deleteModelAdapterAt(index) {
  const currentConfig = await loadPersistedUserConfig();
  const nextAdapters = normalizeModelAdapters(currentConfig.modelAdapters);

  if (index < 0 || index >= nextAdapters.length) {
    return {
      ok: false,
      error: "Model configuration does not exist, cannot delete",
    };
  }

  nextAdapters.splice(index, 1);

  return persistConfigPayload(
    {
      ...currentConfig,
      modelAdapters: nextAdapters,
    },
    { modelAdaptersOnly: true },
  );
}

function splitDisplayNameSeed(value) {
  const text = asString(value);
  const match = text.match(/^(.*?)(?:\s*[-+](\d+))?$/);
  if (!match) {
    return { base: text || "Model", number: 0 };
  }
  const base = asString(match[1]) || "Model";
  const number = match[2] ? Number(match[2]) : 0;
  return { base, number: Number.isFinite(number) ? number : 0 };
}

function buildNextDisplayName(existingAdapters, sourceName) {
  const { base } = splitDisplayNameSeed(sourceName);
  let next = 1;
  const taken = new Set(
    normalizeModelAdapters(existingAdapters)
      .map((adapter) => adapter.displayName)
      .filter(Boolean),
  );

  while (taken.has(`${base}-${next}`)) {
    next += 1;
  }
  return `${base}-${next}`;
}

export async function duplicateModelAdapterAt(index) {
  const currentConfig = await loadPersistedUserConfig();
  const nextAdapters = normalizeModelAdapters(currentConfig.modelAdapters);

  if (index < 0 || index >= nextAdapters.length) {
    return {
      ok: false,
      error: "Model configuration does not exist, cannot copy",
    };
  }

  const source = normalizeModelAdapter(nextAdapters[index]);
  const duplicate = {
    ...source,
    id: "",
    displayName: buildNextDisplayName(nextAdapters, source.displayName || source.modelID || "Model"),
  };

  nextAdapters.splice(index + 1, 0, duplicate);

  return persistConfigPayload(
    {
      ...currentConfig,
      modelAdapters: nextAdapters,
    },
    { modelAdaptersOnly: true },
  );
}

export async function syncServiceState() {
  const state = await getProxyState();
  applyProxyState(state);
  return state;
}

export async function syncHomeMetrics() {
  const startedAt = Date.now();
  appState.homeMetricsLoading = true;
  try {
    const summary = await getHomeMetricsSummary();
    applyHomeMetrics(summary);
    return {
      ok: true,
      error: "",
    };
  } catch (error) {
    appState.homeMetricsError = toUserError(error);
    return {
      ok: false,
      error: appState.homeMetricsError,
    };
  } finally {
    const elapsed = Date.now() - startedAt;
    if (elapsed < HOME_METRICS_MIN_LOADING_MS) {
      await delay(HOME_METRICS_MIN_LOADING_MS - elapsed);
    }
    appState.homeMetricsLoading = false;
  }
}

export async function startService() {
  if (appState.serviceBusy) {
    return { ok: false, error: "Service status updating, please try again later" };
  }
  appState.serviceBusy = true;
  try {
    const saved = await persistUserConfig();
    if (!saved.ok) {
      return saved;
    }
    const state = await startProxyService();
    applyProxyState(state);
    return { ok: true, error: "" };
  } catch (error) {
    await syncServiceState().catch(() => {});
    return { ok: false, error: toUserError(error) };
  } finally {
    appState.serviceBusy = false;
  }
}

export async function stopService() {
  if (appState.serviceBusy) {
    return { ok: false, error: "Service status updating, please try again later" };
  }
  appState.serviceBusy = true;
  try {
    const state = await stopProxyService();
    applyProxyState(state);
    return { ok: true, error: "" };
  } catch (error) {
    await syncServiceState().catch(() => {});
    return { ok: false, error: toUserError(error) };
  } finally {
    appState.serviceBusy = false;
  }
}

export async function toggleService() {
  if (appState.serviceRunning) {
    return stopService();
  }
  return startService();
}

export async function openLocalLogsDirectory() {
  await openLogsDirectory();
}

export async function openConfigWindow() {
  await openConfig();
}

export async function openModelConfigWindow() {
  await openModelConfig();
}

export async function openModelEditorWindow(index, adapter) {
  const adapterJSON = JSON.stringify(normalizeModelAdapter(adapter));
  await openModelEditor(index, adapterJSON);
}

export async function checkForAppUpdates() {
  await checkForUpdates();
}

export function dismissUpdatePrompt() {
  appState.updatePromptVisible = false;
  appState.updatePromptBusy = false;
}

export async function confirmUpdatePrompt() {
  if (appState.updatePromptKind !== "ready") {
    dismissUpdatePrompt();
    return;
  }
  if (appState.updatePromptBusy) {
    return;
  }
  appState.updatePromptBusy = true;
  try {
    await installReadyUpdate();
  } catch (error) {
    appState.updatePromptBusy = false;
    const message = toUserError(error);
    appState.updateError = message;
    openUpdatePrompt("error", { error: message });
  }
}

export function toUserError(error) {
  const message = extractErrorMessage(error);
  return message || GENERIC_SERVICE_ERROR;
}

export async function bootstrapAppState() {
  try {
    await reloadUserConfig();
  } catch (_error) {
    // keep cached config if loading fails
  }
  await refreshModelAdapterTestResults().catch(() => {});
  try {
    appState.appVersion = await getAppVersion();
  } catch (_error) {
    appState.appVersion = "";
  }
  await syncServiceState().catch(() => {});
  await syncHomeMetrics().catch(() => {});
}
