<template>
  <div>
    <div class="d-flex justify-content-between align-items-center mb-4">
      <h1>Мої проекти</h1>
      <button type="button" class="btn btn-primary" @click="openCreateModal">
        Створити проект
      </button>
    </div>

    <div v-if="error" class="alert alert-danger">{{ error }}</div>

    <div class="list-group" v-if="projects.length > 0">
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
        <button class="btn btn-danger btn-sm" @click.prevent="deleteProject(project.ID)">Видалити</button>
      </router-link>
    </div>
    <div class="card" v-else>
        <div class="card-body">
            Поки що немає проектів. Створіть!
        </div>
    </div>

    <!-- модальбне вікно створення проенкту (Bootstrap 5) -->
    <div class="modal fade" id="createProjectModal" tabindex="-1" aria-labelledby="modalLabel" aria-hidden="true">
      <div class="modal-dialog">
        <div class="modal-content">
          <div class="modal-header">
            <h5 class="modal-title" id="modalLabel">Створення нового проекту</h5>
            <button type="button" class="btn-close" @click="closeCreateModal" aria-label="Close"></button>
          </div>
          <div class="modal-body">
            <form @submit.prevent="handleCreateProject">
              <div class="mb-3">
                <label for="projectName" class="form-label">Назва проекту:</label>
                <input type="text" class="form-control" id="projectName" v-model="newProject.name" required>
              </div>
              <div class="mb-3">
                <label for="projectDesc" class="form-label">Опис:</label>
                <textarea class="form-control" id="projectDesc" rows="3" v-model="newProject.description"></textarea>
              </div>
            </form>
          </div>
          <div class="modal-footer">
            <button type="button" class="btn btn-secondary" @click="closeCreateModal">Відмінити</button>
            <button type="button" class="btn btn-primary" @click="handleCreateProject">Створити</button>
          </div>
        </div>
      </div>
    </div>
  </div>
</template>

    
<script setup>
import { ref, onMounted } from 'vue';
import api from '@/services/api';
import { Modal } from 'bootstrap';

const projects = ref([]);
const loading = ref(true);
const error = ref('');
const newProject = ref({ name: '', description: '' });

let createModalInstance = null;

const fetchProjects = async () => {
  loading.value = true;
  error.value = '';
  try {
    const response = await api.getProjects();
    projects.value = response.data || [];
  } catch (err) {
    error.value = 'Не вдалося завантажити проекти';
    console.error("Помилка при завантаженні проектів:", err);
  } finally {
    loading.value = false;
  }
};

const openCreateModal = () => {
  if (createModalInstance) {
    createModalInstance.show();
  }
};

const closeCreateModal = () => {
  if (createModalInstance) {
    createModalInstance.hide();
  }
};

const handleCreateProject = async () => {
  if (!newProject.value.name) {
    alert('Назва проекту обов`язкова');
    return;
  }
  
  try {
    await api.createProject(newProject.value);
    newProject.value = { name: '', description: '' };
    closeCreateModal();
    // update after closing modal window
    await fetchProjects();
  } catch (err) {
    alert('Помилка при створенні проекта');
    console.error(err);
  }
};

const deleteProject = async (projectId) => {
  if (confirm('Ви впевнені, що хочете видалити цей проект?')) {
    try {
      await api.deleteProject(projectId);
      await fetchProjects();
    } catch (err) {
      alert('Не вдалося видалити проект');
    }
  }
};

onMounted(() => {
  fetchProjects();
  const modalEl = document.getElementById('createProjectModal'); 
  if (modalEl) {
    createModalInstance = new Modal(modalEl);
  }
});
</script>