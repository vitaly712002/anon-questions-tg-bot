// Клиент взаимодействия с REST API бота

const API_BASE = '/api';

function getHeaders() {
  const headers = { 'Content-Type': 'application/json' };
  const token = localStorage.getItem('token');
  if (token) {
    headers['Authorization'] = `Bearer ${token}`;
  }
  return headers;
}

export async function login(username, password) {
  const res = await fetch(`${API_BASE}/auth/login`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ username, password }),
  });
  const data = await res.json();
  if (!res.ok) {
    throw new Error(data.error || 'Ошибка входа');
  }
  localStorage.setItem('token', data.token);
  localStorage.setItem('user', JSON.stringify(data.user));
  return data;
}

export function logout() {
  localStorage.removeItem('token');
  localStorage.removeItem('user');
}

export function getCurrentUser() {
  const u = localStorage.getItem('user');
  return u ? JSON.parse(u) : null;
}

export async function getStats() {
  const res = await fetch(`${API_BASE}/stats`, { headers: getHeaders() });
  if (res.status === 401) { logout(); window.location.reload(); }
  return res.json();
}

export async function getQuestions(limit = 50, offset = 0) {
  const res = await fetch(`${API_BASE}/questions?limit=${limit}&offset=${offset}`, { headers: getHeaders() });
  if (res.status === 401) { logout(); window.location.reload(); }
  return res.json();
}

export async function getUsers() {
  const res = await fetch(`${API_BASE}/users`, { headers: getHeaders() });
  if (res.status === 401) { logout(); window.location.reload(); }
  return res.json();
}

export async function updateUserRole(userId, role) {
  const res = await fetch(`${API_BASE}/users/${userId}/role`, {
    method: 'POST',
    headers: getHeaders(),
    body: JSON.stringify({ role }),
  });
  return res.json();
}

export async function toggleUserBan(userId, banned) {
  const res = await fetch(`${API_BASE}/users/${userId}/ban`, {
    method: 'POST',
    headers: getHeaders(),
    body: JSON.stringify({ banned }),
  });
  return res.json();
}

export async function getSettings() {
  const res = await fetch(`${API_BASE}/settings`, { headers: getHeaders() });
  if (res.status === 401) { logout(); window.location.reload(); }
  return res.json();
}

export async function updateSettings(settings) {
  const res = await fetch(`${API_BASE}/settings`, {
    method: 'POST',
    headers: getHeaders(),
    body: JSON.stringify(settings),
  });
  return res.json();
}

export function getMediaUrl(fileId) {
  return `${API_BASE}/media/${fileId}`;
}
