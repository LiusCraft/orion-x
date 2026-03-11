FROM node:22-alpine AS builder

WORKDIR /app

COPY package.json package-lock.json ./
RUN npm ci

COPY . .

ARG VITE_MANAGER_API_BASE_URL=
ENV VITE_MANAGER_API_BASE_URL=${VITE_MANAGER_API_BASE_URL}

RUN npm run build

FROM nginx:1.27-alpine

COPY nginx-static.conf /etc/nginx/conf.d/default.conf
COPY --from=builder /app/dist /usr/share/nginx/html

EXPOSE 80
