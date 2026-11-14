import { createApp } from "vue";
import App from "./App.vue";
import router from "./router";
import "./style.css";

window.addEventListener("auth:unauthorized", () => {
  if (router.currentRoute.value.name === "login") {
    return;
  }
  void router.replace({
    name: "login",
    query: { redirect: router.currentRoute.value.fullPath },
  });
});

createApp(App).use(router).mount("#app");
