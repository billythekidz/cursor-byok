import { reactive } from "vue";

export const inputModalState = reactive({
  visible: false,
  title: "Notice",
  content: "",
  placeholder: "",
  value: "",
  _resolve: null,
});

/**
 * Shows an input modal and returns a Promise<string|null>
 * @param {Object} options - { title, content, placeholder, defaultValue }
 * @returns {Promise<string|null>} - string=value after confirm, null=cancel
 */
export function showInputModal(options = {}) {
  return new Promise((resolve) => {
    inputModalState.visible = true;
    inputModalState.title = options.title ?? "Notice";
    inputModalState.content = options.content ?? "";
    inputModalState.placeholder = options.placeholder ?? "";
    inputModalState.value = String(options.defaultValue ?? "");
    inputModalState._resolve = resolve;
  });
}

export function resolveInputModal(ok) {
  const value = String(inputModalState.value ?? "").trim();
  inputModalState.visible = false;
  inputModalState._resolve?.(ok ? value : null);
  inputModalState._resolve = null;
  if (!ok) {
    inputModalState.value = "";
  }
}
