# Checklist breve de entrega SIGO Dental

- **Completado y probado:** pruebas Go `go test ./...`, `go vet ./...`, compilación TypeScript/Vite y construcción de imágenes Docker.
- **Completado y probado:** API Docker saludable conectada a PostgreSQL; 11 migraciones aplicadas, health check `200`, OpenAPI `200` y ruta privada sin sesión `401`.
- **Completado y probado:** perfil Docker completo con PostgreSQL, Redis, API y frontend saludables; página web responde `200`.
- **Completado y probado:** protocolos HTTP Meta/Twilio con servidor simulado; SMTP sin STARTTLS se rechaza; plazo SOEP configurable con valores inválidos rechazados.
- **Implementado, pendiente de validación:** persistencia, RBAC/MFA, auditoría, anamnesis estructurada, agenda día/semana con filtros, periodontograma, caja ciega, reglas de agenda y flujo paciente-recepción. No se realizó una transacción autenticada de extremo a extremo en la base persistente compartida.
- **Parcial:** figura esquemática del odontograma (requiere revisión clínica), TLS/cifrado integral y pruebas de cobertura crítica.
- **Pendiente:** carga 150 usuarios, umbral <1,5 s, uptime 99,9 %, UAT en navegadores, pruebas SAST/DAST e integración automatizada/cobertura >=80 % en endpoints críticos.
- **Implementado, pendiente de validación / Bloqueado por servicio externo:** adaptadores directos Meta WhatsApp, Twilio SMS y SMTP STARTTLS; protocolos HTTP probados con mocks. Entrega real pendiente de credenciales, cuentas y pruebas del proveedor. SRI y cobros reales también requieren proveedor y homologación.
- **ZIP:** `SIGO-Dental.zip`, código y migraciones incluidos; `.env`, secretos, datos clínicos locales, `node_modules`, `dist` y cachés excluidos.

El detalle verificable y los comandos están en [REQUIREMENTS_CHECKLIST.md](./REQUIREMENTS_CHECKLIST.md) y [GUIA_EJECUCION_LOCAL.md](./GUIA_EJECUCION_LOCAL.md). SIGO Dental no se declara terminado: quedan los requisitos parciales, pendientes y bloqueados listados arriba.
