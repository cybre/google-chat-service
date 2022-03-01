package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"

	"github.com/cybre/google-chat-service/internal/models"
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/streadway/amqp"
)

func main() {
	r := chi.NewRouter()
	r.Use(middleware.Logger)

	rabbitMQHost, ok := os.LookupEnv("RABBITMQ_HOST")
	if !ok {
		log.Fatal("Could not find RABBITMQ_HOST in env")
	}
	conn, err := amqp.Dial(fmt.Sprintf("amqp://guest:guest@%s/", rabbitMQHost))
	if err != nil {
		log.Fatalf("Failed to connect to RabbitMQ: %v", err)
	}
	defer conn.Close()

	ch, err := conn.Channel()
	if err != nil {
		log.Fatalf("Failed to open a channel: %v", err)
	}
	defer ch.Close()

	q, err := ch.QueueDeclare(
		"messages", // name
		true,       // durable
		false,      // delete when unused
		false,      // exclusive
		false,      // no-wait
		nil,        // arguments
	)
	if err != nil {
		log.Fatalf("Failed to create queue: %v", err)
	}

	r.Post("/messages", func(rw http.ResponseWriter, r *http.Request) {
		buf, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(rw, "Failed to read request body", http.StatusInternalServerError)
			return
		}
		reader1 := io.NopCloser(bytes.NewBuffer(buf))
		reader2 := io.NopCloser(bytes.NewBuffer(buf))

		var messages models.Messages
		err = json.NewDecoder(reader1).Decode(&messages)
		if err != nil {
			http.Error(rw, "Failed to decode request body", http.StatusBadRequest)
			return
		}

		payload, err := io.ReadAll(reader2)
		if err != nil {
			http.Error(rw, "Failed to decode request body", http.StatusBadRequest)
			return
		}

		ch.Publish(
			"",
			q.Name,
			false,
			false,
			amqp.Publishing{
				ContentType: "application/json",
				Body:        payload,
			},
		)
	})

	port, ok := os.LookupEnv("PORT")
	if !ok {
		log.Fatal("Could not find PORT in env")
	}
	http.ListenAndServe(fmt.Sprintf(":%s", port), r)
}
