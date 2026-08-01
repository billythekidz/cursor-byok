<script setup>
import { ref, watch } from "vue";
import Button from "@/components/ui/Button.vue";
import Input from "@/components/ui/Input.vue";

const props = defineProps({
  value: {
    type: Number,
    default: 0,
  },
  defaultValue: {
    type: Number,
    default: 1_000_000,
  },
  disabled: {
    type: Boolean,
    default: false,
  },
  saving: {
    type: Boolean,
    default: false,
  },
  error: {
    type: String,
    default: "",
  },
});

const emit = defineEmits(["save"]);
const draftValue = ref("");

watch(
  () => props.value,
  (value) => {
    draftValue.value = Number(value) > 0 ? String(value) : "";
  },
  { immediate: true },
);

function handleInput(value) {
  draftValue.value = String(value || "").replace(/[^0-9]/g, "");
}

function handleSave() {
  emit("save", draftValue.value);
}

function formatDefaultValue(value) {
  return Number(value || 0).toLocaleString("en-US");
}
</script>

<template>
  <div class="rounded-[8px] border border-[#315842] bg-[#17251d] px-3 py-2.5">
    <div class="flex items-center justify-between gap-3">
      <div class="min-w-0">
        <div class="text-[11px] font-medium uppercase tracking-[0.08em] text-[#9ee6b5]">上下文窗口</div>
        <div class="mt-1 text-[11px] leading-relaxed text-[#769982]">
          默认值 {{ formatDefaultValue(defaultValue) }}，留空使用默认值
        </div>
      </div>
      <div class="center-row shrink-0 gap-2">
        <Input
          :model-value="draftValue"
          type="text"
          inputmode="numeric"
          :placeholder="formatDefaultValue(defaultValue)"
          :disabled="disabled || saving"
          class="w-[126px]"
          aria-label="上下文窗口 Token 数"
          @update:model-value="handleInput"
          @keydown.enter="handleSave"
        />
        <Button
          variant="primary"
          :disabled="disabled || saving"
          @click="handleSave"
        >
          {{ saving ? "保存中..." : "保存" }}
        </Button>
      </div>
    </div>
    <div v-if="error" class="mt-2 text-[11px] text-[#fca5a5]">{{ error }}</div>
  </div>
</template>
