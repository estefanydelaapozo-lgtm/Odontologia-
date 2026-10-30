package main

import (
	_ "embed"
	"context"
	"crypto/tls"
	"log"
	"os/signal"
	"syscall"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/cors"
	"github.com/gofiber/fiber/v2/middleware/recover"
	"sigo-dental/backend/config"
	"sigo-dental/backend/controllers"
	"sigo-dental/backend/database"
	"sigo-dental/backend/repository"
	"sigo-dental/backend/routes"
	"sigo-dental/backend/security"
	"sigo-dental/backend/workers"
)

// openAPISpec se sirve en /api/docs/openapi.yaml; /api/docs muestra Swagger UI (carga swagger-ui-dist desde un CDN).
//
//go:embed openapi.yaml
var openAPISpec []byte

const swaggerPage = `<!doctype html><html lang="es"><head><meta charset="utf-8"><title>SIGO Dental · API</title>
<link rel="stylesheet" href="https://unpkg.com/swagger-ui-dist@5/swagger-ui.css"></head><body><div id="ui"></div>
<script src="https://unpkg.com/swagger-ui-dist@5/swagger-ui-bundle.js"></script>
<script>window.ui=SwaggerUIBundle({url:'/api/docs/openapi.yaml',dom_id:'#ui'})</script></body></html>`

func main() {
	config.LoadEnv()
	db, err := config.ConnectDB()
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()
	if err = config.Migrate(db, database.Migrations); err != nil {
		log.Fatal("migración: ", err)
	}
	secret := []byte(config.Getenv("JWT_SECRET", ""))
	if len(secret) < 32 {
		log.Fatal("JWT_SECRET debe tener al menos 32 caracteres")
	}
	cipher, err := security.NewCipher(config.Getenv("DATA_ENCRYPTION_KEY", ""))
	if err != nil {
		log.Fatal(err)
	}
	repo := repository.NewPostgresRepo(db)
	controllers.Initialize(repo, secret, cipher)
	if err = controllers.SeedAdmin(); err != nil {
		log.Fatal("usuario inicial: ", err)
	}
	if err = controllers.SeedDemoUsers(); err != nil {
		log.Fatal("usuarios demo: ", err)
	}
	if err = controllers.SeedDemoData(); err != nil {
		log.Fatal("datos demo: ", err)
	}
	app := fiber.New(fiber.Config{AppName: "SIGO Dental API", BodyLimit: 16 * 1024 * 1024})
	app.Use(recover.New())
	app.Use(cors.New(cors.Config{AllowOrigins: config.Getenv("FRONTEND_ORIGIN", "http://localhost:5173"), AllowHeaders: "Origin, Content-Type, Accept, Authorization", ExposeHeaders: "X-Session-Token"}))
	app.Get("/api/docs/openapi.yaml", func(c *fiber.Ctx) error { c.Type("yaml"); return c.Send(openAPISpec) })
	app.Get("/api/docs", func(c *fiber.Ctx) error { c.Type("html"); return c.SendString(swaggerPage) })
	routes.SetupRoutes(app, secret, repo)
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	go workers.StartNotifications(ctx, repo, cipher)
	go workers.StartFiscal(ctx, repo)
	errCh := make(chan error, 1)
	cert, key := config.Getenv("TLS_CERT_FILE", ""), config.Getenv("TLS_KEY_FILE", "")
	if cert != "" && key != "" {
		app.Server().TLSConfig = &tls.Config{MinVersion: tls.VersionTLS13}
		go func() { errCh <- app.ListenTLS(":"+config.Getenv("PORT", "3000"), cert, key) }()
	} else {
		go func() { errCh <- app.Listen(":" + config.Getenv("PORT", "3000")) }()
	}
	select {
	case err = <-errCh:
		if err != nil {
			log.Fatal(err)
		}
	case <-ctx.Done():
		if e := app.Shutdown(); e != nil {
			log.Printf("apagado: %v", e)
		}
	}
}
