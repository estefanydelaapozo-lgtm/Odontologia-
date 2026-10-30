# Estado de SIGO Dental

El proyecto contiene una API Go/Fiber, interfaz React/TypeScript, PostgreSQL, migraciones SQL, Docker Compose, OpenAPI y módulos de agenda, pacientes, clínica, pagos e inventario.

## Validación efectuada

- `go test ./...`: correcto.
- `go vet ./...`: correcto.
- `npm run build`: correcto.
- `docker compose config -q` y `docker compose --profile full build`: correctos.
- Perfil Docker completo (PostgreSQL, Redis, API y frontend): saludable; API conectada a PostgreSQL (`GET /api/health`: base de datos `ok`).
- Swagger/OpenAPI: HTTP 200; endpoint privado sin autenticación: HTTP 401.
- Protocolos HTTP Meta WhatsApp/Twilio probados con servidores simulados; SMTP sin STARTTLS se rechaza; plazo configurable de edición SOEP con pruebas de límites.

## Alcance no certificado como terminado

Consulta el estado trazable por requisito en [REQUIREMENTS_CHECKLIST.md](./REQUIREMENTS_CHECKLIST.md). Siguen pendientes validación end-to-end con roles y datos, pruebas de seguridad y cobertura de endpoints, carga de 150 usuarios, medición de latencia/uptime y UAT en navegadores. El odontograma gráfico aún es esquemático. Envíos reales de mensajería requieren credenciales de Meta/Twilio/SMTP; autorización tributaria y cobros reales requieren servicios externos homologados.

## Datos y claves

El ZIP de entrega excluye `backend/.env`, claves y datos clínicos locales. Crea `backend/.env` a partir de `.env.example`, establece secretos propios antes de iniciar, y no cambies `DATA_ENCRYPTION_KEY` una vez guardados datos clínicos. Mantén activadas las opciones demo únicamente en ambientes de prueba y sin datos reales.

La guía de instalación y resolución de errores está en [GUIA_EJECUCION_LOCAL.md](./GUIA_EJECUCION_LOCAL.md).
