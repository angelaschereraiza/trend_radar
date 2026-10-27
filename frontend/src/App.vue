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
      <nav aria-label="Main navigation">
        <RouterLink to="/">Dashboard</RouterLink>
        <RouterLink to="/stocks">Stocks</RouterLink>
        <RouterLink to="/strategies">Strategies</RouterLink>
        <RouterLink to="/alerts">Alerts</RouterLink>
      </nav>
    </header>
    <main id="main-content">
      <RouterView />
    </main>
    <footer>@2026 Aiza GmbH</footer>
  </div>
</template>
