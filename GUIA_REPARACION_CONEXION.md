# SIGO Dental — reparación de conexión Docker

Esta versión conserva la estructura del proyecto y configura el frontend para enviar las solicitudes `/api` mediante Nginx al servicio `api` dentro de la red Docker. Esto evita depender de `localhost:3000` desde el navegador y corrige el error típico `Failed to fetch` cuando la API no es accesible directamente desde el navegador.

## Aplicar la actualización

1. Haz una copia de seguridad de la carpeta actual del proyecto.
2. Extrae este ZIP en una carpeta nueva o copia los archivos modificados a tu proyecto actual:
   - `frontend/nginx.conf`
   - `frontend/Dockerfile`
   - `frontend/vite.config.ts`
   - `frontend/src/services/api.ts`
   - `backend/docker-compose.yml`
3. Abre PowerShell en la carpeta `backend` del proyecto actualizado.
4. Reconstruye solo frontend y backend, sin borrar volúmenes:

```powershell
docker compose --profile full up -d --build api frontend
```

5. Comprueba:
   - Frontend: http://localhost:5173
   - Salud de API a través de Nginx: http://localhost:5173/api/health
   - Salud directa de API: http://localhost:3000/api/health

No uses `docker compose down -v`; puede eliminar volúmenes de datos.

El frontend utiliza `/api` en el mismo origen y Nginx lo reenvía al servicio `api:3000`; para `npm run dev`, Vite configura un proxy equivalente. Si el puerto 5173 está ocupado, configura `FRONTEND_PORT` en `backend/.env` y abre el puerto elegido. Si se accede desde otro origen, configura `FRONTEND_ORIGIN` para que coincida.

## Si persiste `Failed to fetch`

Revisa los registros de `sigo-dental-backend` y `sigo-dental-frontend` en Docker Desktop. La respuesta de `/api/health` debe llegar a través del frontend. Si falla, el registro del contenedor indicará si la API no inicia por configuración, migraciones o conexión a PostgreSQL.

Esta reparación corrige el enrutamiento de la API en Docker; no certifica por sí sola que todas las funciones del PDF hayan pasado pruebas de aceptación.
