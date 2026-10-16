<script setup lang="ts">
import type { Tone } from '@/types'

defineProps<{
  value: string
  tone: Tone
  href?: string
  hint?: string
  small?: boolean
}>()
</script>

<template>
  <a
    v-if="href"
    class="chip"
    :class="[`chip--${tone}`, { 'chip--small': small }]"
    :href="href"
    target="_blank"
    rel="noreferrer"
    :title="hint"
    >{{ value }}</a
  >
  <span v-else class="chip" :class="[`chip--${tone}`, { 'chip--small': small }]" :title="hint">{{
    value
  }}</span>
</template>
