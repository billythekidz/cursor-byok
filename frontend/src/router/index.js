import { createRouter, createWebHashHistory } from "vue-router";
import Home from "@/views/Home.vue";
import ModelConfig from "@/views/ModelConfig.vue";
import ModelEditor from "@/views/ModelEditor.vue";

const router = createRouter({
  history: createWebHashHistory(),
  routes: [
    {
      path: "/",
      component: Home,
      meta: { showIcon: true, title: "Cursor Helper | Free & BYOK", directlyClose: false },
    },
    {
      path: "/model-config",
      component: ModelConfig,
      meta: { showIcon: false, title: "Model Configuration", directlyClose: true },
    },
    {
      path: "/model-editor",
      component: ModelEditor,
      meta: { showIcon: false, title: "Model Editor", directlyClose: true },
    },
  ],
});

export default router;
