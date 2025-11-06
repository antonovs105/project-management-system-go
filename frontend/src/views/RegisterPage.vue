<template>
  <div class="row justify-content-center mt-5">
    <div class="col-md-6">
      <div class="card">
        <div class="card-header">Реєстрація</div>
        <div class="card-body">
            <div v-if="error" class="alert alert-danger" role="alert">{{ error }}</div>
            <form @submit.prevent="handleRegister">
                <div class="mb-3">
                    <label for="username-input" class="form-label">Ім`я користувача:</label>
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
                    Зареєструватися
                </button>
                <p class="mt-3">
                    Вже є обліковий запис? <router-link to="/login">Увійти</router-link>
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
const form = ref({ username: '', email: '', password: '' });
const loading = ref(false);
const error = ref('');

const handleRegister = async () => {
  loading.value = true;
  error.value = '';
  try {
    await authStore.register(form.value);
  } catch (err) {
    error.value = 'Помилка реєстрації';
  } finally {
    loading.value = false;
  }
};
</script>