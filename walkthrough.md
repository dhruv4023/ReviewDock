# Project Walkthrough - GitHub PR Rebase Manager

This document summarizes the generated scaffolding and provides instructions for running and compiling the desktop application.

---

## 1. What was Scaffolded

We have generated a fully structured, modular cross-platform desktop codebase incorporating:
- **`main.go` & `app.go`**: Bootstrapping, Wails binding methods (auth, queue submissions, and path validations).
- **`backend/models/`**: Unified Go structs mapping sessions, repository configs, and settings.
- **`backend/storage/`**: Thread-safe file-locked JSON reading and writing.
- **`backend/git/`**: OS-independent subprocess execution of Git CLI commands, streaming live logs.
- **`backend/github/`**: REST API client fetching PRs, check runs, and handling GitHub Device Authorization flow.
- **`backend/queue/`**: Thread-safe concurrent worker pool processing parallel rebases.
- **`frontend/`**: Vite + ReactJS framework pre-configured with TailwindCSS, Zustand state stores, and `xterm.js` terminal logging components.

---

## 2. Setup & Compilation Guide

Since Wails relies on native build environments, compile the project using these steps:

### Prerequisites
Make sure you have the following installed on your machine:
1. **Go** (v1.20+)
2. **NodeJS** (v18+) & **NPM**
3. **Wails CLI** (Install using: `go install github.com/wailsapp/wails/v2/cmd/wails@latest`)

### Steps to Run (Development Mode)
1. Navigate to the root project directory:
   ```bash
   cd github-pr
   ```
2. Run the Wails development server. This starts the backend debugger and watches the Vite frontend:
   ```bash
   wails dev
   ```
3. A desktop application window will open. Changes to Go files or React code will live-reload automatically.

### Steps to Build (Production Release)
To package the application as a standalone executable:
```bash
wails build
```
This generates:
- Linux: An executable file in `build/bin/`
- macOS: A `.app` bundle
- Windows: A `.exe` file

---

## 3. GitHub OAuth App Registration

To hook up authorization:
1. Go to your GitHub profile **Settings** -> **Developer Settings** -> **OAuth Apps** -> **New OAuth App**.
2. Set:
   - **Application Name**: `PR Rebase Manager`
   - **Homepage URL**: `https://github.com` (can be anything)
   - **Authorization callback URL**: `https://github.com` (since we use the **Device Flow**, this is required but not visited).
3. Copy the **Client ID**.
4. Paste it into the `DefaultClientID` constant in [backend/github/oauth.go](file:///home/odoo/Files/github-pr/backend/github/oauth.go#L11).
