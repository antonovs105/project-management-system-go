<template>
  <div class="row justify-content-center mt-5">
    <div class="col-md-6">
      <div class="card">
        <div class="card-header">Вхід у систему</div>
        <div class="card-body">
          <div v-if="error" class="alert alert-danger" role="alert">{{ error }}</div>
          <form @submit.prevent="handleLogin">
            <div class="mb-3">
              <label for="email-input" class="form-label">Email:</label>
              <input id="email-input" v-model="form.email" type="email" class="form-control" required />
            </div>
            <div class="mb-3">
              <label for="password-input" class="form-label">Пароль:</label>
              <input id="password-input" v-model="form.password" type="password" class="form-control" required />
            </div>
            <button type="submit" class="btn btn-primary mt-2" :disabled="loading">
              <span v-if="loading" class="spinner-border spinner-border-sm" role="status" aria-hidden="true"></span>
              Увійти
            </button>
            <p class="mt-3">
              Немає обікового запису? <router-link to="/register">Зареєструватися</router-link>
            </p>
          </form>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup>
import { ref } from 'vue';
import { useAuthStore } from '@/stores/auth';

const authStore = useAuthStore();
const form = ref({ email: '', password: '' });
const loading = ref(false);
const error = ref('');

const handleLogin = async () => {
  loading.value = true;
  error.value = '';
  try {
    await authStore.login(form.value);
  } catch (err) {
    error.value = 'Невірний email або пароль';
  } finally {
    loading.value = false;
  }
};
</script>