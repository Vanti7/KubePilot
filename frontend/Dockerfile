FROM node:20-alpine AS builder
WORKDIR /app
COPY package.json package-lock.json* ./
RUN npm ci
COPY . .
RUN npm run build

FROM nginx:alpine
COPY --from=builder /app/dist /usr/share/nginx/html
# nginx:alpine entrypoint processes *.template files via envsubst before starting
COPY nginx.conf /etc/nginx/templates/default.conf.template
EXPOSE 80
