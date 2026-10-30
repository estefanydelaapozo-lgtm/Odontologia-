# Revisión técnica de SIGO Dental

## Resultado de la revisión estática

- La solución conserva la separación `frontend/` y `backend/` y las capas de rutas, controladores, repositorio, dominio, seguridad y trabajadores.
- Se excluyeron del paquete las cachés de Go/npm, artefactos de compilación y el archivo `backend/.env`.
- Se ajustó `backend/config/db.go` para que no use silenciosamente una URL PostgreSQL con credenciales conocidas si falta `DATABASE_URL`. La URL debe venir de la configuración local o del entorno de despliegue.
- Se hizo genérica la ruta de ejemplo en la guía de ejecución para no depender de una carpeta de usuario concreta.
- Se reforzó `.gitignore` para evitar incluir archivos de entorno y artefactos.

## Validación que sí se intentó

- `go test ./...`: no se pudo completar porque el entorno de revisión no pudo descargar dependencias de `proxy.golang.org` (fallo de red/DNS). No se debe interpretar como una prueba superada.
- `npm ci` y `npm run build`: la instalación no terminó dentro del tiempo disponible en el entorno de revisión; el build del frontend queda pendiente de ejecución local.
- No se pudo ejecutar una prueba integrada con PostgreSQL, Docker, credenciales de proveedores ni datos de prueba.

## Antes de declarar cumplimiento total del SRS

1. Crear `backend/.env` copiando `backend/.env.example`; cambiar `ADMIN_PASSWORD`, `JWT_SECRET` y `DATA_ENCRYPTION_KEY`. No compartir `.env`.
2. Ejecutar `docker compose up -d --build` desde `backend/` y comprobar que PostgreSQL, API y frontend quedan saludables.
3. Ejecutar `go test ./...` y `npm ci && npm run build` en un equipo con acceso a las dependencias.
4. Probar RBAC y MFA con cada rol; verificar cierre por inactividad, auditoría, adendas e inmutabilidad clínica.
5. Probar agenda con intentos concurrentes de reservar el mismo sillón/profesional.
6. Probar odontograma FDI/Universal, periodontograma, planes parciales, cobros mixtos, caja ciega y descuento de inventario en transacción.
7. Configurar proveedores reales de facturación y mensajería y certificar los flujos. Configurar proveedor de pago y validar webhook en sandbox.
8. Ejecutar pruebas de carga (150 usuarios), medir el tiempo de carga del expediente/odontograma y realizar pruebas de disponibilidad y compatibilidad en Chrome, Firefox, Edge y Safari.
9. Confirmar la jurisdicción tributaria y la normativa aplicable antes de usar datos reales de pacientes.

## Estado honesto

El código y la interfaz constituyen una base amplia, pero no puede afirmarse cumplimiento total del SRS hasta completar las pruebas de integración, rendimiento, seguridad, compatibilidad y certificación de servicios externos. Las capturas de pantalla muestran la interfaz; no prueban por sí solas la persistencia ni los controles de seguridad.


## Corrección adicional de migraciones
Se corrigió la comprobación del bloqueo advisory de PostgreSQL en `backend/config/migrate.go`: el resultado de `pg_advisory_lock` se comprobaba con `IS NULL`, lo que podía informar falsamente que no se obtuvo el bloqueo. Ahora se comprueba con `IS NOT NULL`.

## Cambio añadido: solicitudes de citas del portal del paciente
- Se añadió la migración `011_patient_appointment_requests.sql` para persistir solicitudes en PostgreSQL.
- El portal del paciente ahora consulta y crea solicitudes mediante la API autenticada; ya no usa `localStorage` para almacenar solicitudes.
- Recepción puede consultar solicitudes desde el panel de inicio, actualizar su estado a confirmada/rechazada y dejar una nota visible para el paciente.
- Rutas: `GET/POST /api/patient-appointment-requests` y `PATCH /api/patient-appointment-requests/:id`.
- Validación pendiente: no se pudieron ejecutar `go test ./...` ni `npm run build` en este entorno porque las dependencias no estaban disponibles y no se pudo acceder a los registries externos. La integración debe probarse localmente con Docker/PostgreSQL.
