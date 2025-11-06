<template>
  <nav class="navbar navbar-expand-lg navbar-dark bg-primary">
    <div class="container">
      <router-link class="navbar-brand" to="/">GOProjectr</router-link>
      <button class="navbar-toggler" type="button" data-bs-toggle="collapse" data-bs-target="#nav-collapse" aria-controls="nav-collapse" aria-expanded="false" aria-label="Toggle navigation">
        <span class="navbar-toggler-icon"></span>
      </button>

      <div class="collapse navbar-collapse" id="nav-collapse">
        <ul class="navbar-nav ms-auto mb-2 mb-lg-0">
          <template v-if="auth.isAuthenticated">
            <li class="nav-item">
              <router-link to="/" class="nav-link" active-class="active">Проекти</router-link>
            </li>
            <li class="nav-item">
              <a href="#" class="nav-link" @click.prevent="handleLogout">Вийти</a>
            </li>
          </template>
          <template v-else>
            <li class="nav-item">
              <router-link to="/login" class="nav-link" active-class="active">Увійти</router-link>
            </li>
            <li class="nav-item">
              <router-link to="/register" class="nav-link" active-class="active">Реєстрація</router-link>
            </li>
          </template>
        </ul>
      </div>
    </div>
  </nav>
</template>

<script setup>
import { useAuthStore } from '@/stores/auth';

const auth = useAuthStore();

const handleLogout = () => {
  auth.logout();
};
</script>