<template>
  <div class="row justify-content-center mt-5">
    <div class="col-md-6">
      <div class="card">
        <div class="card-header">Регистрация</div>
        <div class="card-body">
            <div v-if="error" class="alert alert-danger" role="alert">{{ error }}</div>
            <form @submit.prevent="handleRegister">
                <div class="mb-3">
                    <label for="username-input" class="form-label">Имя пользователя:</label>
                    <input id="username-input" v-model="form.username" class="form-control" required />
                </div>
                <div class="mb-3">
                    <label for="email-input" class="form-label">Email:</label>
                    <input id="email-input" v-model="form.email" type="email" class="form-control" required />
                </div>
                <div class="mb-3">
                    <label for="password-input" class="form-label">Пароль:</label>
                    <input id="password-input" v-model="form.password" type="password" class="form-control" required />
                </div>
                <button type="submit" class="btn btn-primary mt-2" :disabled="loading">
                    <span v-if="loading" class="spinner-border spinner-border-sm" aria-hidden="true"></span>
                    Зарегистрироваться
                </button>
                <p class="mt-3">
                    Уже есть аккаунт? <router-link to="/login">Войти</router-link>
                </p>
            </form>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup>
// Скрипт остается без изменений
import { ref } from 'vue';
import { useAuthStore } from '@/stores/auth';

const authStore = useAuthStore();
const form = ref({ username: '', email: '', password: '' });
const loading = ref(false);
const error = ref('');

const handleRegister = async () => {
  loading.value = true;
  error.value = '';
  try {
    await authStore.register(form.value);
  } catch (err) {
    error.value = 'Ошибка регистрации. Возможно, email уже занят.';
  } finally {
    loading.value = false;
  }
};
</script>