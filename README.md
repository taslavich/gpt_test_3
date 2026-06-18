# AI Prelander Builder MVP

Local MVP for generating multiple safe mock prelander pages for popunder traffic. Backend uses Go + Gin, frontend uses React + Vite + TypeScript, generated pages are rendered with Go `html/template` and saved in `backend/generated/`.

## Backend

```bash
cd backend
go mod tidy
go run ./cmd/server
```

Backend runs on http://localhost:8080.

## Frontend

```bash
cd frontend
npm install
npm run dev
```

Open http://localhost:5173.

## Features

- Generate 1-10 prelander variants across quiz, urgency, native article, minimal confirm, and calculator styles.
- Choose vertical, GEO, language, offer URL, and visual mode.
- Upload an image or generate a mock SVG creative.
- Preview generated HTML pages at `/preview/:prelander_id`.
- Store metadata in `backend/data/prelanders.json`.
