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
      <nav aria-label="Main navigation">
        <RouterLink to="/">Dashboard</RouterLink>
        <RouterLink to="/stocks">Stocks</RouterLink>
        <RouterLink to="/strategies">Strategies</RouterLink>
        <RouterLink to="/alerts">Alerts</RouterLink>
      </nav>
    </header>
    <main>
      <RouterView />
    </main>
    <footer>Research tool only — not investment advice. Capital limit: CHF 5,000.</footer>
  </div>
</template>
