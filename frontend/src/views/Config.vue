<script setup>
import Button from "@/components/ui/Button.vue";
import Card from "@/components/ui/Card.vue";
import LocaleSelect from "@/components/LocaleSelect.vue";
import Select from "@/components/ui/Select.vue";
import { showModal } from "@/composables/useModal";
import {
  appState,
  cancelCodexRuntimeSetup,
  deviceLoginCodexRuntime,
  installCodexRuntime,
  loginCodexRuntime,
  openModelConfigWindow,
  persistUserConfig,
  reloadUserConfig,
  ROUTE_MODE_OPTIONS,
  refreshCodexRuntimeStatus,
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

async function handleInstallCodex() {
  await installCodexRuntime().catch((error) => showActionError("Codex 安装失败", toUserError(error)));
}

async function handleLoginCodex() {
  await loginCodexRuntime().catch((error) => showActionError("Codex 登录失败", toUserError(error)));
}

async function handleDeviceLoginCodex() {
  await deviceLoginCodexRuntime().catch((error) => showActionError("Codex 设备登录失败", toUserError(error)));
}

async function handleCancelCodex() {
  await cancelCodexRuntimeSetup().catch((error) => showActionError("取消失败", toUserError(error)));
}

onMounted(async () => {
  await reloadUserConfig().catch(() => {});
  await refreshCodexRuntimeStatus().catch(() => {});
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
      <div class="flex flex-col gap-3">
        <div class="flex items-center justify-between gap-4">
          <div>
            <h2 class="text-base font-medium text-white">Codex 运行环境</h2>
            <div class="text-sm text-[#a3a3a3]">使用 Codex CLI 管理 OAuth、工作区工具和模型线程。</div>
          </div>
          <span class="rounded-full border px-2 py-1 text-xs" :class="appState.codexRuntime.installed && appState.codexRuntime.authenticated ? 'border-[#1ca35a] text-[#10AD5D]' : 'border-[#8b6f3d] text-[#e0a458]'">
            {{ appState.codexRuntime.installed ? (appState.codexRuntime.authenticated ? "已就绪" : "已安装，需登录") : "未安装" }}
          </span>
        </div>
        <div v-if="appState.codexRuntime.version" class="text-xs text-[#8f8f8f]">版本 {{ appState.codexRuntime.version }}</div>
        <div v-if="appState.codexRuntime.error" class="text-xs text-[#e06c75]">{{ appState.codexRuntime.error }}</div>
        <pre v-if="appState.codexRuntime.setupOutput" class="max-h-32 overflow-auto rounded-[6px] bg-[#1f1f1f] p-2 text-xs text-[#a3a3a3]">{{ appState.codexRuntime.setupOutput }}</pre>
        <div class="flex flex-wrap gap-2">
          <Button variant="default" @click="refreshCodexRuntimeStatus">检查状态</Button>
          <Button v-if="!appState.codexRuntime.installed" variant="primary" @click="handleInstallCodex">安装 Codex</Button>
          <Button v-else-if="!appState.codexRuntime.authenticated" variant="primary" @click="handleLoginCodex">使用 ChatGPT 登录</Button>
          <Button v-if="appState.codexRuntime.installed && !appState.codexRuntime.authenticated" variant="default" @click="handleDeviceLoginCodex">无浏览器时使用设备登录</Button>
          <Button v-if="appState.codexRuntime.setupPhase === 'installing' || appState.codexRuntime.setupPhase === 'logging_in' || appState.codexRuntime.setupPhase === 'device_logging_in'" variant="text" @click="handleCancelCodex">取消设置</Button>
        </div>
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
