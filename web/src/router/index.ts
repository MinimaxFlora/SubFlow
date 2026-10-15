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
      meta: { title: 'Overview', subtitle: 'Engine status, capability summary and quick actions.' },
    },
    {
      path: '/subs',
      name: 'subs',
      component: SubsView,
      meta: { title: 'Subscriptions', subtitle: 'Remote and local node sources.' },
    },
    {
      path: '/convert',
      name: 'convert',
      component: ConvertView,
      meta: { title: 'Converter', subtitle: 'Fetch, process and render nodes for any client.' },
    },
    {
      path: '/settings',
      name: 'settings',
      component: SettingsView,
      meta: { title: 'Settings', subtitle: 'Runtime configuration stored with your data.' },
    },
    { path: '/:pathMatch(.*)*', redirect: { name: 'overview' } },
  ],
})

export default router
