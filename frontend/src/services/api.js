import axios from 'axios';
import { useAuthStore } from '@/stores/auth';

// Создаем экземпляр axios с базовой конфигурацией
const apiClient = axios.create({
    baseURL: 'http://localhost:8080',
    headers: {
        'Content-Type': 'application/json'
    }
});

// Добавляем interceptor для всех исходящих запросов
apiClient.interceptors.request.use(config => {
    // Проверяем, является ли запрос защищенным (начинается с /api)
    if (config.url && config.url.startsWith('/api')) {
        const authStore = useAuthStore();
        const token = authStore.token;
        if (token) {
            config.headers.Authorization = `Bearer ${token}`;
        }
    }
    return config;
}, error => {
    return Promise.reject(error);
});

export default {
    // === AUTHENTICATION ===
    register(user) {
        return apiClient.post('/register', user);
    },
    login(credentials) {
        return apiClient.post('/login', credentials);
    },

    // === PROJECTS ===
    getProjects() {
        return apiClient.get('/api/projects');
    },
    getProject(projectId) {
        return apiClient.get(`/api/projects/${projectId}`);
    },
    createProject(project) {
        return apiClient.post('/api/projects', project);
    },
    updateProject(projectId, project) {
        return apiClient.patch(`/api/projects/${projectId}`, project);
    },
    deleteProject(projectId) {
        return apiClient.delete(`/api/projects/${projectId}`);
    },

    // === MEMBERS ===
    addProjectMember(projectId, memberData) {
        return apiClient.post(`/api/projects/${projectId}/members`, memberData);
    },
    
    // === TICKETS ===
    getTickets(projectId) {
        return apiClient.get(`/api/projects/${projectId}/tickets`);
    },
    getTicket(ticketId) {
        return apiClient.get(`/api/tickets/${ticketId}`);
    },
    createTicket(projectId, ticket) {
        return apiClient.post(`/api/projects/${projectId}/tickets`, ticket);
    },
    updateTicket(ticketId, ticket) {
        return apiClient.patch(`/api/tickets/${ticketId}`, ticket);
    },
    deleteTicket(ticketId) {
        return apiClient.delete(`/api/tickets/${ticketId}`);
    }
};