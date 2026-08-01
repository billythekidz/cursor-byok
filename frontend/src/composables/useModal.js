import { reactive } from "vue";

export const modalState = reactive({
  visible: false,
  title: "Notice",
  content: "",
  confirmText: "Confirm",
  cancelText: "Cancel",
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
    modalState.title = options.title ?? "Notice";
    modalState.content = options.content ?? "";
    modalState.confirmText = options.confirmText ?? "Confirm";
    modalState.cancelText = options.cancelText ?? "Cancel";
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
