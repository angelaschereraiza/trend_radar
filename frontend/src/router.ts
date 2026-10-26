import { createRouter, createWebHistory } from 'vue-router'
import HomeView from './views/HomeView.vue'
import StockExplorerView from './views/StockExplorerView.vue'
import StockDetailView from './views/StockDetailView.vue'
import StrategyView from './views/StrategyView.vue'
import BacktestView from './views/BacktestView.vue'
import AlertsView from './views/AlertsView.vue'

export default createRouter({
  history: createWebHistory(),
  routes: [
    { path: '/', name: 'home', component: HomeView },
    { path: '/stocks', name: 'stocks', component: StockExplorerView },
    { path: '/stocks/:id', name: 'stock', component: StockDetailView },
    { path: '/strategies', name: 'strategies', component: StrategyView },
    { path: '/backtests/:id', name: 'backtest', component: BacktestView },
    { path: '/alerts', name: 'alerts', component: AlertsView },
  ],
})
