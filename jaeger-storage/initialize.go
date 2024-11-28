package main

import (
	"context"
	"fmt"
	"github.com/jmoiron/sqlx"
	"github.com/neo4j/neo4j-go-driver/v5/neo4j"
	"log"
	"os"
)

type NewDbOpt struct {
	Username string
	Password string
	DbName   string
}

func NewDb(opt NewDbOpt) (*sqlx.DB, error) {
	var dbHost = "localhost"
	fmt.Println("os.Getenv(POSTGRES_HOST)", os.Getenv("POSTGRES_HOST"))
	if os.Getenv("POSTGRES_HOST") != "" {
		dbHost = os.Getenv("POSTGRES_HOST")
	}
	// todo: move everything to env variables
	db, err := sqlx.Connect("postgres", fmt.Sprintf("host=%s user=%s password=%s dbname=%s sslmode=disable", dbHost, opt.Username, opt.Password, opt.DbName))
	if err != nil {
		fmt.Println("error while connecting to DB", err)
		return nil, err
	}
	log.Println("[NewDb] connected to postgresql")
	return db, err
}

func NewNeo4jDriver() (*neo4j.DriverWithContext, error) {
	ctx := context.Background()
	// todo: move everything to env variables
	fmt.Println("os.Getenv(NEO4J_URI)", os.Getenv("NEO4J_URI"))
	var dbUri = "bolt://localhost:7687"
	if os.Getenv("NEO4J_URI") != "" {
		dbUri = os.Getenv("NEO4J_URI")
	}

	dbUser := ""
	dbPassword := ""
	driver, err := neo4j.NewDriverWithContext(
		dbUri,
		neo4j.BasicAuth(dbUser, dbPassword, ""))
	if err != nil {
		log.Println(err)
		return nil, err
	}

	err = driver.VerifyConnectivity(ctx)
	if err != nil {
		log.Println(err)
		return nil, err
	}
	log.Println("[NewNeo4jDriver] connected to neo4j")
	return &driver, nil
}

func MakeSureThingsAreOk() {
	if os.Getenv("OPENAI_API_KEY") == "" {
		panic("please provide a value to OPENAI_API_KEY env")
	}
}
