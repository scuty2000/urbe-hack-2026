import { createApp } from "vue";
import App from "./App.vue";
import { clearAllPasses } from "./passStore";
import router from "./router";
import "./style.css";

window.addEventListener("auth:unauthorized", () => {
  // The API layer already cleared the cached pass tokens; clear again here so
  // any other path that emits the event cannot leave a QR addressable.
  clearAllPasses();
  if (router.currentRoute.value.name === "login") {
    return;
  }
  void router.replace({
    name: "login",
    query: { redirect: router.currentRoute.value.fullPath },
  });
});

createApp(App).use(router).mount("#app");
