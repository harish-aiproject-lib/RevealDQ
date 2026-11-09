package main

import (
	"context"
	"log"
	"net/http"
	"os"

	"github.com/go-chi/chi/v5"
	chimw "github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/cors"
	"github.com/harish-aiproject-lib/AgenticDataGov/backend/internal/config"
	"github.com/harish-aiproject-lib/AgenticDataGov/backend/internal/db"
	"github.com/harish-aiproject-lib/AgenticDataGov/backend/internal/handlers"
	"github.com/harish-aiproject-lib/AgenticDataGov/backend/internal/middleware"
	"github.com/harish-aiproject-lib/AgenticDataGov/backend/internal/voc"
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

	if err := db.EnsureSchema(ctrlDB); err != nil {
		log.Printf("ensure schema: %v", err)
	}

	if err := db.EnsureAdmin(ctrlDB, cfg.AdminEmail, cfg.AdminPassword); err != nil {
		log.Printf("ensure admin: %v", err)
	}

	if config.InsecureSecret(cfg.JWTSecret) && os.Getenv("ALLOW_INSECURE_DEV") != "1" {
		log.Fatal("JWT_SECRET is a placeholder. Set a long random JWT_SECRET in .env (ALLOW_INSECURE_DEV=1 only for a local demo).")
	}

	h := handlers.New(ctrlDB, srcDB, cfg)
	if n, err := h.MigratePlaintextSecrets(); err != nil {
		log.Printf("connection secret migration: %v", err)
	} else if n > 0 {
		log.Printf("encrypted %d connection secret(s) at rest", n)
	}

	// ---- Reveal VoC: own database (created if missing) + automation engine ----
	vocCfg := voc.Config{
		AgentURL:             cfg.AgentURL,
		JWTSecret:            cfg.JWTSecret,
		UIURL:                cfg.UIURL,
		LinkedInClientID:     cfg.LinkedInClientID,
		LinkedInClientSecret: cfg.LinkedInClientSecret,
		LinkedInRedirectURI:  cfg.LinkedInRedirectURI,
		LinkedInAccountEmail: cfg.LinkedInAccountEmail,
	}
	var vocH *voc.Handler
	if err := voc.EnsureDatabase(ctrlDB, cfg.VocDBName); err != nil {
		log.Printf("warning: voc database: %v", err)
	} else if vocDB, err := db.Open(cfg.MySQLDSNVoc); err != nil {
		log.Printf("warning: voc db unavailable: %v", err)
	} else {
		defer vocDB.Close()
		if err := voc.EnsureSchema(vocDB); err != nil {
			log.Printf("warning: voc schema: %v", err)
		}
		store := voc.NewStore(vocDB)
		engine := voc.NewEngine(store, vocCfg)
		engine.StartScheduler(context.Background())
		vocH = voc.NewHandler(store, engine, vocCfg)
		log.Printf("Reveal VoC ready on database %s (scheduler running)", cfg.VocDBName)
	}

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
		w.Write([]byte(`{"status":"ok","service":"reality-reveal-api","platform":"Reality Reveal","modules":["DQ","VOC"]}`))
	})

	r.Route("/api/v1", func(api chi.Router) {
		api.Post("/auth/register", h.Register)
		api.Post("/auth/login", h.Login)
		api.Post("/internal/connection-handoff", h.RedeemHandoff)

		// LinkedIn redirects the browser here after consent (no JWT — state is HMAC-signed).
		if vocH != nil {
			api.Get("/voc/linkedin/oauth/callback", vocH.OAuthCallback)
		}

		api.Group(func(priv chi.Router) {
			priv.Use(middleware.JWT(cfg.JWTSecret))
			priv.Get("/auth/me", h.Me)

			// Reality Reveal product-module catalog (Reveal DQ, Reveal VoC, ...)
			priv.Get("/products", h.ListProducts)

			priv.Get("/workspaces", h.ListWorkspaces)
			priv.Post("/workspaces", h.CreateWorkspace)
			priv.Get("/workspaces/{id}", h.GetWorkspace)

			priv.Get("/workspaces/{id}/connections", h.ListConnections)
			priv.Post("/workspaces/{id}/connections", h.CreateConnection)
			priv.Post("/workspaces/{id}/connections/{connId}/test", h.TestConnection)
			priv.Delete("/workspaces/{id}/connections/{connId}", h.DeleteConnection)

			priv.Get("/workspaces/{id}/rules/search", h.SearchRules)
			priv.Get("/workspaces/{id}/rules", h.ListRules)
			priv.Post("/workspaces/{id}/rules", h.CreateRule)
			priv.Patch("/workspaces/{id}/rules/{ruleId}", h.UpdateRule)
			priv.Put("/workspaces/{id}/rules/{ruleId}", h.EditRule)
			priv.Delete("/workspaces/{id}/rules/{ruleId}", h.DeleteRule)
			priv.Get("/workspaces/{id}/audit", h.ListRuleAudit)

			priv.Get("/workspaces/{id}/pipelines", h.ListPipelines)
			priv.Post("/workspaces/{id}/pipelines/{pipelineId}/run", h.RunPipeline)
			priv.Get("/workspaces/{id}/runs", h.ListRuns)
			priv.Get("/workspaces/{id}/dashboard", h.Dashboard)

			priv.Get("/workspaces/{id}/acl", h.GetACL)
			priv.Put("/workspaces/{id}/acl", h.UpdateACL)

			priv.Post("/workspaces/{id}/analyser/chat", h.AnalyserChat)
			priv.Post("/workspaces/{id}/etl/analyze", h.EtlAnalyze)
			priv.Post("/workspaces/{id}/etl/explain-node", h.EtlExplainNode)
			priv.Post("/workspaces/{id}/conversion/analyze", h.ConversionAnalyze)
			priv.Post("/workspaces/{id}/conversion/run", h.ConversionRun)
			priv.Post("/workspaces/{id}/conversion/checkin", h.ConversionCheckin)
			priv.Post("/workspaces/{id}/conversion/deploy", h.ConversionDeploy)
			priv.Post("/workspaces/{id}/conversion/job", h.ConversionJob)
			priv.Post("/workspaces/{id}/conversion/audit", h.SaveConversionAudit)
			priv.Get("/workspaces/{id}/conversion/audit", h.ListConversionAudit)

			// Reveal VoC module
			if vocH != nil {
				priv.Route("/workspaces/{id}/voc", vocH.Routes)
			}
		})
	})

	addr := ":" + cfg.APIPort
	log.Printf("Reality Reveal API listening on %s", addr)
	if err := http.ListenAndServe(addr, r); err != nil {
		log.Fatal(err)
	}
}
