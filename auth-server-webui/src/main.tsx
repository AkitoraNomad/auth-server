import { StrictMode } from 'react'
import { createRoot } from 'react-dom/client'
import { BrowserRouter } from 'react-router'
import './index.css'
import App from './App.tsx'
import { APP_BASE } from './lib/config'

createRoot(document.getElementById('root')!).render(
  <StrictMode>
    <BrowserRouter basename={APP_BASE || '/'}>
      <App />
    </BrowserRouter>
  </StrictMode>,
)
