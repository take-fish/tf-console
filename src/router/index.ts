import { createRouter, createWebHistory } from 'vue-router'

import PipelineView from '@/views/PipelineView.vue'

const router = createRouter({
  history: createWebHistory(import.meta.env.BASE_URL),
  routes: [
    {
      path: '/',
      name: 'pipeline',
      component: PipelineView,
    },
    {
      path: '/repos/:repo',
      name: 'repo',
      component: () => import('@/views/RepoView.vue'),
      props: true,
    },
    {
      path: '/:pathMatch(.*)*',
      redirect: '/',
    },
  ],
  scrollBehavior() {
    return { top: 0 }
  },
})

export default router
