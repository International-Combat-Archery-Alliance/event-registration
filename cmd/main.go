package main

import (
	"fmt"
	"log"
	"net"
	"net/http"
	"os"

	"github.com/International-Combat-Archery-Alliance/event-registration/api"
	middleware "github.com/oapi-codegen/nethttp-middleware"
)

func main() {
	eventAPI := &api.API{}

	swagger, err := api.GetSwagger()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error loading swagger spec\n: %s", err)
		os.Exit(1)
	}

	swagger.Servers = nil

	strictHandler := api.NewStrictHandler(eventAPI, []api.StrictMiddlewareFunc{})

	r := http.NewServeMux()

	api.HandlerFromMux(strictHandler, r)

	h := middleware.OapiRequestValidator(swagger)(r)

	serverSettings := getServerSettingsFromEnv()
	s := &http.Server{
		Handler: h,
		Addr:    net.JoinHostPort(serverSettings.Host, serverSettings.Port),
	}

	log.Fatal(s.ListenAndServe())
}

type ServerSettings struct {
	Host string
	Port string
}

func getServerSettingsFromEnv() ServerSettings {
	return ServerSettings{
		Host: getEnvOrDefault("HOST", "0.0.0.0"),
		Port: getEnvOrDefault("PORT", "8080"),
	}
}

func getEnvOrDefault(key string, defaultVal string) string {
	if v, ok := os.LookupEnv(key); ok {
		return v
	}

	return defaultVal
}
