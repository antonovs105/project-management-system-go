import { defineStore } from 'pinia';
import api from '@/services/api';
import router from '@/router';

export const useAuthStore = defineStore('auth', {
    state: () => ({
        token: localStorage.getItem('token') || null,
        user: null // TODO: store user info
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
                // after successfl loging redirect to main page
                router.push('/');
            } catch (error) {
                console.error("Login failed:", error);
                // TODO: error handling
                throw error;
            }
        },
        async register(userData) {
            try {
                await api.register(userData);
                // after successfl reg redirect to login page
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
        initializeAuth() {
            const token = localStorage.getItem('token');
            if (token) {
                this.token = token;
            }
        }
    }
});