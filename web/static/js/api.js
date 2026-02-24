/**
 * API wrapper for Marstaff backend
 * Provides a clean interface for all API calls
 */

const API_BASE = window.location.origin;

// API wrapper with error handling
class API {
    /**
     * Make a fetch request with error handling
     */
    async request(url, options = {}) {
        try {
            const response = await fetch(url, {
                ...options,
                headers: {
                    'Content-Type': 'application/json',
                    ...options.headers,
                },
            });

            if (!response.ok) {
                let errorData = {};
                try {
                    errorData = await response.json();
                } catch {}
                throw new Error(errorData.error || `HTTP ${response.status}`);
            }

            return await response.json();
        } catch (error) {
            console.error('API request failed:', error);
            throw error;
        }
    }

    // ===== Sessions API =====
    async createSession(data) {
        return this.request(`${API_BASE}/api/sessions`, {
            method: 'POST',
            body: JSON.stringify(data),
        });
    }

    async getSession(sessionId) {
        return this.request(`${API_BASE}/api/sessions/${sessionId}`);
    }

    async listSessions(params = {}) {
        const query = new URLSearchParams(params).toString();
        return this.request(`${API_BASE}/api/sessions?${query}`);
    }

    async updateSession(sessionId, data) {
        return this.request(`${API_BASE}/api/sessions/${sessionId}`, {
            method: 'PATCH',
            body: JSON.stringify(data),
        });
    }

    async deleteSession(sessionId) {
        return this.request(`${API_BASE}/api/sessions/${sessionId}`, {
            method: 'DELETE',
        });
    }

    async addMessage(sessionId, data) {
        return this.request(`${API_BASE}/api/sessions/${sessionId}/messages`, {
            method: 'POST',
            body: JSON.stringify(data),
        });
    }

    async getMessages(sessionId, limit = 100) {
        return this.request(`${API_BASE}/api/sessions/${sessionId}/messages?limit=${limit}`);
    }

    // ===== Projects API =====
    async createProject(data) {
        return this.request(`${API_BASE}/api/projects`, {
            method: 'POST',
            body: JSON.stringify(data),
        });
    }

    async getProject(projectId) {
        return this.request(`${API_BASE}/api/projects/${projectId}`);
    }

    async listProjects(params = {}) {
        const query = new URLSearchParams(params).toString();
        return this.request(`${API_BASE}/api/projects?${query}`);
    }

    async updateProject(projectId, data) {
        return this.request(`${API_BASE}/api/projects/${projectId}`, {
            method: 'PATCH',
            body: JSON.stringify(data),
        });
    }

    async deleteProject(projectId) {
        return this.request(`${API_BASE}/api/projects/${projectId}`, {
            method: 'DELETE',
        });
    }

    async getProjectTemplates() {
        return this.request(`${API_BASE}/api/projects/templates`);
    }

    async getProjectSessions(projectId) {
        return this.request(`${API_BASE}/api/projects/${projectId}/sessions`);
    }

    // ===== Memory API =====
    async setMemory(userId, data) {
        return this.request(`${API_BASE}/api/memory/${userId}`, {
            method: 'POST',
            body: JSON.stringify(data),
        });
    }

    async getMemory(userId, category = '') {
        return this.request(`${API_BASE}/api/memory/${userId}${category ? `?category=${category}` : ''}`);
    }

    // ===== Workspace API (deprecated, use Projects API instead) =====
    async createWorkspace(data) {
        return this.request(`${API_BASE}/api/workspaces`, {
            method: 'POST',
            body: JSON.stringify(data),
        });
    }
}

// Export singleton instance
const api = new API();
