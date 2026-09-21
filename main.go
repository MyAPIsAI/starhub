package main

import (
	"log"
	"net/http"
	"os"

	"starhub/common"
	"starhub/controller"
	"starhub/middleware"
	"starhub/model"
)

func main() {
	dbPath := os.Getenv("STARHUB_DB")
	if dbPath == "" {
		dbPath = "starhub.db"
	}
	port := os.Getenv("STARHUB_PORT")
	if port == "" {
		port = "3000"
	}
	if err := model.InitDB(dbPath); err != nil {
		log.Fatalf("failed to init database: %v", err)
	}
	if err := model.LoadOptionsToSettings(); err != nil {
		log.Fatalf("failed to load options: %v", err)
	}
	rootPassword := os.Getenv("STARHUB_ROOT_PASSWORD")
	if rootPassword == "" {
		rootPassword = common.DefaultRootPasswd
		log.Printf("WARNING: using default root password %q, set STARHUB_ROOT_PASSWORD to override", rootPassword)
	}
	if err := model.Seed(rootPassword); err != nil {
		log.Fatalf("failed to seed database: %v", err)
	}
	log.Printf("starhub %s listening on :%s (db: %s)", common.Version, port, dbPath)
	if err := http.ListenAndServe(":"+port, NewRouter()); err != nil {
		log.Fatal(err)
	}
}

// NewRouter wires the management API and the relay endpoints.
// Relay chain contract: TokenAuth (writes CtxUsingGroup) ->
// Distribute (writes CtxSelectedChannel) -> Relay (consumes both).
func NewRouter() *http.ServeMux {
	mux := http.NewServeMux()

	mux.HandleFunc("POST /api/user/login", controller.Login)
	mux.Handle("POST /api/user/logout", middleware.UserAuth(http.HandlerFunc(controller.Logout)))
	mux.Handle("GET /api/user/self", middleware.UserAuth(http.HandlerFunc(controller.GetSelf)))
	mux.Handle("GET /api/user", middleware.AdminAuth(http.HandlerFunc(controller.ListUsers)))
	mux.Handle("GET /api/user/", middleware.AdminAuth(http.HandlerFunc(controller.ListUsers)))
	mux.Handle("PUT /api/user", middleware.AdminAuth(http.HandlerFunc(controller.UpdateUser)))
	mux.Handle("PUT /api/user/", middleware.AdminAuth(http.HandlerFunc(controller.UpdateUser)))

	mux.Handle("POST /api/channel", middleware.AdminAuth(http.HandlerFunc(controller.CreateChannel)))
	mux.Handle("POST /api/channel/", middleware.AdminAuth(http.HandlerFunc(controller.CreateChannel)))
	mux.Handle("GET /api/channel", middleware.AdminAuth(http.HandlerFunc(controller.ListChannels)))
	mux.Handle("GET /api/channel/", middleware.AdminAuth(http.HandlerFunc(controller.ListChannels)))
	mux.Handle("GET /api/channel/{id}", middleware.AdminAuth(http.HandlerFunc(controller.GetChannel)))
	mux.Handle("PUT /api/channel", middleware.AdminAuth(http.HandlerFunc(controller.UpdateChannel)))
	mux.Handle("PUT /api/channel/", middleware.AdminAuth(http.HandlerFunc(controller.UpdateChannel)))
	mux.Handle("DELETE /api/channel/{id}", middleware.AdminAuth(http.HandlerFunc(controller.DeleteChannel)))

	mux.Handle("POST /api/token", middleware.UserAuth(http.HandlerFunc(controller.CreateToken)))
	mux.Handle("POST /api/token/", middleware.UserAuth(http.HandlerFunc(controller.CreateToken)))
	mux.Handle("GET /api/token", middleware.UserAuth(http.HandlerFunc(controller.ListTokens)))
	mux.Handle("GET /api/token/", middleware.UserAuth(http.HandlerFunc(controller.ListTokens)))
	mux.Handle("PUT /api/token", middleware.UserAuth(http.HandlerFunc(controller.UpdateToken)))
	mux.Handle("PUT /api/token/", middleware.UserAuth(http.HandlerFunc(controller.UpdateToken)))
	mux.Handle("DELETE /api/token/{id}", middleware.UserAuth(http.HandlerFunc(controller.DeleteToken)))

	mux.Handle("GET /api/option", middleware.AdminAuth(http.HandlerFunc(controller.ListOptions)))
	mux.Handle("GET /api/option/", middleware.AdminAuth(http.HandlerFunc(controller.ListOptions)))
	mux.Handle("PUT /api/option", middleware.AdminAuth(http.HandlerFunc(controller.UpdateOption)))
	mux.Handle("PUT /api/option/", middleware.AdminAuth(http.HandlerFunc(controller.UpdateOption)))

	mux.Handle("GET /api/log", middleware.UserAuth(http.HandlerFunc(controller.ListLogs)))
	mux.Handle("GET /api/log/", middleware.UserAuth(http.HandlerFunc(controller.ListLogs)))

	mux.Handle("POST /v1/chat/completions",
		middleware.TokenAuth(middleware.Distribute(http.HandlerFunc(controller.Relay))))
	mux.Handle("GET /v1/models", middleware.TokenAuth(http.HandlerFunc(controller.ListModels)))

	return mux
}
