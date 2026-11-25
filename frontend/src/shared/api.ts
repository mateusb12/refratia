export const API_URL = (import.meta.env.VITE_API_URL ?? '').trim().replace(/\/$/, '')
export const API_WRITE_TOKEN = import.meta.env.VITE_CASE_DELETE_TOKEN ?? 'local-dev-delete-only'
