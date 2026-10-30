# Portal del paciente y recepción

- El paciente autenticado envía una solicitud con fecha, hora y motivo mediante `POST /api/patient-appointment-requests`; se persiste en PostgreSQL.
- Desde la migración `012_encrypted_patient_requests.sql`, los motivos de solicitudes nuevas se cifran con `DATA_ENCRYPTION_KEY`; los registros anteriores se conservan sin modificación.
- El paciente consulta solo sus propias solicitudes mediante `GET /api/patient-appointment-requests`. El estado se vuelve a consultar al abrir “Mis solicitudes” y cada 30 segundos mientras esa vista está abierta.
- Recepción consulta solicitudes con el mismo `GET`, y puede confirmarlas o rechazarlas con una nota opcional mediante `PATCH /api/patient-appointment-requests/:id`. La bandeja se actualiza cada 30 segundos.
- Confirmar una solicitud no crea una cita automáticamente. Recepción debe revisar disponibilidad y programar la cita desde la agenda.
- Los datos de solicitudes no se almacenan en `localStorage`; el navegador conserva únicamente datos de sesión/autenticación.

El envío de confirmaciones por correo, WhatsApp o SMS depende de configurar y validar un proveedor externo; el cambio de estado no acredita que el mensaje se haya enviado.
