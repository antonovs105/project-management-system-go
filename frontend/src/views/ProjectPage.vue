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
          <button class="btn btn-success me-2" data-bs-toggle="modal" data-bs-target="#createTicketModal">Створити тикет</button>
          <button class="btn btn-info" data-bs-toggle="modal" data-bs-target="#addMemberModal">Додати учасника</button>
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
                <button class="btn btn-success btn-sm mt-2" @click="markTicketAsDone(ticket.ID)">
                  Виконано
                </button>
              </div>
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
                <button class="btn btn-success btn-sm mt-2" @click="markTicketAsDone(ticket.ID)">
                  Виконано
                </button>
              </div>
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
              </div>
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
            <button type="button" class="btn-close" data-bs-dismiss="modal"></button>
          </div>
          <div class="modal-body">
            <form @submit.prevent="handleCreateTicket">
                <div class="mb-3"><label class="form-label">Заголовок:</label><input class="form-control" v-model="newTicket.title" required></div>
                <div class="mb-3"><label class="form-label">Опис:</label><textarea class="form-control" v-model="newTicket.description"></textarea></div>
                <div class="mb-3"><label class="form-label">Пріорітет:</label><select class="form-select" v-model="newTicket.priority"><option>low</option><option>medium</option><option>high</option></select></div>
                <div class="mb-3"><label class="form-label">Призначити на (ID):</label><input type="number" class="form-control" v-model.number="newTicket.assignee_id"></div>
            </form>
          </div>
          <div class="modal-footer">
            <button type="button" class="btn btn-secondary" data-bs-dismiss="modal">Відміна</button>
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
            <button type="button" class="btn-close" data-bs-dismiss="modal"></button>
          </div>
          <div class="modal-body">
            <form @submit.prevent="handleAddMember">
                <div class="mb-3"><label class="form-label">ID Користувача:</label><input type="number" class="form-control" v-model.number="newMember.user_id" required></div>
                <div class="mb-3"><label class="form-label">Роль:</label><select class="form-select" v-model="newMember.role"><option>worker</option><option>manager</option></select></div>
            </form>
          </div>
          <div class="modal-footer">
            <button type="button" class="btn btn-secondary" data-bs-dismiss="modal">Відміна</button>
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

const markTicketAsDone = (ticketId) => {
  const ticketToUpdate = tickets.value.find(t => t.ID === ticketId);
  if (ticketToUpdate) {
    ticketToUpdate.Status = 'done';
  }
};
// =========================

const handleCreateTicket = async () => {
    if (!newTicket.value.title) { alert('Заголовок обов`язковий'); return; }
    try {
        await api.createTicket(projectId, newTicket.value);
        newTicket.value = { title: '', description: '', priority: 'medium', assignee_id: null };
        createTicketModalInstance.hide();
        setTimeout(() => { fetchData(); }, 350);
    } catch (err) { alert('Помилка при створенні тикета'); }
}

const handleAddMember = async () => {
    if (!newMember.value.user_id) { alert('ID користувача обов`язковий'); return; }
    try {
        await api.addProjectMember(projectId, newMember.value);
        newMember.value = { user_id: null, role: 'worker'};
        addMemberModalInstance.hide();
        alert('Учасника додано!');
    } catch (err) { alert('Помилка при призначенні учасника'); }
}

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
</style>