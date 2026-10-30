# Guía de ejecución local — SIGO Dental

## Requisitos

- Docker Desktop con Docker Compose
- Para ejecución fuera de Docker: Go 1.23+ y Node.js 20+

## Arranque completo con Docker (PowerShell)

```powershell
cd backend
Copy-Item .env.example .env
```

Edita `.env`: establece una contraseña inicial única, reemplaza `JWT_SECRET` por un valor aleatorio de al menos 32 caracteres y `DATA_ENCRYPTION_KEY` por Base64 de exactamente 32 bytes aleatorios. Por ejemplo, genera la clave de cifrado así y copia el resultado al archivo:

```powershell
$bytes = [byte[]]::new(32)
$rng = [Security.Cryptography.RandomNumberGenerator]::Create()
$rng.GetBytes($bytes)
[Convert]::ToBase64String($bytes)
$rng.Dispose()
```

No cambies la clave de cifrado después de guardar datos, salvo que migres los datos cifrados. No compartas `.env`.

```powershell
docker compose --profile full up -d --build
docker compose --profile full ps
Invoke-RestMethod http://localhost:3000/api/health
```

Abre `http://localhost:5173`. Swagger está en `http://localhost:3000/api/docs`. Las migraciones se ejecutan al iniciar la API. PostgreSQL, Redis y archivos clínicos están en volúmenes persistentes.

## Arranque separado para desarrollo

Primero crea y configura `backend/.env` como en el paso anterior. En una terminal:

```powershell
cd backend
docker compose --profile full stop api frontend
docker compose up -d
go run .
```

El comando `stop` evita conflictos si antes se inició el perfil Docker completo: la API y el frontend en contenedores usan los mismos puertos que el arranque local. `docker compose up -d` inicia PostgreSQL y Redis. No hay `database/init.sql`; `go run .` ejecuta las migraciones versionadas de `backend/database/migrations`. No necesitas `go mod tidy` para arrancar mientras `go.mod` y `go.sum` estén presentes.

En otra terminal, inicia el frontend:

```powershell
cd frontend
npm install
npm run dev
```

Abre el frontend en `http://localhost:5173`. Si Vite indica otro puerto, detén el frontend del perfil Docker antes de volver a ejecutar `npm run dev`.

El frontend local usa `http://localhost:3000/api`; configura `VITE_API_URL` en `frontend/.env` si la API se sirve en otra dirección. No ejecutes simultáneamente `go run .` y el contenedor `api`: ambos usan el puerto 3000.

`CLINICAL_NOTE_EDIT_HOURS` fija el plazo de edición de notas SOEP sin firmar (1–720 horas; valor predeterminado: 24). Las notas firmadas no se editan; las correcciones se agregan como adendas.

## Detener y preservar los datos

```powershell
cd backend
docker compose down
```

Este comando conserva los volúmenes. No uses `docker compose down -v`: elimina los datos persistidos. Los volúmenes no sustituyen respaldos; respalda PostgreSQL y el volumen `sigo_clinical_files`, y conserva la clave de cifrado por separado.

## Datos de demostración

En `.env`, `ENABLE_DEMO_USERS=true` habilita las cuentas demo predecibles `admin@odontologia.local`, `recepcion@odontologia.local`, `odontologo@odontologia.local`, `asistente@odontologia.local`, `auditor@odontologia.local` y `paciente@odontologia.local` (contraseña `123456`). `ENABLE_DEMO_DATA=true` carga registros ficticios. Activar estas opciones puede actualizar las contraseñas demo existentes; úsalas solo con una base local desechable y sin datos reales. Ambas opciones deben permanecer desactivadas en producción.

## Solución de errores

- Si Docker no inicia, abre Docker Desktop y espera que PostgreSQL muestre estado `healthy` en `docker compose ps`.
- Si el puerto 5432 está ocupado, cambia el puerto publicado de PostgreSQL y actualiza `DATABASE_URL` para ejecuciones locales.
- Si aparece `users_role_check`, conserva la base de datos y reinicia la API: las migraciones 008/009 reparan esa restricción.
- Si la API rechaza `DATA_ENCRYPTION_KEY`, genera una clave Base64 de 32 bytes. No reemplaces una clave que ya protege datos existentes.
- Si faltan paquetes frontend, ejecuta `npm install` desde `frontend/`.
- No se utiliza Redis como caché en la aplicación actual.

## Servicios externos

Las confirmaciones de cita y los recordatorios 48/24 horas se guardan en la outbox cifrada cuando el paciente autorizó el contacto. Configura el canal y las credenciales del proveedor para que el worker los entregue. No pongas secretos en el frontend:

- WhatsApp Cloud API de Meta: `REMINDER_CHANNEL=whatsapp` o `all`; define `WHATSAPP_PHONE_NUMBER_ID`, `WHATSAPP_ACCESS_TOKEN`, `WHATSAPP_TEMPLATE_NAME` y `WHATSAPP_TEMPLATE_LANGUAGE`. El nombre debe ser una plantilla aprobada por Meta con un parámetro de texto en el cuerpo. Los destinatarios deben tener formato internacional y haber aceptado recibir mensajes.
- SMS Twilio: `REMINDER_CHANNEL=sms` o `all`; define `TWILIO_ACCOUNT_SID`, `TWILIO_AUTH_TOKEN` y `TWILIO_FROM_NUMBER`, además de verificar remitente/destino en la consola del proveedor si se usa una cuenta de prueba.
- Correo SMTP: `REMINDER_CHANNEL=email` o `all`; define `SMTP_HOST`, `SMTP_PORT`, `SMTP_FROM`, y las credenciales `SMTP_USERNAME`/`SMTP_PASSWORD` si el proveedor las requiere. El sistema exige STARTTLS y rechaza el envío si el servidor no ofrece cifrado. `SMTP_TIMEOUT_SECONDS` admite valores de 1 a 60.
- Adaptador existente: también se pueden usar `MESSAGING_API_URL` y `MESSAGING_API_TOKEN`. Si ambos están definidos, ese endpoint maneja todos los canales con el contrato JSON `{channel,to,template,payload}`.

Los formatos de las solicitudes HTTP de Meta y Twilio se probaron localmente con servidores simulados. **No se enviaron mensajes reales**: hacen falta cuentas, plantillas aprobadas, remitentes, consentimiento y credenciales de los proveedores.

- Facturación electrónica de Ecuador: `FISCAL_API_URL` y `FISCAL_API_TOKEN`, además de proveedor autorizado, certificados y pruebas de homologación. La cola no equivale a una autorización del SRI.
- Pagos: `PAYMENT_API_URL`, `PAYMENT_API_TOKEN` y `PAYMENT_WEBHOOK_SECRET`; configura un proveedor compatible y certifica el flujo en su entorno de pruebas.

No se afirman envíos reales, autorización tributaria, transacciones reales, SLA ni capacidad de carga sin credenciales y validación del proveedor/entorno correspondiente.

## Estado de validación

Los comandos probados en la copia del proyecto se registran en [REQUIREMENTS_CHECKLIST.md](./REQUIREMENTS_CHECKLIST.md). Los requisitos de carga, disponibilidad, seguridad dinámica, navegadores y certificación externa requieren pruebas específicas de despliegue y no se consideran verificados por compilar.
