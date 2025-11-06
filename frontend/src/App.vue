<template>
  <div>
    <AppNavbar />
    <b-container class="mt-4">
      <router-view />
    </b-container>
  </div>
</template>

<script setup>
import AppNavbar from './components/AppNavbar.vue';
</script>

<style>
/* global styles here */
</style>