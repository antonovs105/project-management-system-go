import { createRouter, createWebHistory } from 'vue-router'
import { useAuthStore } from '@/stores/auth';

// Импортируем представления (страницы)
import LoginPage from '../views/LoginPage.vue'
import RegisterPage from '../views/RegisterPage.vue'
import DashboardPage from '../views/DashboardPage.vue'
import ProjectPage from '../views/ProjectPage.vue'

const router = createRouter({
  history: createWebHistory(import.meta.env.BASE_URL),
  routes: [
    {
      path: '/login',
      name: 'login',
      component: LoginPage
    },
    {
      path: '/register',
      name: 'register',
      component: RegisterPage
    },
    {
      path: '/',
      name: 'dashboard',
      component: DashboardPage,
      meta: { requiresAuth: true } // Эта страница требует аутентификации
    },
    {
      path: '/projects/:projectId',
      name: 'project',
      component: ProjectPage,
      meta: { requiresAuth: true }
    },
    // Редирект на логин, если путь не найден
    {
        path: '/:pathMatch(.*)*',
        redirect: '/login'
    }
  ]
})

// Навигационный страж (Navigation Guard)
router.beforeEach((to, from, next) => {
  const authStore = useAuthStore();
  // Инициализируем состояние, если оно еще не было
  if (!authStore.token) {
      authStore.initializeAuth();
  }

  const isAuthenticated = authStore.isAuthenticated;
  const requiresAuth = to.matched.some(record => record.meta.requiresAuth);

  if (requiresAuth && !isAuthenticated) {
    // Если требуется аутентификация, а пользователь не залогинен, отправляем на /login
    next({ name: 'login' });
  } else if ((to.name === 'login' || to.name === 'register') && isAuthenticated) {
    // Если пользователь залогинен и пытается зайти на /login или /register, отправляем на главную
    next({ name: 'dashboard' });
  } else {
    // В остальных случаях просто продолжаем
    next();
  }
});


export default router