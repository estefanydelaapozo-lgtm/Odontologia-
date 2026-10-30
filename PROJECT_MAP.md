# Mapa del proyecto SIGO Dental

La raíz separa el servidor Go del cliente React, siguiendo la organización de MultiCatálogo: rutas, controladores, modelos/reglas de dominio, persistencia PostgreSQL y vistas por componente.

## Backend

| Ruta | Responsabilidad |
|---|---|
| `backend/main.go` | Arranque, migraciones, dependencias, TLS 1.3 opcional y apagado ordenado |
| `backend/config/` | Variables de entorno, PostgreSQL y ejecución versionada de migraciones |
| `backend/database/migrations/` | Esquema clínico, financiero, inventario, seguridad y trabajos externos |
| `backend/routes/routes.go` | Rutas, permisos por rol y requisito MFA |
| `backend/controllers/` | Manejadores HTTP de cuentas, pacientes, clínica, agenda, tratamientos, caja e inventario |
| `backend/controllers/payments_gateway.go` | Checkout externo, firma HMAC e idempotencia de webhooks |
| `backend/repository/` | Interfaz e implementación de persistencia SQL |
| `backend/models/` | Modelos compartidos |
| `backend/domain/` | Conversión FDI/Universal, superficies, NIC y cálculo de honorarios |
| `backend/auth/`, `backend/middleware/` | JWT, MFA, RBAC, auditoría y expiración por inactividad |
| `backend/security/` | AES-256-GCM, HMAC y firmas Ed25519 |
| `backend/workers/` | Outbox de notificaciones y envío fiscal configurable |
| `backend/openapi.yaml` | Especificación del API |

## Frontend

| Ruta | Responsabilidad |
|---|---|
| `frontend/src/App.tsx` | Sesión, navegación, permisos visibles y composición general |
| `frontend/src/components/ClinicalWorkspace.tsx` | Odontograma, periodontograma, SOEP, consentimientos y adjuntos |
| `frontend/src/components/ManagementWorkspace.tsx` | Presupuestos, agenda de tratamientos, inventario, caja y honorarios |
| `frontend/src/components/SignaturePad.tsx` | Captura de firma manuscrita en pantalla |
| `frontend/src/services/api.ts` | Cliente HTTP autenticado, carga/descarga y renovación JWT |
| `frontend/src/style.css` | Estilos responsive |

## Capas y flujo

```text
React (App y componentes)
        ↓ HTTP/JWT
Fiber routes → middleware (RBAC/MFA/auditoría) → controllers
                                                   ↓
                                         repository PostgreSQL
                                                   ↓
                                    domain cuando aplica reglas
```

Los datos clínicos cifrados usan una clave AES externa a PostgreSQL; los adjuntos se cifran antes de guardarse en el directorio privado. `REQUIREMENTS_CHECKLIST.md` relaciona estas capacidades con el SRS del profesor. El SRS deja DICOM 3D para una fase posterior y los requisitos de infraestructura/UAT se mantienen como pendientes de verificación.
