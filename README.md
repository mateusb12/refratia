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

Por padrão, os arquivos confirmados são armazenados no bucket local `refratia`.

Para rodar o backend local contra o Tigris de produção, configure no `.env` os
mesmos valores de storage usados pelo backend publicado:

```env
BUCKET_NAME=<bucket-de-producao>
AWS_REGION=<regiao-do-tigris>
AWS_ACCESS_KEY_ID=<chave-do-tigris>
AWS_SECRET_ACCESS_KEY=<segredo-do-tigris>
AWS_ENDPOINT_URL_S3=<endpoint-s3-do-tigris>
AWS_PUBLIC_ENDPOINT_URL_S3=<endpoint-s3-publico-do-tigris>
CASE_DELETE_TOKEN=<token-aceito-pelo-backend-local>
```

O `compose.yaml` usa essas variáveis quando presentes e continua usando o
MinIO local como fallback quando elas não estão definidas. Com as credenciais
de produção configuradas, o backend local lista, envia e exclui objetos no
bucket de produção.

Após a análise, o backend guarda um rascunho em `drafts/{intakeId}`. A confirmação
envia somente o `intakeId`; o backend promove os documentos e grava
`paciente_compilado.json` por último, como marcador de um caso completo.

Para apagar um caso inteiro no ambiente local:

```bash
curl -X DELETE -H 'Authorization: Bearer local-dev-delete-only' http://localhost:3000/api/cases/{caseId}
```

Em produção, o endpoint só é habilitado quando `CASE_DELETE_TOKEN` estiver definido.

## Validação

```bash
npm run build
npm run lint
npm run build:backend
```
