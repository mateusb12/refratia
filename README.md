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

Defina `OPENAI_API_KEY` no ambiente do backend para extrair os documentos antes da confirmação. `OPENAI_MODEL` é opcional (padrão: `gpt-5.4-mini`).

### Stack local com Docker

Requer Docker com Compose:

```bash
docker compose up --build
```

- Frontend: http://localhost:5173
- API: http://localhost:3000/health
- MinIO: http://localhost:9001 (`minioadmin` / `minioadmin`)

Os arquivos confirmados são armazenados no bucket local `refratia`.

## Validação

```bash
npm run build
npm run lint
npm run build:backend
```
