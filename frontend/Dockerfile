FROM node:22-alpine AS build

WORKDIR /app

COPY package.json package-lock.json ./
RUN npm ci

COPY frontend ./frontend

ARG VITE_API_URL=https://api.refratia.matbessa.com.br
ENV VITE_API_URL=$VITE_API_URL

ARG VITE_SHOW_DEMO_CASES=true
ENV VITE_SHOW_DEMO_CASES=$VITE_SHOW_DEMO_CASES

RUN npm run build -- --base=/

FROM nginx:1.27-alpine

COPY --from=build /app/frontend/dist /usr/share/nginx/html

RUN printf 'server {\n\
    listen 80;\n\
    server_name _;\n\
    root /usr/share/nginx/html;\n\
    index index.html;\n\
\n\
    location / {\n\
        try_files $uri $uri/ /index.html;\n\
    }\n\
}\n' > /etc/nginx/conf.d/default.conf

EXPOSE 80

HEALTHCHECK --interval=30s --timeout=5s --start-period=10s --retries=3 \
  CMD wget -q -O /dev/null http://127.0.0.1/ || exit 1

CMD ["nginx", "-g", "daemon off;"]
