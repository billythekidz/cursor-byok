import { reactive } from "vue";

export const modalState = reactive({
  visible: false,
  title: "提示",
  content: "",
  confirmText: "确定",
  cancelText: "取消",
  showCancel: true,
  confirmDisabled: false,
  _resolve: null,
});

/**
 * Shows a confirm modal and returns a Promise<boolean>
 * @param {Object} options - { title, content }
 * @returns {Promise<boolean>} - true=confirm, false=cancel
 */
export function showModal(options = {}) {
  return new Promise((resolve) => {
    modalState.visible = true;
    modalState.title = options.title ?? "提示";
    modalState.content = options.content ?? "";
    modalState.confirmText = options.confirmText ?? "确定";
    modalState.cancelText = options.cancelText ?? "取消";
    modalState.showCancel = options.showCancel ?? true;
    modalState.confirmDisabled = options.confirmDisabled ?? false;
    modalState._resolve = resolve;
  });
}

export function resolveModal(ok) {
  modalState.visible = false;
  const resolve = modalState._resolve;
  modalState._resolve = null;
  resolve?.(ok);
}
