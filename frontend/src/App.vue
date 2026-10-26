<script setup lang="ts">
import { RouterLink, RouterView } from 'vue-router'
</script>

<template>
  <div class="app-shell">
    <header class="topbar">
      <RouterLink
        class="brand"
        to="/"
      >
        TrendRadar
      </RouterLink>
      <span class="tagline">Market research, made transparent</span>
    </header>
    <main>
      <RouterView />
    </main>
    <footer>Research tool only — not investment advice. Capital limit: CHF 5,000.</footer>
  </div>
</template>
