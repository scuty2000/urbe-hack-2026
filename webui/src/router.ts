import {
  createRouter,
  createWebHistory,
  type RouteRecordRaw,
} from "vue-router";
import { loadSession, user } from "./api";
import type { Role } from "./types";

declare module "vue-router" {
  interface RouteMeta {
    public?: boolean;
    chromeless?: boolean;
    roles?: Role[];
    title?: string;
  }
}

export function homeForRole(role?: Role): string {
  switch (role) {
    case "finder":
      return "/finder";
    case "searcher":
      return "/searcher";
    case "intermediary":
      return "/intermediary";
    case "institution":
      return "/institution";
    default:
      return "/welcome";
  }
}

const routes: RouteRecordRaw[] = [
  {
    path: "/login",
    name: "login",
    component: () => import("./views/LoginView.vue"),
    meta: { public: true, chromeless: true, title: "Accedi" },
  },
  {
    path: "/",
    name: "home",
    // On a full reload the session has not been loaded yet, so send the user
    // to the login route: its guard performs the session check and forwards
    // an already-authenticated user to the role home.
    redirect: () => (user.value ? homeForRole(user.value.role) : "/login"),
  },
  {
    path: "/welcome",
    name: "welcome",
    component: () => import("./views/WelcomeView.vue"),
    meta: { title: "Benvenuto" },
  },
  {
    path: "/finder",
    name: "finder-enroll",
    component: () => import("./views/FinderEnrollView.vue"),
    meta: { roles: ["finder"], title: "Segnala un oggetto" },
  },
  {
    path: "/finder/receipt/:id",
    name: "finder-receipt",
    component: () => import("./views/FinderReceiptView.vue"),
    meta: { roles: ["finder"], title: "Ricevuta" },
  },
  {
    path: "/intermediary",
    name: "intermediary-dashboard",
    component: () => import("./views/IntermediaryDashboardView.vue"),
    meta: { roles: ["intermediary"], title: "Dashboard" },
  },
  {
    path: "/intermediary/items/:id",
    name: "intermediary-item",
    component: () => import("./views/IntermediaryItemView.vue"),
    meta: { roles: ["intermediary"], title: "Dettaglio oggetto" },
  },
  {
    path: "/intermediary/enroll",
    name: "intermediary-enroll",
    component: () => import("./views/IntermediaryEnrollView.vue"),
    meta: { roles: ["intermediary"], title: "Registra un oggetto" },
  },
  {
    path: "/intermediary/scan",
    name: "intermediary-scan",
    component: () => import("./views/IntermediaryScanView.vue"),
    meta: { roles: ["intermediary"], title: "Scansiona un pass" },
  },
  {
    path: "/searcher",
    name: "searcher",
    component: () => import("./views/SearcherChatView.vue"),
    meta: { roles: ["searcher"], title: "Cerca il tuo oggetto" },
  },
  {
    path: "/searcher/pass/:id",
    name: "searcher-pass",
    component: () => import("./views/SearcherPassView.vue"),
    meta: { roles: ["searcher"], title: "Pass di ritiro" },
  },
  {
    path: "/institution",
    name: "institution-dashboard",
    component: () => import("./views/InstitutionDashboardView.vue"),
    meta: { roles: ["institution"], title: "Ritiri" },
  },
  {
    path: "/institution/intake",
    name: "institution-intake",
    component: () => import("./views/InstitutionIntakeView.vue"),
    meta: { roles: ["institution"], title: "Intake ufficiale" },
  },
  {
    path: "/institution/items/:id",
    name: "institution-item",
    component: () => import("./views/InstitutionItemView.vue"),
    meta: { roles: ["institution"], title: "Dettaglio oggetto" },
  },
  {
    path: "/institution/scan",
    name: "institution-scan",
    component: () => import("./views/IntermediaryScanView.vue"),
    meta: { roles: ["institution"], title: "Scansiona un pass" },
  },
  {
    path: "/:pathMatch(.*)*",
    redirect: "/",
  },
];

const router = createRouter({
  history: createWebHistory(import.meta.env.BASE_URL),
  routes,
  scrollBehavior(_to, _from, savedPosition) {
    return savedPosition ?? { top: 0 };
  },
});

router.beforeEach(async (to) => {
  const current = await loadSession();

  if (to.meta.public) {
    if (to.name === "login" && current) {
      const redirect =
        typeof to.query.redirect === "string" ? to.query.redirect : null;
      return redirect && redirect.startsWith("/") ? redirect : homeForRole(current.role);
    }
    return true;
  }

  if (!current) {
    return {
      name: "login",
      query: to.fullPath === "/" ? {} : { redirect: to.fullPath },
    };
  }

  const allowed = to.meta.roles;
  if (allowed && !allowed.includes(current.role)) {
    return homeForRole(current.role);
  }

  return true;
});

router.afterEach((to) => {
  const base = "Ritrova · Oggetti smarriti";
  document.title = to.meta.title ? `${to.meta.title} · Ritrova` : base;
});

export default router;
