# SIGO Dental

Sistema de gestión odontológica con API Go/Fiber, PostgreSQL, Redis y cliente React/Vite. Las migraciones se ejecutan al iniciar la API; PostgreSQL y los archivos clínicos usan volúmenes persistentes de Docker.

## Requisitos

- Docker Desktop con Docker Compose
- Para ejecución local sin contenedores: Go 1.23+ y Node.js 20+

## Instalación con Docker

En PowerShell:

```powershell
cd backend
Copy-Item .env.example .env
```

Edita `backend/.env` antes del primer arranque. Cambia `ADMIN_PASSWORD`, `JWT_SECRET` y `DATA_ENCRYPTION_KEY`; la clave de cifrado debe ser Base64 de exactamente 32 bytes. Puedes generarla con:

```powershell
$bytes = [byte[]]::new(32)
$rng = [Security.Cryptography.RandomNumberGenerator]::Create()
$rng.GetBytes($bytes)
[Convert]::ToBase64String($bytes)
$rng.Dispose()
```

Guarda el resultado como `DATA_ENCRYPTION_KEY` y configura un `JWT_SECRET` aleatorio de al menos 32 caracteres. Conserva la clave de cifrado de forma segura: perderla impide leer los datos clínicos cifrados.

```powershell
docker compose --profile full up -d --build
docker compose --profile full ps
Invoke-RestMethod http://localhost:3000/api/health
Invoke-RestMethod http://localhost:5173/api/health
```

Abre `http://localhost:5173`. El frontend usa rutas relativas `/api`; Nginx en el contenedor reenvía las solicitudes al backend en la red Docker. Para desarrollo separado, Vite aplica el mismo proxy (por defecto a `http://localhost:3000`, configurable con `VITE_DEV_API_PROXY`).

Para habilitar las seis cuentas de demostración en una instalación local, establece `ENABLE_DEMO_USERS=true` en `backend/.env` antes de iniciar la API. Todas usan `123456`: `admin@odontologia.local`, `recepcion@odontologia.local`, `odontologo@odontologia.local`, `asistente@odontologia.local`, `auditor@odontologia.local` y `paciente@odontologia.local`. No habilites estas cuentas en producción. En bases existentes, las cuentas que ya tengan MFA activado seguirán solicitando su código TOTP.

Si un puerto del equipo ya está ocupado, cambia el puerto de host correspondiente en `backend/.env`: `FRONTEND_PORT` (5173), `API_PORT` (3000), `POSTGRES_PORT` (5432) o `REDIS_PORT` (6379). Si cambias `FRONTEND_PORT` y accedes desde otro origen, actualiza también `FRONTEND_ORIGIN`. Reconstruye con `docker compose --profile full up -d --build`; no elimines volúmenes.

El usuario administrador inicial se define mediante `ADMIN_EMAIL` y `ADMIN_PASSWORD`. `ENABLE_DEMO_DATA=true` carga datos ficticios de demostración.

Para ejecución local separada:

```powershell
cd backend
docker compose --profile full stop api frontend
docker compose up -d
go run .
```

`docker compose --profile full stop api frontend` evita que los contenedores del perfil completo sigan ocupando los puertos 3000 y 5173. `docker compose up -d` inicia PostgreSQL y Redis; no se necesita `database/init.sql`, porque `go run .` aplica automáticamente las migraciones incluidas en `backend/database/migrations`. No hace falta ejecutar `go mod tidy` para iniciar una copia con `go.mod` y `go.sum` presentes.

En otra terminal, inicia el cliente web:

```powershell
cd frontend
npm install
npm run dev
```

Abre el frontend en `http://localhost:5173`. Si Vite indica otro puerto, detén el frontend del perfil Docker antes de volver a ejecutar `npm run dev`.

Comprueba el proxy de desarrollo en `http://localhost:5173/api/health` (o en el puerto alternativo configurado).

## Persistencia y respaldos

`docker compose down` conserva la base de datos, Redis y archivos clínicos. No ejecutes `docker compose down -v` si deseas conservarlos. Configura respaldos regulares de PostgreSQL, el volumen `sigo_clinical_files` y `DATA_ENCRYPTION_KEY`; el volumen por sí solo no constituye un respaldo.

## Integraciones externas

Confirmaciones de cita y recordatorios cifrados se encolan para los canales autorizados. Hay adaptadores directos configurables para WhatsApp Cloud API de Meta, Twilio SMS y SMTP con STARTTLS; faltan cuentas/credenciales para verificar envíos reales. Configura los campos `WHATSAPP_*`, `TWILIO_*` o `SMTP_*` descritos en `.env.example` y en la guía. También sigue disponible el adaptador HTTP genérico. Facturación electrónica de Ecuador y pasarela requieren proveedor/credenciales propios; no se emiten comprobantes autorizados ni cargos reales sin su homologación/configuración.

La implementación y los límites de validación se mantienen en [REQUIREMENTS_CHECKLIST.md](./REQUIREMENTS_CHECKLIST.md). Consulta [GUIA_EJECUCION_LOCAL.md](./GUIA_EJECUCION_LOCAL.md) para diagnóstico.
