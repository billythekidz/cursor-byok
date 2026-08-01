<script setup>
import Button from "@/components/ui/Button.vue";
import Card from "@/components/ui/Card.vue";
import LocaleSelect from "@/components/LocaleSelect.vue";
import Select from "@/components/ui/Select.vue";
import { showModal } from "@/composables/useModal";
import {
  appState,
  openModelConfigWindow,
  persistUserConfig,
  reloadUserConfig,
  ROUTE_MODE_OPTIONS,
  toUserError,
} from "@/state/appState";
import { onMounted } from "vue";

const routeModeOptions = ROUTE_MODE_OPTIONS;

async function showActionError(title, error) {
  await showModal({
    title,
    content: String(error || "Service error").trim() || "Service error",
  });
}

async function handleSaveConfig() {
  const result = await persistUserConfig();
  if (!result.ok) {
    await showActionError("Save failed", result.error);
    return;
  }
  await showModal({
    title: "Info",
    content: "Local config saved",
  });
}

async function handleOpenModelConfig() {
  try {
    await openModelConfigWindow();
  } catch (error) {
    await showActionError("Open failed", toUserError(error));
  }
}

onMounted(async () => {
  await reloadUserConfig().catch(() => {});
});
</script>

<template>
  <div class="flex h-full min-h-0 flex-col gap-4 overflow-y-auto p-4 pt-0 text-[#e5e5e5]">
    <Card>
      <div class="flex items-center justify-between gap-4">
        <div>
          <h2 class="text-base font-medium text-white">Local Configuration</h2>
          <div class="text-sm text-[#a3a3a3]">
            Configure routing mode and model channels; logs are located in <code>~/.cursor-local-assistant-v2/logs/</code>
          </div>
        </div>
        <Button variant="primary" :disabled="appState.configSaving" @click="handleSaveConfig">
          {{ appState.configSaving ? "Saving..." : "Save Config" }}
        </Button>
      </div>
    </Card>

    <Card>
      <div class="flex items-center justify-between gap-4">
        <div>
          <h2 class="text-base font-medium text-white">Routing Mode</h2>
          <div class="text-sm text-[#a3a3a3]">
            Control whether whitelist requests go through local service or direct upstream.
          </div>
        </div>
        <div class="w-[220px] max-w-full">
          <Select
            v-model="appState.routingMode"
            :options="routeModeOptions"
            placeholder="Select mode"
          />
        </div>
      </div>
    </Card>

    <Card>
      <div class="flex items-center justify-between gap-4">
        <div>
          <h2 class="text-base font-medium text-white">Interface Language</h2>
          <div class="text-sm text-[#a3a3a3]">
            Switch application language. Changes take effect immediately.
          </div>
        </div>
        <LocaleSelect wrapper-class="w-[220px] max-w-full" />
      </div>
    </Card>

    <Card>
      <div class="flex items-center justify-between gap-4">
        <div>
          <h2 class="text-base font-medium text-white">Model Configuration</h2>
          <div class="text-sm text-[#a3a3a3]">
            Configured {{ appState.modelAdapters.length }} model adapters
          </div>
        </div>
        <Button variant="primary" @click="handleOpenModelConfig">Open Model Config</Button>
      </div>
    </Card>
  </div>
</template>
