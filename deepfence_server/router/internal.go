package router

import (
	"github.com/ThreeDotsLabs/watermill-kafka/v2/pkg/kafka"
	"github.com/deepfence/ThreatMapper/deepfence_server/handler"
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/jwtauth/v5"
	"github.com/go-playground/validator/v10"
	"github.com/twmb/franz-go/pkg/kgo"
)

func InternalRoutes(r *chi.Mux, serverPort string, jwtSecret []byte,
	ingestC chan *kgo.Record, taskPublisher *kafka.Publisher) error {
	// JWT
	tokenAuth := jwtauth.New("HS256", jwtSecret, nil)

	// authorization
	authEnforcer, err := newAuthorizationHandler()
	if err != nil {
		return err
	}

	dfHandler := &handler.Handler{
		TokenAuth:      tokenAuth,
		AuthEnforcer:   authEnforcer,
		SaasDeployment: IsSaasDeployment(),
		Validator:      validator.New(),
		IngestChan:     ingestC,
		TasksPublisher: taskPublisher,
	}

	r.Route("/deepfence/internal", func(r chi.Router) {
		r.Group(func(r chi.Router) {
			r.Route("/console-api-token", func(r chi.Router) {
				r.Get("/", dfHandler.GetApiTokenForConsoleAgent)
			})
		})
	})

	return nil
}
