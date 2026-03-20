// Dual-mode transport: Wails desktop vs standalone browser.
//
// In a Wails desktop build, window.go is injected by the Wails runtime.
// App.GetServerURL() returns the URL of the embedded HTTP gateway (random port).
// In a standalone browser build, VITE_BACKEND_URL or localhost:8080 is used.

declare global {
  interface Window {
    go?: { main: { App: { GetServerURL: () => Promise<string> } } }
  }
}

let _baseURL: string | null = null

async function resolveBaseURL(): Promise<string> {
  if (window.go?.main?.App?.GetServerURL) {
    return await window.go.main.App.GetServerURL()
  }
  return import.meta.env.VITE_BACKEND_URL ?? 'http://localhost:8080'
}

export async function getBaseURL(): Promise<string> {
  if (_baseURL) return _baseURL
  _baseURL = await resolveBaseURL()
  return _baseURL
}
