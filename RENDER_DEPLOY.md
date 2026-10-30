# SIGO Dental — despliegue en Render

Este paquete conserva el código de la aplicación y agrega únicamente los archivos necesarios para desplegarla como un único Web Service en Render.

## Qué hace

- React/Vite se compila durante el build.
- Go/Fiber se compila durante el build.
- Nginx sirve React y hace proxy de `/api/*` al backend.
- Frontend y backend quedan bajo el mismo dominio, por lo que el frontend usa `/api`.
- Las migraciones PostgreSQL se ejecutan automáticamente al arrancar el backend.
- Los archivos clínicos se guardan en el Persistent Disk de Render.
- `ENABLE_DEMO_USERS` y `ENABLE_DEMO_DATA` quedan desactivados.

## Render

1. Sube este proyecto a GitHub.
2. En Render: **New → Blueprint**.
3. Selecciona el repositorio y aplica `render.yaml`.
4. Completa los secretos marcados como `sync: false`.
5. Para correo SMTP, coloca tu proveedor y credenciales reales.

### Gmail

Para Gmail/Google Workspace usa normalmente:

- `SMTP_HOST=smtp.gmail.com`
- `SMTP_PORT=587`
- `SMTP_USERNAME=tu-correo@gmail.com`
- `SMTP_PASSWORD=App Password de Google`
- `SMTP_FROM=tu-correo@gmail.com`

No uses la contraseña normal de Gmail si la cuenta utiliza verificación en dos pasos; usa una App Password.

### Importante sobre Render Free

El servicio necesita un plan pagado si vas a enviar correo por SMTP, porque Render bloquea los puertos SMTP 25, 465 y 587 en Web Services Free. Además, el disco persistente requiere un servicio pagado.
