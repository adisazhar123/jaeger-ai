package main

import (
	"context"
	"log"
	"time"
)

func main() {
	db, err := NewDb(NewDbOpt{
		Username: "postgres",
		Password: "password",
		DbName:   "jaeger-storage",
	})
	if err != nil {
		log.Println("error connecting to DB")
		return
	}

	w := NewWriterDBClient(db)

	id, err := w.upsertService(context.TODO(), service{
		Name:      "user_service",
		CreatedAt: time.Now(),
	})

	if err != nil {
		log.Println("error upsert service", err)
		return
	}

	log.Println("id", id)
}
