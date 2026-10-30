# Credenciales demo de SIGO Dental en Render

Estas son las cuentas que crea el sistema cuando `ENABLE_DEMO_USERS=true`.

Contraseña para todas: `123456`

- Administrador: `admin@odontologia.local`
- Recepción: `recepcion@odontologia.local`
- Odontólogo: `odontologo@odontologia.local`
- Asistente: `asistente@odontologia.local`
- Auditoría: `auditor@odontologia.local`
- Paciente: `paciente@odontologia.local`

## IMPORTANTE: primera configuración en Render

El proyecto necesita una `DATA_ENCRYPTION_KEY` válida porque el sistema cifra información clínica.

En Render, crea el Blueprint desde `render.yaml`. Cuando Render solicite el valor de `DATA_ENCRYPTION_KEY`, coloca una cadena Base64 que represente exactamente 32 bytes.

No cambies esa clave después de haber guardado datos cifrados.

Los correos SMTP son opcionales para iniciar sesión. Para que funcionen los recordatorios y OTP por correo, configura `SMTP_HOST`, `SMTP_USERNAME`, `SMTP_PASSWORD` y `SMTP_FROM` en Render.
