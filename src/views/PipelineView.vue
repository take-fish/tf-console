<script setup lang="ts">
import { computed } from 'vue'

import PipelineBoard from '@/components/PipelineBoard.vue'
import { usePipelineStore } from '@/stores/pipeline'

const store = usePipelineStore()
const repos = computed(() => store.repos)
</script>

<template>
  <main>
    <PipelineBoard :repos="repos" :loading="store.loading" />
  </main>
</template>
