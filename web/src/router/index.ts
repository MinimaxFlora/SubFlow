import { createRouter, createWebHistory } from 'vue-router'
import OverviewView from '@/views/OverviewView.vue'
import SubsView from '@/views/SubsView.vue'
import ConvertView from '@/views/ConvertView.vue'
import SettingsView from '@/views/SettingsView.vue'

const router = createRouter({
  history: createWebHistory(),
  routes: [
    { path: '/', redirect: { name: 'overview' } },
    {
      path: '/overview',
      name: 'overview',
      component: OverviewView,
      meta: { titleKey: 'meta.overview.title', subtitleKey: 'meta.overview.subtitle' },
    },
    {
      path: '/subs',
      name: 'subs',
      component: SubsView,
      meta: { titleKey: 'meta.subs.title', subtitleKey: 'meta.subs.subtitle' },
    },
    {
      path: '/convert',
      name: 'convert',
      component: ConvertView,
      meta: { titleKey: 'meta.convert.title', subtitleKey: 'meta.convert.subtitle' },
    },
    {
      path: '/settings',
      name: 'settings',
      component: SettingsView,
      meta: { titleKey: 'meta.settings.title', subtitleKey: 'meta.settings.subtitle' },
    },
    { path: '/:pathMatch(.*)*', redirect: { name: 'overview' } },
  ],
})

export default router
