# RefratIA

Monorepo full-stack do RefratIA:

- `frontend/`: aplicação React + Vite.
- `backend/`: servidor Go nativo, atualmente com `GET /health`.

## Desenvolvimento

```bash
npm install
npm run dev          # frontend
npm run dev:backend  # backend, em outro terminal
```

## Validação

```bash
npm run build
npm run lint
npm run build:backend
```
