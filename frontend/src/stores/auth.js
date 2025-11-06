import { defineStore } from 'pinia';
import api from '@/services/api';
import router from '@/router';

export const useAuthStore = defineStore('auth', {
    state: () => ({
        token: localStorage.getItem('token') || null,
        user: null // Здесь можно будет хранить информацию о пользователе
    }),
    getters: {
        isAuthenticated: (state) => !!state.token,
    },
    actions: {
        async login(credentials) {
            try {
                const response = await api.login(credentials);
                const token = response.data.token;
                this.token = token;
                localStorage.setItem('token', token);
                // После успешного входа перенаправляем на главную
                router.push('/');
            } catch (error) {
                console.error("Login failed:", error);
                // Можно добавить обработку ошибок, например, показать сообщение пользователю
                throw error;
            }
        },
        async register(userData) {
            try {
                await api.register(userData);
                // После успешной регистрации перенаправляем на страницу входа
                router.push('/login');
            } catch (error) {
                console.error("Registration failed:", error);
                throw error;
            }
        },
        logout() {
            this.token = null;
            this.user = null;
            localStorage.removeItem('token');
            router.push('/login');
        },
        // Метод для инициализации состояния из localStorage при загрузке приложения
        initializeAuth() {
            const token = localStorage.getItem('token');
            if (token) {
                this.token = token;
            }
        }
    }
});