# Development Guide: Go + Vite Monorepo with Hot Reload

This document explains how to set up a Go + Vite monorepo for development with real-time frontend preview while keeping the backend running.

## Menu

- [Development Guide: Go + Vite Monorepo with Hot Reload](#development-guide-go--vite-monorepo-with-hot-reload)
  - [Menu](#menu)
  - [Overview](#overview)
  - [Architecture](#architecture)
    - [Production Mode](#production-mode)
    - [Development Mode](#development-mode)
  - [Setup Instructions](#setup-instructions)
    - [Prerequisites](#prerequisites)
    - [Step 1: Start the Go Backend](#step-1-start-the-go-backend)
    - [Step 2: Start the Vite Dev Server](#step-2-start-the-vite-dev-server)
    - [Step 3: Configure NGINX (Optional, for HTTPS)](#step-3-configure-nginx-optional-for-https)
    - [Step 4: Access the Application](#step-4-access-the-application)
  - [Key Configuration Files](#key-configuration-files)
    - [`web/vite.config.ts` (Production)](#webviteconfigts-production)
    - [`web/vite.config.dev.ts` (Development)](#webviteconfigdevts-development)
    - [`web/package.json`](#webpackagejson)
    - [`Makefile`](#makefile)
  - [How It Works](#how-it-works)
    - [Vite Proxy Configuration](#vite-proxy-configuration)
    - [Hot Module Replacement (HMR)](#hot-module-replacement-hmr)
    - [Production Build](#production-build)
  - [Troubleshooting](#troubleshooting)
    - [API Requests Return 404](#api-requests-return-404)
    - [HMR Not Working](#hmr-not-working)
    - [CORS Errors](#cors-errors)
    - [Port Already in Use](#port-already-in-use)
  - [Comparison: Before vs After](#comparison-before-vs-after)
    - [Before (Old Workflow)](#before-old-workflow)
    - [After (New Workflow)](#after-new-workflow)
  - [Summary](#summary)


## Overview

This project is a monorepo where:
- **Backend**: Go (Gin framework) serves the API and static files in production
- **Frontend**: React + Vite SPA located in `web/`

The challenge: How to develop the frontend with hot reload while the backend stays running, and still deploy as a single Go binary?

## Architecture

### Production Mode

```
┌─────────────────────────────────────────────────────┐
│                   Go Binary                          │
│  ┌───────────────┐    ┌──────────────────────────┐  │
│  │   API Routes  │    │   Static File Server     │  │
│  │  /gptchat/api │    │   (serves web/dist/)     │  │
│  └───────────────┘    └──────────────────────────┘  │
└─────────────────────────────────────────────────────┘
           ▲
           │ HTTPS
           │
      ┌────┴────┐
      │ Browser │
      └─────────┘
```

In production:
1. `make build` compiles the frontend into `web/dist/`
2. The Go binary serves both API and static files
3. Single process, single port

### Development Mode

```
┌─────────────────┐      ┌───────────────────┐
│  Go Backend     │      │  Vite Dev Server  │
│  (port 24456)   │◄─────│  (port 25173)     │
│  - API only     │ proxy│  - React app      │
└─────────────────┘      │  - HMR enabled    │
                         └───────────────────┘
                                  ▲
                                  │ HTTPS
                                  │
                         ┌────────┴────────┐
                         │      NGINX      │
                         │ (SSL termination)│
                         └────────┬────────┘
                                  │
                             ┌────┴────┐
                             │ Browser │
                             └─────────┘
```

In development:
1. Go backend runs independently (only handles API)
2. Vite dev server serves the frontend with Hot Module Replacement (HMR)
3. Vite proxies API requests to the Go backend
4. Frontend changes appear instantly without rebuilding or restarting

## Setup Instructions

### Prerequisites

- Go 1.25+
- Node.js 20+
- pnpm (via corepack)
- NGINX (for SSL termination in test environments)

### Step 1: Start the Go Backend

```bash
go run -race main.go -c "/path/to/settings.yml" -t gptchat --debug
```

This starts the backend on port 24456 (default).

### Step 2: Start the Vite Dev Server

```bash
make dev
```

This runs `pnpm -C web run dev:proxy` which:
- Starts Vite on port 25173
- Uses `vite.config.dev.ts` (development-specific config)
- Proxies API requests to the Go backend

### Step 3: Configure NGINX (Optional, for HTTPS)

If you need HTTPS for testing browser features, configure NGINX:

```nginx
server {
    listen 443 ssl;
    server_name chat2.laisky.com;

    ssl_certificate /path/to/cert.crt;
    ssl_certificate_key /path/to/key.key;

    location / {
        proxy_pass http://127.0.0.1:25173;
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_buffering off;
    }
}
```

### Step 4: Access the Application

- **Direct (HTTP)**: http://localhost:25173/gptchat
- **Via NGINX (HTTPS)**: https://your-domain.com/gptchat

## Key Configuration Files

### `web/vite.config.ts` (Production)

Default Vite config for production builds:
- No base path (Go serves from root)
- Optimized for bundling

### `web/vite.config.dev.ts` (Development)

Development-specific Vite config:
- No base path (NGINX forwards directly)
- Proxy configuration for API requests
- HMR enabled

```typescript
export default defineConfig({
  plugins: [react()],
  server: {
    host: true,
    port: 25173,
    proxy: {
      // Proxy API requests to Go backend
      '^/gptchat/(api|user|audit|...)': {
        target: 'http://127.0.0.1:24456',
        changeOrigin: true,
      },
    },
  },
})
```

### `web/package.json`

Scripts:
- `dev`: Standard Vite dev server (for local development)
- `dev:proxy`: Development with API proxy (for testing with backend)
- `build`: Production build

### `Makefile`

- `make dev`: Runs `pnpm -C web run dev:proxy`
- `make build`: Builds frontend, ready for Go binary

## How It Works

### Vite Proxy Configuration

The key to this setup is Vite's proxy feature. When the frontend makes an API request (e.g., `/gptchat/api`), Vite intercepts it and forwards it to the Go backend:

```typescript
proxy: {
  '^/gptchat/(api|user|audit|audio|ramjet|oneapi|version|favicon\\.ico|create-payment-intent)': {
    target: 'http://127.0.0.1:24456',
    changeOrigin: true,
  },
}
```

### Hot Module Replacement (HMR)

Vite's HMR allows instant updates:
1. You edit a React component
2. Vite detects the change
3. Only the changed module is sent to the browser
4. The page updates without a full reload
5. React state is preserved when possible

### Production Build

When building for production:

```bash
make build
```

1. Vite compiles the React app to `web/dist/`
2. The Go binary embeds or serves `web/dist/`
3. No Vite server needed in production

## Troubleshooting

### API Requests Return 404

**Symptom**: Frontend loads but API calls fail.

**Solution**: Ensure the Go backend is running on the expected port (24456) and the Vite proxy patterns match your API routes.

### HMR Not Working

**Symptom**: Changes require manual page refresh.

**Solution**:
1. Check browser console for WebSocket errors
2. Ensure Vite server is running (`make dev`)
3. Check that NGINX isn't blocking WebSocket connections

### CORS Errors

**Symptom**: Browser blocks API requests with CORS errors.

**Solution**: Vite's proxy handles CORS in development. If you see CORS errors, requests may be going directly to the backend instead of through Vite.

### Port Already in Use

**Symptom**: "Port 25173 is in use" when starting Vite.

**Solution**:
```bash
# Find and kill processes using the port
lsof -i :25173
kill <PID>
```

## Comparison: Before vs After

### Before (Old Workflow)

1. Edit frontend code
2. Run `make build` (30+ seconds)
3. Restart Go server
4. Refresh browser
5. Test changes

**Total time per change**: ~1-2 minutes

### After (New Workflow)

1. Edit frontend code
2. Changes appear automatically

**Total time per change**: <1 second

## Summary

This setup provides the best of both worlds:
- **Development**: Instant feedback with HMR
- **Production**: Single Go binary with embedded static files
- **Testing**: Full API integration through Vite proxy

The key insight is using separate Vite configurations for development vs production, with the development config proxying API requests to the running Go backend.
