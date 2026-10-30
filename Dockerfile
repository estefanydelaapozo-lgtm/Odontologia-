# Imagen única de despliegue para Render: compila React y Go y sirve ambos bajo el mismo dominio.
FROM node:20-alpine AS frontend-build
WORKDIR /frontend
COPY frontend/package.json frontend/package-lock.json ./
RUN npm ci
COPY frontend/ ./
ARG VITE_API_URL=/api
ENV VITE_API_URL=${VITE_API_URL}
RUN npm run build

FROM golang:1.23-alpine AS backend-build
WORKDIR /src
COPY backend/go.mod backend/go.sum ./
RUN go mod download
COPY backend/ ./
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/sigo-dental .

FROM nginx:1.27-alpine
RUN apk add --no-cache ca-certificates wget \
    && addgroup -S -g 10001 sigo \
    && adduser -S -D -H -u 10001 -G sigo sigo \
    && mkdir -p /app/var/clinical /usr/share/nginx/html \
    && chown -R sigo:sigo /app /usr/share/nginx/html

COPY --from=backend-build /out/sigo-dental /app/sigo-dental
COPY --from=frontend-build /frontend/dist /usr/share/nginx/html
COPY deploy/nginx.conf.template /etc/nginx/templates/default.conf.template
COPY deploy/start.sh /app/start.sh
RUN chmod +x /app/start.sh && chown sigo:sigo /app/start.sh /app/sigo-dental

ENV PORT=10000
ENV BACKEND_PORT=3000
EXPOSE 10000
ENTRYPOINT ["/app/start.sh"]
