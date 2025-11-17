<template>
  <div>
    <div v-if="loading" class="text-center">
        <div class="spinner-border" role="status"><span class="visually-hidden">Завантаження...</span></div>
    </div>
    <div v-else-if="error">
      <div class="alert alert-danger">{{ error }}</div>
    </div>
    <div v-else>
      <div class="d-flex justify-content-between align-items-center mb-4">
        <div>
          <h1>{{ project.Name }}</h1>
          <p class="text-muted">{{ project.Description }}</p>
        </div>
        <div>
          <button class="btn btn-success me-2" @click="openCreateTicketModal">Створити тикет</button>
          <button class="btn btn-info" @click="openAddMemberModal">Додати учасника</button>
        </div>
      </div>

      <!-- Kanban Board -->
      <div class="row">
        <!-- New Column -->
        <div class="col">
          <h3>Нові</h3>
          <div class="ticket-column">
            <div v-for="ticket in newTickets" :key="ticket.ID" class="card mb-2">
              <div class="card-body">
                <h5 class="card-title">{{ ticket.Title }}</h5>
                <p class="card-text">{{ ticket.Description }}</p>
                <span :class="['badge', priorityVariant(ticket.Priority)]">{{ ticket.Priority }}</span>
                <div class="mt-2">
                  <button class="btn btn-primary btn-sm me-1" @click="updateTicketStatus(ticket.ID, 'in_progress')">
                    У роботу
                  </button>
                  <button class="btn btn-success btn-sm" @click="updateTicketStatus(ticket.ID, 'done')">
                    Виконано
                  </button>
                </div>
              </div>
            </div>
            <div v-if="newTickets.length === 0" class="text-muted text-center p-3">
              Немає тикетів
            </div>
          </div>
        </div>

        <!-- In Progress Column -->
        <div class="col">
          <h3>У роботі</h3>
           <div class="ticket-column">
            <div v-for="ticket in inProgressTickets" :key="ticket.ID" class="card mb-2">
               <div class="card-body">
                <h5 class="card-title">{{ ticket.Title }}</h5>
                <p class="card-text">{{ ticket.Description }}</p>
                <span :class="['badge', priorityVariant(ticket.Priority)]">{{ ticket.Priority }}</span>
                <div class="mt-2">
                  <button class="btn btn-secondary btn-sm me-1" @click="updateTicketStatus(ticket.ID, 'new')">
                    Повернути
                  </button>
                  <button class="btn btn-success btn-sm" @click="updateTicketStatus(ticket.ID, 'done')">
                    Виконано
                  </button>
                </div>
              </div>
            </div>
            <div v-if="inProgressTickets.length === 0" class="text-muted text-center p-3">
              Немає тикетів
            </div>
          </div>
        </div>

        <!-- Done Column -->
        <div class="col">
          <h3>Готові</h3>
           <div class="ticket-column">
            <div v-for="ticket in doneTickets" :key="ticket.ID" class="card mb-2">
               <div class="card-body">
                <h5 class="card-title">{{ ticket.Title }}</h5>
                <p class="card-text">{{ ticket.Description }}</p>
                <span :class="['badge', priorityVariant(ticket.Priority)]">{{ ticket.Priority }}</span>
                <div class="mt-2">
                  <button class="btn btn-warning btn-sm" @click="updateTicketStatus(ticket.ID, 'in_progress')">
                    Повернути у роботу
                  </button>
                </div>
              </div>
            </div>
            <div v-if="doneTickets.length === 0" class="text-muted text-center p-3">
              Немає тикетів
            </div>
          </div>
        </div>
      </div>
    </div>

    <div class="modal fade" id="createTicketModal" tabindex="-1">
      <div class="modal-dialog">
        <div class="modal-content">
          <div class="modal-header">
            <h5 class="modal-title">Новий тикет</h5>
            <button type="button" class="btn-close" @click="closeCreateTicketModal"></button>
          </div>
          <div class="modal-body">
            <form @submit.prevent="handleCreateTicket">
                <div class="mb-3">
                  <label class="form-label">Заголовок:</label>
                  <input class="form-control" v-model="newTicket.title" required>
                </div>
                <div class="mb-3">
                  <label class="form-label">Опис:</label>
                  <textarea class="form-control" v-model="newTicket.description" rows="3"></textarea>
                </div>
                <div class="mb-3">
                  <label class="form-label">Пріоритет:</label>
                  <select class="form-select" v-model="newTicket.priority">
                    <option value="low">Низький</option>
                    <option value="medium">Середній</option>
                    <option value="high">Високий</option>
                  </select>
                </div>
                <div class="mb-3">
                  <label class="form-label">Призначити на (ID):</label>
                  <input type="number" class="form-control" v-model.number="newTicket.assignee_id" placeholder="Не обов'язково">
                </div>
            </form>
          </div>
          <div class="modal-footer">
            <button type="button" class="btn btn-secondary" @click="closeCreateTicketModal">Відміна</button>
            <button type="button" class="btn btn-primary" @click="handleCreateTicket">Створити</button>
          </div>
        </div>
      </div>
    </div>

    <div class="modal fade" id="addMemberModal" tabindex="-1">
      <div class="modal-dialog">
        <div class="modal-content">
          <div class="modal-header">
            <h5 class="modal-title">Додати учасника</h5>
            <button type="button" class="btn-close" @click="closeAddMemberModal"></button>
          </div>
          <div class="modal-body">
            <form @submit.prevent="handleAddMember">
                <div class="mb-3">
                  <label class="form-label">ID Користувача:</label>
                  <input type="number" class="form-control" v-model.number="newMember.user_id" required>
                </div>
                <div class="mb-3">
                  <label class="form-label">Роль:</label>
                  <select class="form-select" v-model="newMember.role">
                    <option value="worker">Виконавець</option>
                    <option value="manager">Менеджер</option>
                  </select>
                </div>
            </form>
          </div>
          <div class="modal-footer">
            <button type="button" class="btn btn-secondary" @click="closeAddMemberModal">Відміна</button>
            <button type="button" class="btn btn-primary" @click="handleAddMember">Додати</button>
          </div>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup>
import { ref, onMounted, computed } from 'vue';
import { useRoute } from 'vue-router';
import api from '@/services/api';
import { Modal } from 'bootstrap';

const route = useRoute();
const projectId = route.params.projectId;
const project = ref({});
const tickets = ref([]);
const loading = ref(true);
const error = ref('');
const newTicket = ref({ title: '', description: '', priority: 'medium', assignee_id: null });
const newMember = ref({ user_id: null, role: 'worker'});
let createTicketModalInstance = null;
let addMemberModalInstance = null;

const fetchData = async () => {
  loading.value = true;
  error.value = '';
  try {
    const [projectResponse, ticketsResponse] = await Promise.all([
      api.getProject(projectId),
      api.getTickets(projectId)
    ]);
    project.value = projectResponse.data;
    tickets.value = ticketsResponse.data || [];
  } catch (err) {
    error.value = 'Не вдалося завантажити дані проекта';
    console.error('Помилка завантаження:', err);
  } finally {
    loading.value = false;
  }
};

const newTickets = computed(() => tickets.value.filter(t => t.Status === 'new'));
const inProgressTickets = computed(() => tickets.value.filter(t => t.Status === 'in_progress'));
const doneTickets = computed(() => tickets.value.filter(t => t.Status === 'done'));

const priorityVariant = (priority) => {
  if (priority === 'high') return 'bg-danger';
  if (priority === 'medium') return 'bg-warning text-dark';
  return 'bg-info text-dark';
};

// update ticket status
const updateTicketStatus = async (ticketId, newStatus) => {
  try {
    await api.updateTicket(ticketId, { status: newStatus });
    
    const ticket = tickets.value.find(t => t.ID === ticketId);
    if (ticket) {
      ticket.Status = newStatus;
    }
  } catch (err) {
    console.error('Помилка оновлення статусу:', err);
    alert('Не вдалося оновити статус тикета');
  }
};

// modal windows controls
const openCreateTicketModal = () => {
  if (createTicketModalInstance) {
    createTicketModalInstance.show();
  }
};

const closeCreateTicketModal = () => {
  if (createTicketModalInstance) {
    createTicketModalInstance.hide();
  }
};

const openAddMemberModal = () => {
  if (addMemberModalInstance) {
    addMemberModalInstance.show();
  }
};

const closeAddMemberModal = () => {
  if (addMemberModalInstance) {
    addMemberModalInstance.hide();
  }
};

const handleCreateTicket = async () => {
    if (!newTicket.value.title) { 
      alert('Заголовок обов`язковий'); 
      return; 
    }
    try {
        await api.createTicket(projectId, newTicket.value);
        newTicket.value = { title: '', description: '', priority: 'medium', assignee_id: null };
        closeCreateTicketModal();
        await fetchData();
    } catch (err) { 
      console.error('Помилка створення тикета:', err);
      alert('Помилка при створенні тикета'); 
    }
};

const handleAddMember = async () => {
    if (!newMember.value.user_id) { 
      alert('ID користувача обов`язковий'); 
      return; 
    }
    try {
        await api.addProjectMember(projectId, newMember.value);
        newMember.value = { user_id: null, role: 'worker'};
        closeAddMemberModal();
        alert('Учасника додано!');
    } catch (err) { 
      console.error('Помилка додавання учасника:', err);
      alert('Помилка при призначенні учасника'); 
    }
};

onMounted(() => {
  fetchData();
  const ticketModalEl = document.getElementById('createTicketModal');
  if(ticketModalEl) createTicketModalInstance = new Modal(ticketModalEl);
  const memberModalEl = document.getElementById('addMemberModal');
  if(memberModalEl) addMemberModalInstance = new Modal(memberModalEl);
});
</script>

<style scoped>
.ticket-column {
  background-color: #f4f5f7;
  border-radius: 3px;
  padding: 8px;
  min-height: 400px;
}

.card {
  transition: box-shadow 0.2s ease;
}

.card:hover {
  box-shadow: 0 2px 8px rgba(0, 0, 0, 0.15);
}

.btn-sm {
  font-size: 0.8rem;
  padding: 0.25rem 0.5rem;
}
</style>