import { createRouter, createWebHistory } from 'vue-router'
import { getToken } from './session'

const router = createRouter({
  history: createWebHistory(),
  routes: [
    {
      path: '/',
      redirect: '/overview',
    },
    {
      path: '/login',
      component: () => import('./views/LoginView.vue'),
      meta: { guestOnly: true },
    },
    {
      path: '/overview',
      component: () => import('./views/OverviewView.vue'),
      meta: { requiresAuth: true },
    },
    {
      path: '/desktop',
      component: () => import('./views/DesktopServiceView.vue'),
      meta: { requiresAuth: true },
    },
    {
      path: '/sites',
      component: () => import('./views/SitesView.vue'),
      meta: { requiresAuth: true },
    },
    {
      path: '/gateway',
      component: () => import('./views/GatewayView.vue'),
      meta: { requiresAuth: true },
    },
    {
      path: '/checkins',
      redirect: '/sites',
    },
    {
      path: '/connectivity',
      component: () => import('./views/ConnectivityView.vue'),
      meta: { requiresAuth: true },
    },
    {
      path: '/chat-test',
      component: () => import('./views/ChatTestView.vue'),
      meta: { requiresAuth: true },
    },
    {
      path: '/settings',
      component: () => import('./views/SettingsView.vue'),
      meta: { requiresAuth: true },
    },
  ],
})

router.beforeEach((to) => {
  const token = getToken()
  if (to.meta.requiresAuth && !token) {
    return '/login'
  }
  if (to.meta.guestOnly && token) {
    return '/overview'
  }
  return true
})

export default router
