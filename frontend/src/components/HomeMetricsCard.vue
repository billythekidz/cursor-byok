<script setup>
import CacheHitRateChart from "@/components/charts/CacheHitRateChart.vue";
import Switch from "@/components/ui/Switch.vue";
import Tooltip from "@/components/ui/Tooltip.vue";
import { appState, saveIncludeCacheWriteInHitRate } from "@/state/appState";
import { formatCompactInteger, formatInteger } from "@/utils/numberFormat";
import { computed, ref } from "vue";

const emit = defineEmits(["refresh", "open-ad"]);

const TOKEN_PRICE_PER_MILLION = {
  input: 5,
  output: 25,
  cacheRead: 0.5,
  cacheWrite: 6.25,
};

const props = defineProps({
  metrics: {
    type: Object,
    required: true,
  },
  loading: {
    type: Boolean,
    default: false,
  },
  error: {
    type: String,
    default: "",
  },
  homeAd: {
    type: Object,
    default: null,
  },
  homeAds: {
    type: Array,
    default: () => [],
  },
});

const homeMetricsConfigSaving = ref(false);
const homeMetricsConfigError = ref("");

function normalizeNumber(value) {
  const number = Number(value);
  if (!Number.isFinite(number)) {
    return 0;
  }
  return Math.round(number);
}

function formatMetricValue(value) {
  const full = formatInteger(value);
  const compact = formatCompactInteger(value);
  return full === compact ? full : `${full} (${compact})`;
}

function formatRateLabel(value) {
  const rate = Number(value);
  if (!Number.isFinite(rate)) {
    return "No data available";
  }
  return `${(Math.max(0, Math.min(1, rate)) * 100).toFixed(2)}%`;
}

function calculateRate(numerator, denominator) {
  const top = normalizeNumber(numerator);
  const bottom = normalizeNumber(denominator);
  if (bottom <= 0) {
    return null;
  }
  return top / bottom;
}

function priceTokens(tokens, pricePerMillion) {
  return (normalizeNumber(tokens) / 1_000_000) * pricePerMillion;
}

function formatUSD(value) {
  const amount = Number(value);
  if (!Number.isFinite(amount)) {
    return "$0.00";
  }
  if (amount > 0 && amount < 0.01) {
    return "<$0.01";
  }
  return `$${amount.toFixed(2)}`;
}

const cacheReadTokensTotal = computed(() => normalizeNumber(props.metrics?.cacheReadTokens));
const cacheWriteTokensTotal = computed(() => normalizeNumber(props.metrics?.cacheWriteTokens));

const inputTokensTotal = computed(() => {
  const promptTokensTotal = normalizeNumber(props.metrics?.promptTokensTotal);
  return Math.max(0, promptTokensTotal - cacheReadTokensTotal.value - cacheWriteTokensTotal.value);
});

const defaultCacheHitRate = computed(() =>
  calculateRate(cacheReadTokensTotal.value, cacheReadTokensTotal.value + inputTokensTotal.value),
);

const cacheReuseRate = computed(() =>
  calculateRate(
    cacheReadTokensTotal.value,
    cacheReadTokensTotal.value + cacheWriteTokensTotal.value + inputTokensTotal.value,
  ),
);

const includeCacheWriteInHitRate = computed(() => appState.includeCacheWriteInHitRate);

const selectedCacheHitRate = computed(() =>
  includeCacheWriteInHitRate.value ? cacheReuseRate.value : defaultCacheHitRate.value,
);

const selectedCacheRateModeLabel = computed(() =>
  includeCacheWriteInHitRate.value ? "Include cache creation" : "Default formula",
);

const validTurnsRate = computed(() => {
  const turnsTotal = normalizeNumber(props.metrics?.turnsTotal);
  if (turnsTotal <= 0) {
    return null;
  }
  return normalizeNumber(props.metrics?.validTurnsTotal) / turnsTotal;
});

const completionTokensTotal = computed(() => {
  const requestTokensTotal = normalizeNumber(props.metrics?.requestTokensTotal);
  const promptTokensTotal = normalizeNumber(props.metrics?.promptTokensTotal);
  return Math.max(0, requestTokensTotal - promptTokensTotal);
});

const estimatedTokenCost = computed(() => {
  const input = priceTokens(inputTokensTotal.value, TOKEN_PRICE_PER_MILLION.input);
  const output = priceTokens(completionTokensTotal.value, TOKEN_PRICE_PER_MILLION.output);
  const cacheRead = priceTokens(cacheReadTokensTotal.value, TOKEN_PRICE_PER_MILLION.cacheRead);
  const cacheWrite = priceTokens(cacheWriteTokensTotal.value, TOKEN_PRICE_PER_MILLION.cacheWrite);
  return {
    input,
    output,
    cacheRead,
    cacheWrite,
    total: input + output + cacheRead + cacheWrite,
  };
});

const cacheTooltipContent = computed(() => {
  const formula = includeCacheWriteInHitRate.value
    ? "Cache Read / (Cache Read + Cache Write + Input Tokens)"
    : "Cache Read / (Cache Read + Input Tokens)";
  return [
    `Current: ${formatRateLabel(selectedCacheHitRate.value)}`,
    `Formula: ${formula}`,
    `Default ${formatRateLabel(defaultCacheHitRate.value)} / Include Creation ${formatRateLabel(cacheReuseRate.value)}`,
  ].join("\n");
});

const turnsTooltipContent = computed(() =>
  [
    "Aggregated by turns scanned in history.",
    "",
    `Total Turns: ${formatMetricValue(props.metrics?.turnsTotal)}`,
    `Valid Turns: ${formatMetricValue(props.metrics?.validTurnsTotal)}`,
    `Invalid Turns: ${formatMetricValue(props.metrics?.invalidTurnsTotal)}`,
    `Valid Ratio: ${formatRateLabel(validTurnsRate.value)}`,
  ].join("\n"),
);

const tokensTooltipContent = computed(() =>
  [
    "Total Request Tokens include Prompt and Model Output.",
    "",
    `Total Requests: ${formatMetricValue(props.metrics?.requestTokensTotal)}`,
    `Prompt: ${formatMetricValue(props.metrics?.promptTokensTotal)}`,
    `Output Tokens: ${formatMetricValue(completionTokensTotal.value)}`,
    `Input Tokens: ${formatMetricValue(inputTokensTotal.value)}`,
    `Cache Read: ${formatMetricValue(cacheReadTokensTotal.value)}`,
    `Cache Write: ${formatMetricValue(cacheWriteTokensTotal.value)}`,
    "",
    "Cache read/write is included in prompt side statistics.",
  ].join("\n"),
);

const costTooltipContent = computed(() =>
  [
    "Estimated based on Claude Opus 4.7 pricing.",
    `Cache Policy: ${selectedCacheRateModeLabel.value} (${formatRateLabel(selectedCacheHitRate.value)})`,
    "",
    `Input: ${formatMetricValue(inputTokensTotal.value)} × $${TOKEN_PRICE_PER_MILLION.input}/1M = ${formatUSD(estimatedTokenCost.value.input)}`,
    `Output: ${formatMetricValue(completionTokensTotal.value)} × $${TOKEN_PRICE_PER_MILLION.output}/1M = ${formatUSD(estimatedTokenCost.value.output)}`,
    `Cache Read: ${formatMetricValue(cacheReadTokensTotal.value)} × $${TOKEN_PRICE_PER_MILLION.cacheRead}/1M = ${formatUSD(estimatedTokenCost.value.cacheRead)}`,
    `Cache Write: ${formatMetricValue(cacheWriteTokensTotal.value)} × $${TOKEN_PRICE_PER_MILLION.cacheWrite}/1M = ${formatUSD(estimatedTokenCost.value.cacheWrite)}`,
    "",
    `Total: ${formatUSD(estimatedTokenCost.value.total)}`,
  ].join("\n"),
);

function normalizeHomeAd(item, index) {
  const source = item && typeof item === "object" ? item : {};
  const title = typeof source.title === "string" ? source.title.trim() : "";
  if (!title) {
    return null;
  }
  return {
    id: typeof source.id === "string" && source.id.trim() ? source.id.trim() : String(index + 1),
    title,
    subtitle: typeof source.subtitle === "string" ? source.subtitle.trim() : "",
  };
}

async function toggleIncludeCacheWriteInHitRate(value) {
  const nextValue = Boolean(value);
  homeMetricsConfigSaving.value = true;
  homeMetricsConfigError.value = "";
  try {
    const result = await saveIncludeCacheWriteInHitRate(nextValue);
    if (!result?.ok) {
      homeMetricsConfigError.value = result?.error || "Save failed";
    }
  } catch (error) {
    homeMetricsConfigError.value = error?.message || "Save failed";
  } finally {
    homeMetricsConfigSaving.value = false;
  }
}

const normalizedHomeAds = computed(() => {
  const list = Array.isArray(props.homeAds) && props.homeAds.length > 0 ? props.homeAds : [props.homeAd];
  return list.map(normalizeHomeAd).filter(Boolean);
});

const hasHomeAd = computed(() => normalizedHomeAds.value.length > 0);
</script>

<template>
  <div>
    <div class="flex flex-col gap-4">
      <div class="flex items-center justify-between gap-4 h-[42px]">
        <div v-if="!hasHomeAd" class="flex flex-col gap-1 w-[200px] shrink-0">
          <h2 class="text-[14px] font-medium text-white/80">Session Metrics</h2>
        </div>
        <div v-else class="grid min-w-0  grid-cols-3 gap-2 shrink-0">
          <div
            v-for="ad in normalizedHomeAds"
            :key="ad.id"
            style="font-family: var(--font-num)"
            class="center-row h-[42px] min-w-0 cursor-pointer gap-[8px] rounded-[6px] border border-[#343434] bg-[#242424] px-[8px] pr-[10px] text-left transition-colors duration-150 hover:border-[#4a4a4a] hover:bg-[#2a2a2a] focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-amber-400/50"
            role="button"
            tabindex="0"
            :title="ad.subtitle ? `${ad.title}\n${ad.subtitle}` : ad.title"
            @click="emit('open-ad', ad.id)"
            @keydown.enter.prevent="emit('open-ad', ad.id)"
            @keydown.space.prevent="emit('open-ad', ad.id)"
          >
            <div
              class="center-row h-[20px] w-[20px] shrink-0 justify-center text-[20px] text-amber-400"
            >
              <span class="icon-[cil--badge]"></span>
            </div>
            <div class="min-w-0 flex-1">
              <div class="truncate text-[13px] font-medium leading-[16px] text-white">
                {{ ad.title }}
              </div>
              <div
                v-if="ad.subtitle"
                class="mt-[2px] center-row min-w-0 gap-[2px] text-[11px] leading-[12px] text-[#8A8A8A]"
              >
                <span class="truncate">{{ ad.subtitle }}</span>
              </div>
            </div>
          </div>
        </div>
        <div
          class="flex-1 center-row justify-end shrink-0 gap-2 text-xs text-[#6f6f6f] pr-4 w-[200px]"
        >
          <button
            type="button"
            class="center-row justify-center h-[24px] w-[24px] rounded-[6px] border border-[#3b3b3b] bg-[#242424] text-[#9d9d9d] transition-colors duration-150 hover:border-[#4c4c4c] hover:text-white disabled:cursor-not-allowed disabled:opacity-60"
            :disabled="loading"
            :title="loading ? 'Refreshing...' : 'Refresh Metrics'"
            @click="emit('refresh')"
          >
            <span
              class="icon-[mdi--refresh] text-[14px]"
              :class="{ '!animate-spin': loading }"
            ></span>
          </button>
        </div>
      </div>

      <div
        class="mt-[-4px] grid grid-cols-4 gap-0 overflow-hidden rounded-[8px] border border-[#343434] bg-[#242424] h-[130px]"
      >
        <div class="min-w-0 px-4 py-4 flex flex-col justify-between">
          <div class="center-row justify-start gap-1 text-xs text-[#7f7f7f]">
            <span>Cache Hit Rate</span>
            <Tooltip>
              <div class="w-[280px] space-y-3">
                <div class="border-b border-[#343434] pb-3">
                  <Switch
                    compact
                    label="Include Cache Creation"
                    description="When enabled, includes cache creation in the denominator"
                    enabled-text="Displaying reuse rate metric"
                    disabled-text="Displaying default hit rate metric"
                    :enabled="includeCacheWriteInHitRate"
                    :busy="homeMetricsConfigSaving"
                    :disabled="homeMetricsConfigSaving"
                    @change="toggleIncludeCacheWriteInHitRate"
                  />
                </div>
                <div class="whitespace-pre-wrap">{{ cacheTooltipContent }}</div>
                <div v-if="homeMetricsConfigError" class="text-[11px] text-[#f87171]">
                  {{ homeMetricsConfigError }}
                </div>
              </div>
            </Tooltip>
          </div>
          <CacheHitRateChart :rate="selectedCacheHitRate" />
        </div>

        <div
          class="min-w-0 border-l border-[#343434] px-4 py-4 flex flex-col justify-between"
        >
          <div class="center-row justify-start gap-1 text-xs text-[#7f7f7f]">
            <span>Conversation Turns</span>
            <Tooltip :content="turnsTooltipContent" />
          </div>
          <div>
            <div
              class="text-[30px] leading-none text-[#fff]"
              style="font-family: var(--font-num)"
              :title="formatInteger(metrics.turnsTotal)"
            >
              {{ formatCompactInteger(metrics.turnsTotal) }}
            </div>
            <div class="mt-3 text-xs leading-5 text-[#8c8c8c]">
              Valid
              <span :title="formatInteger(metrics.validTurnsTotal)">
                {{ formatCompactInteger(metrics.validTurnsTotal) }}
              </span>
              / Invalid
              <span :title="formatInteger(metrics.invalidTurnsTotal)">
                {{ formatCompactInteger(metrics.invalidTurnsTotal) }}
              </span>
            </div>
          </div>
        </div>

        <div
          class="min-w-0 border-l border-[#343434] px-4 py-4 flex flex-col justify-between"
        >
          <div class="center-row justify-start gap-1 text-xs text-[#7f7f7f]">
            <span>Token Consumption</span>
            <Tooltip :content="tokensTooltipContent" />
          </div>
          <div>
            <div
              class="truncate text-[30px] leading-none text-white"
              style="font-family: var(--font-num)"
              :title="formatInteger(metrics.requestTokensTotal)"
            >
              {{ formatCompactInteger(metrics.requestTokensTotal) }}
            </div>
            <div class="mt-3 text-xs leading-5 text-[#8c8c8c]">
              Prompt
              <span :title="formatInteger(metrics.promptTokensTotal)">
                {{ formatCompactInteger(metrics.promptTokensTotal) }}
              </span>
            </div>
          </div>
        </div>

        <div
          class="min-w-0 border-l border-[#343434] px-4 py-4 flex flex-col justify-between"
        >
          <div class="center-row justify-start gap-1 text-xs text-[#7f7f7f]">
            <span>Cost Estimation</span>
            <Tooltip :content="costTooltipContent" />
          </div>
          <div>
            <div
              class="truncate text-[30px] leading-none text-white"
              style="font-family: var(--font-num)"
              :title="formatUSD(estimatedTokenCost.total)"
            >
              {{ formatUSD(estimatedTokenCost.total) }}
            </div>
            <div class="mt-3 text-xs leading-5 text-[#8c8c8c]">
              Cache Read/Write
              <span :title="formatUSD(estimatedTokenCost.cacheRead + estimatedTokenCost.cacheWrite)">
                {{ formatUSD(estimatedTokenCost.cacheRead + estimatedTokenCost.cacheWrite) }}
              </span>
            </div>
          </div>
        </div>
      </div>
    </div>
  </div>
</template>

<style scoped></style>
