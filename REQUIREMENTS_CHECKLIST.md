# SIGO Dental — estado verificable frente al SRS

Estados: **Completado y probado**, **Implementado, pendiente de validación**, **Parcial**, **Pendiente** y **Bloqueado por servicio externo**. Una compilación no demuestra por sí sola el flujo clínico completo ni su persistencia en producción.

## Resultados probados en esta copia

- **Completado y probado:** `go test ./...` desde `backend/`.
- **Completado y probado:** `npm run build` desde `frontend/` (TypeScript y Vite).
- **Completado y probado:** `docker compose config -q` y `docker compose --profile full build`.
- **Completado y probado:** PostgreSQL está saludable; el contenedor API quedó saludable y `GET /api/health` informó estado `ok` y base de datos `ok`.
- **Completado y probado:** el esquema informa 11 migraciones aplicadas, de `001_core.sql` a `011_patient_appointment_requests.sql`.
- **Completado y probado:** `GET /api/docs/openapi.yaml` respondió HTTP 200; el endpoint protegido de solicitudes sin autenticación respondió HTTP 401.
- **Completado y probado:** servicios PostgreSQL, Redis, API y frontend quedaron saludables con el perfil `full`; la página del frontend responde HTTP 200.
- **Completado y probado:** protocolos HTTP de Meta/Twilio con servidor simulado; SMTP sin STARTTLS se rechaza; plazo de edición SOEP valida límites y valor por defecto.
- **No ejecutado:** pruebas de integración autenticadas del flujo paciente-recepción, por no modificar la base persistente compartida durante el smoke test; SAST/DAST, cobertura crítica, carga de 150 usuarios, medición de latencia, pruebas de navegador y uptime.

## Requisitos funcionales del PDF

| Requisito | Estado | Implementación / brecha verificable |
|---|---|---|
| RF-01 Registro unificado | Implementado, pendiente de validación | Ficha con datos de contacto/emergencia, tutor y seguro en PostgreSQL. Falta validar el flujo completo de datos y reglas obligatorias. |
| RF-02 Anamnesis y alertas | Implementado, pendiente de validación | Antecedentes estructurados (cardiovascular, hipertensión, diabetes, anticoagulantes/coagulopatía, alergias críticas, asma, renal/hepática, inmunosupresión, embarazo y cirugía), notas heredadas conservadas, almacenamiento cifrado y advertencia visible al abrir el expediente. Falta UAT clínica sobre los datos y la presentación. |
| RF-03 Odontograma | Parcial | FDI/Universal, 32 permanentes y 20 temporales; diagrama de cinco superficies seleccionables y colores por condición, con persistencia existente. La figura es esquemática, no un odontograma anatómico completo, por lo que requiere revisión clínica. |
| RF-04 Periodontograma | Implementado, pendiente de validación | Modelo para seis sitios por pieza, margen, NIC, furca, movilidad y sangrado; falta integración clínica verificada contra PostgreSQL. |
| RF-05 Notas SOEP | Implementado, pendiente de validación | Notas cifradas, firma y adendas inmutables; plazo configurable de 1–720 horas para editar notas sin firmar, predeterminado 24 horas. Se probaron los límites de configuración. |
| RF-06 Consentimiento | Implementado, pendiente de validación | Plantillas versionadas, firma en pantalla y flujo OTP en cola. El envío del OTP requiere mensajería externa. |
| RF-07/08 Presupuestos | Implementado, pendiente de validación | Planes por fases/sesiones, costes, descuentos autorizados, aprobación parcial y conversión a cita; falta prueba integrada del flujo completo. |
| RF-09 Agenda | Implementado, pendiente de validación | Calendario día/semana, consulta del rango desde PostgreSQL y filtros por sillón/profesional; falta validar con personal clínico. |
| RF-10 Estados de cita | Implementado, pendiente de validación | Estados de negocio, con registro de procedimiento para completar atención; falta validar transiciones integradas. |
| RF-11 Notificaciones | Implementado, pendiente de validación / Bloqueado por servicio externo | Confirmación en outbox al programar citas (con consentimiento) y recordatorios 48/24 h; reintentos, errores y cifrado. Adaptadores directos Meta WhatsApp Cloud API (plantilla aprobada), Twilio SMS y SMTP STARTTLS, además del contrato HTTP genérico. Formatos HTTP Meta/Twilio probados con mocks. Envíos reales requieren cuentas, credenciales, remitentes verificados, consentimiento y pruebas de entrega. |
| RF-12 Facturación y cobros | Parcial / Bloqueado por servicio externo | Facturas, pagos mixtos/parciales y adaptador configurable. No hay emisión fiscal autorizada de Ecuador sin proveedor homologado y credenciales. El pago real requiere pasarela. |
| RF-13 Comisiones | Implementado, pendiente de validación | Reglas de honorarios y liquidaciones; cálculo de dominio cubierto por pruebas unitarias, falta integración financiera. |
| RF-14 Inventario | Implementado, pendiente de validación | Recetas de consumo, movimientos y descuento transaccional al realizar tratamiento; falta prueba integrada. |

## Reglas, seguridad y calidad

| Requisito | Estado | Nota |
|---|---|---|
| RN-01 Inmutabilidad clínica | Implementado, pendiente de validación | Procedimientos realizados y adendas protegidos en lógica/base de datos; faltan pruebas de integración adversariales. |
| RN-02 Cierre de caja ciego | Implementado, pendiente de validación | Conteo físico antes de mostrar el valor teórico; falta prueba integrada por rol. |
| RN-03 Sin solapamiento | Implementado, pendiente de validación | Trigger PostgreSQL con bloqueo para sillón y profesional; falta prueba de concurrencia en la base. |
| RNF-01 <1,5 s | Pendiente | No se midió en condiciones definidas por el SRS. |
| RNF-02 99,9 % | Pendiente | Requiere monitoreo y medición de disponibilidad del despliegue. |
| RNF-03 150 usuarios | Pendiente | `max_connections=200` y pool configurado no prueban capacidad; no se ejecutó prueba de carga. |
| RNF-04 Cifrado/TLS | Parcial | AES-GCM para datos clínicos/archivos y TLS 1.3 configurable; TLS no está habilitado por defecto en Compose y no se verificó cifrado integral de disco/base completa. |
| RNF-05 RBAC/MFA/sesión | Implementado, pendiente de validación | Roles, TOTP en áreas sensibles y timeout cliente de 15 minutos. Falta probar todos los permisos y expiración en integración. |
| RNF-06 Auditoría | Implementado, pendiente de validación | Registro append-only de solicitudes con actor/IP/fecha; falta cobertura demostrada de cada acceso y operación sensible. |
| RNF-07 Responsive | Implementado, pendiente de validación | Estilos responsive; falta UAT en resoluciones y tabletas del SRS. |
| RNF-08 Navegadores | Pendiente | No se ejecutó UAT en las dos últimas versiones de Chrome, Firefox, Edge y Safari. |
| OpenAPI 3.0 / Swagger | Implementado, pendiente de validación | Especificación disponible en `/api/docs/openapi.yaml`; citas documentan filtros de fecha; respuesta HTTP 200. No se hizo validación formal exhaustiva de todas las rutas. |
| Pruebas y cobertura | Parcial | `go test ./...`, `go vet ./...` y build frontend pasan. No se alcanza/demuestra 80 % en endpoints críticos; faltan pruebas de integración y análisis SAST/DAST. |

## Distribución e instalación

- **Completado y probado:** Compose válido; construcción de imágenes API/frontend completada.
- **Completado y probado:** perfil Docker completo con PostgreSQL, Redis, API y frontend saludables; el health check confirmó acceso a PostgreSQL.
- **Implementado, pendiente de validación:** migraciones `001`–`011`, volúmenes de datos, `.env.example` y carga demo separada mediante flags. No se recreó ni borró la base existente.
- El ZIP excluye `.env`, credenciales, `node_modules`, `dist`, cachés y archivos clínicos locales.

## Bloqueos y comandos

Usa [README.md](./README.md) o [GUIA_EJECUCION_LOCAL.md](./GUIA_EJECUCION_LOCAL.md) para instalación segura.

```powershell
cd backend
docker compose --profile full up -d --build
docker compose --profile full ps
```

```powershell
cd backend
go test ./...
```

```powershell
cd frontend
npm ci
npm run build
```

Antes de producción faltan: credenciales y entregas reales de mensajería, certificación SRI y pasarela; pruebas integradas de flujos y migraciones; revisión de seguridad; pruebas de carga/latencia y uptime; UAT de navegadores; y completar la figura anatómica del odontograma.
