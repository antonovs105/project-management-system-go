import { createApp } from 'vue'
import { createPinia } from 'pinia'
import App from './App.vue'
import router from './router'

// Импортируем только CSS от Bootstrap. Он у нас уже есть.
import 'bootstrap/dist/css/bootstrap.css'
// ИМПОРТИРУЕМ JS-БАНДЛ BOOTSTRAP ДЛЯ РАБОТЫ МОДАЛЬНЫХ ОКОН И ДР.
import 'bootstrap/dist/js/bootstrap.bundle.min.js'

const app = createApp(App)

app.use(createPinia())
app.use(router)

// Никаких app.use(BootstrapVueNext) больше нет!

app.mount('#app')