import axios from 'axios';

export const api = axios.create({
  baseURL: process.env.NEXT_PUBLIC_API_URL || 'http://localhost:8080',
  headers: {
    'Content-Type': 'application/json',
  },
  timeout: 30000,
});

// Request interceptor for auth
api.interceptors.request.use(
  (config) => {
    // Token is added by auth store
    return config;
  },
  (error) => Promise.reject(error)
);

// Response interceptor for error handling
api.interceptors.response.use(
  (response) => response,
  (error) => {
    if (error.response?.status === 401) {
      // Token expired or invalid
      if (typeof window !== 'undefined') {
        localStorage.removeItem('auth-storage');
        window.location.href = '/login';
      }
    }
    return Promise.reject(error);
  }
);

// API functions
export const projectsApi = {
  list: () => api.get('/api/v1/projects'),
  get: (id: string) => api.get(`/api/v1/projects/${id}`),
  create: (data: { name: string; description?: string; repo_url?: string }) =>
    api.post('/api/v1/projects', data),
  update: (id: string, data: { name?: string; description?: string; repo_url?: string }) =>
    api.put(`/api/v1/projects/${id}`, data),
  delete: (id: string) => api.delete(`/api/v1/projects/${id}`),
};

export const environmentsApi = {
  list: (projectId: string) => api.get(`/api/v1/projects/${projectId}/environments`),
  get: (projectId: string, envId: string) => api.get(`/api/v1/projects/${projectId}/environments/${envId}`),
  create: (projectId: string, data: { name: string; namespace: string; cluster?: string; config?: Record<string, unknown> }) =>
    api.post(`/api/v1/projects/${projectId}/environments`, data),
  delete: (projectId: string, envId: string) => api.delete(`/api/v1/projects/${projectId}/environments/${envId}`),
  deploy: (projectId: string, envId: string, data: { commit_sha: string; branch?: string }) =>
    api.post(`/api/v1/projects/${projectId}/environments/${envId}/deploy`, data),
};

export const deploymentsApi = {
  list: (projectId: string) => api.get(`/api/v1/projects/${projectId}/deployments`),
  get: (projectId: string, deploymentId: string) => api.get(`/api/v1/projects/${projectId}/deployments/${deploymentId}`),
  getLogs: (projectId: string, deploymentId: string) => api.get(`/api/v1/projects/${projectId}/deployments/${deploymentId}/logs`),
};

export const aiApi = {
  listJobs: (projectId: string) => api.get(`/api/v1/projects/${projectId}/ai/jobs`),
  reviewPR: (projectId: string, data: { pr_number: number; repo_url: string; base_branch: string; head_branch: string; files?: string[]; focus_areas?: string[] }) =>
    api.post(`/api/v1/ai/review`, { ...data, project_id: projectId }),
  generateDocs: (projectId: string, data: { repo_url: string; paths?: string[]; doc_types?: string[]; format?: string }) =>
    api.post(`/api/v1/ai/docs`, { ...data, project_id: projectId }),
  generateTests: (projectId: string, data: { repo_url: string; paths?: string[]; test_types?: string[]; framework?: string; coverage?: number }) =>
    api.post(`/api/v1/ai/tests`, { ...data, project_id: projectId }),
  migrateCode: (projectId: string, data: { repo_url: string; source_lang: string; target_lang: string; paths?: string[]; preserve_tests?: boolean }) =>
    api.post(`/api/v1/ai/migrate`, { ...data, project_id: projectId }),
};

export const authApi = {
  login: (email: string, password: string) => api.post('/auth/login', { email, password }),
  register: (email: string, password: string, name: string) => api.post('/auth/register', { email, password, name }),
  me: () => api.get('/auth/me'),
};