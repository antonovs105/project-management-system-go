<template>
  <div>
    <div class="d-flex justify-content-between align-items-center mb-4">
      <h1>Мои проекты</h1>
      <!-- Кнопка для вызова модального окна через data-атрибуты -->
      <button type="button" class="btn btn-primary" data-bs-toggle="modal" data-bs-target="#createProjectModal">
        Создать проект
      </button>
    </div>

    <div v-if="error" class="alert alert-danger">{{ error }}</div>
    <div v-if="loading" class="text-center">
      <div class="spinner-border" role="status">
        <span class="visually-hidden">Загрузка...</span>
      </div>
    </div>
    
    <div class="list-group" v-else-if="projects.length > 0">
      <router-link 
        v-for="project in projects" 
        :key="project.ID" 
        :to="{ name: 'project', params: { projectId: project.ID } }"
        class="list-group-item list-group-item-action d-flex justify-content-between align-items-center"
      >
        <div>
          <h5>{{ project.Name }}</h5>
          <p class="mb-0 text-muted">{{ project.Description }}</p>
        </div>
        <button class="btn btn-danger btn-sm" @click.prevent="deleteProject(project.ID)">Удалить</button>
      </router-link>
    </div>
    <div class="card" v-else>
        <div class="card-body">
            У вас пока нет проектов. Создайте первый!
        </div>
    </div>

    <!-- Модальное окно создания проекта (стандартная разметка Bootstrap 5) -->
    <div class="modal fade" id="createProjectModal" tabindex="-1" aria-labelledby="modalLabel" aria-hidden="true">
      <div class="modal-dialog">
        <div class="modal-content">
          <div class="modal-header">
            <h5 class="modal-title" id="modalLabel">Создание нового проекта</h5>
            <button type="button" class="btn-close" data-bs-dismiss="modal" aria-label="Close"></button>
          </div>
          <div class="modal-body">
            <form @submit.prevent="handleCreateProject">
              <div class="mb-3">
                <label for="projectName" class="form-label">Название проекта:</label>
                <input type="text" class="form-control" id="projectName" v-model="newProject.name" required>
              </div>
              <div class="mb-3">
                <label for="projectDesc" class="form-label">Описание:</label>
                <textarea class="form-control" id="projectDesc" rows="3" v-model="newProject.description"></textarea>
              </div>
            </form>
          </div>
          <div class="modal-footer">
            <button type="button" class="btn btn-secondary" data-bs-dismiss="modal">Отмена</button>
            <button type="button" class="btn btn-primary" @click="handleCreateProject">Создать</button>
          </div>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup>
import { ref, onMounted } from 'vue';
import api from '@/services/api';
// Импортируем класс Modal из Bootstrap для управления окном из JS
import { Modal } from 'bootstrap';

const projects = ref([]);
const loading = ref(true);
const error = ref('');
const newProject = ref({ name: '', description: '' });

let createModalInstance = null; // Переменная для хранения экземпляра модального окна

const fetchProjects = async () => {
  loading.value = true;
  error.value = '';
  try {
    const response = await api.getProjects();
    projects.value = response.data;
  } catch (err) {
    error.value = 'Не удалось загрузить проекты.';
  } finally {
    loading.value = false;
  }
};

const handleCreateProject = async () => {
  if (!newProject.value.name) {
    alert('Название проекта обязательно для заполнения');
    return;
  }
  
  try {
    await api.createProject(newProject.value);
    newProject.value = { name: '', description: '' };
    await fetchProjects();
    // Программно закрываем модальное окно после успеха
    createModalInstance.hide();
  } catch (err) {
    alert('Ошибка при создании проекта.');
    console.error(err);
  }
};

const deleteProject = async (projectId) => {
  if (confirm('Вы уверены, что хотите удалить этот проект?')) {
    try {
      await api.deleteProject(projectId);
      await fetchProjects();
    } catch (err) {
      alert('Не удалось удалить проект.');
    }
  }
};

onMounted(() => {
  fetchProjects();
  // Инициализируем модальное окно Bootstrap после того, как компонент будет смонтирован
  const modalEl = document.getElementById('createProjectModal');
  if (modalEl) {
    createModalInstance = new Modal(modalEl);
  }
});
</script>