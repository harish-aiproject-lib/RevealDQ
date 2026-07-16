package main

import (
	"log"
	"net/http"

	"github.com/go-chi/chi/v5"
	chimw "github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/cors"
	"github.com/harish-aiproject-lib/AgenticDataGov/backend/internal/config"
	"github.com/harish-aiproject-lib/AgenticDataGov/backend/internal/db"
	"github.com/harish-aiproject-lib/AgenticDataGov/backend/internal/handlers"
	"github.com/harish-aiproject-lib/AgenticDataGov/backend/internal/middleware"
	"github.com/joho/godotenv"
)

func main() {
	_ = godotenv.Load("../.env")
	_ = godotenv.Load(".env")

	cfg := config.Load()
	ctrlDB, err := db.Open(cfg.MySQLDSNCtrl)
	if err != nil {
		log.Fatalf("ctrl db: %v", err)
	}
	defer ctrlDB.Close()

	srcDB, err := db.Open(cfg.MySQLDSNSource)
	if err != nil {
		log.Printf("warning: source db unavailable: %v", err)
	} else {
		defer srcDB.Close()
	}

	if err := db.EnsureAdmin(ctrlDB, cfg.AdminEmail, cfg.AdminPassword); err != nil {
		log.Printf("ensure admin: %v", err)
	}

	h := handlers.New(ctrlDB, srcDB, cfg)
	r := chi.NewRouter()
	r.Use(chimw.Logger)
	r.Use(chimw.Recoverer)
	r.Use(cors.Handler(cors.Options{
		AllowedOrigins:   []string{"http://localhost:4200", "http://127.0.0.1:4200"},
		AllowedMethods:   []string{"GET", "POST", "PUT", "PATCH", "DELETE", "OPTIONS"},
		AllowedHeaders:   []string{"Accept", "Authorization", "Content-Type"},
		AllowCredentials: true,
	}))

	r.Get("/health", func(w http.ResponseWriter, _ *http.Request) {
		w.Write([]byte(`{"status":"ok","service":"reveal-dq-api"}`))
	})

	r.Route("/api/v1", func(api chi.Router) {
		api.Post("/auth/register", h.Register)
		api.Post("/auth/login", h.Login)

		api.Group(func(priv chi.Router) {
			priv.Use(middleware.JWT(cfg.JWTSecret))
			priv.Get("/auth/me", h.Me)

			priv.Get("/workspaces", h.ListWorkspaces)
			priv.Post("/workspaces", h.CreateWorkspace)
			priv.Get("/workspaces/{id}", h.GetWorkspace)

			priv.Get("/workspaces/{id}/connections", h.ListConnections)
			priv.Post("/workspaces/{id}/connections", h.CreateConnection)
			priv.Post("/workspaces/{id}/connections/{connId}/test", h.TestConnection)

			priv.Get("/workspaces/{id}/rules", h.ListRules)
			priv.Post("/workspaces/{id}/rules", h.CreateRule)
			priv.Patch("/workspaces/{id}/rules/{ruleId}", h.UpdateRule)

			priv.Get("/workspaces/{id}/pipelines", h.ListPipelines)
			priv.Post("/workspaces/{id}/pipelines/{pipelineId}/run", h.RunPipeline)
			priv.Get("/workspaces/{id}/runs", h.ListRuns)
			priv.Get("/workspaces/{id}/dashboard", h.Dashboard)

			priv.Get("/workspaces/{id}/acl", h.GetACL)
			priv.Put("/workspaces/{id}/acl", h.UpdateACL)

			priv.Post("/workspaces/{id}/analyser/chat", h.AnalyserChat)
		})
	})

	addr := ":" + cfg.APIPort
	log.Printf("Reveal DQ API listening on %s", addr)
	if err := http.ListenAndServe(addr, r); err != nil {
		log.Fatal(err)
	}
}
