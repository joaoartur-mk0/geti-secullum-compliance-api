# Estágio 1: build do Vite
FROM node:22-alpine AS build
WORKDIR /app
COPY package.json package-lock.json ./
RUN npm ci
COPY . .
# Endereço da API assado no bundle (variável VITE_* é resolvida em build time).
# Passe via build arg no compose de produção: build.args.VITE_API_URL=https://<APP_DOMAIN>.
# Se vazio, o código cai no padrão http://localhost:8080.
ARG VITE_API_URL=""
ENV VITE_API_URL=$VITE_API_URL
RUN npm run build

# Estágio 2: nginx servindo o build estático
FROM nginx:alpine
COPY nginx.conf /etc/nginx/conf.d/default.conf
COPY --from=build /app/dist /usr/share/nginx/html
EXPOSE 80
