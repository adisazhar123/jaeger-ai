package main

import (
	"context"
	"fmt"
	"github.com/jmoiron/sqlx"
	"github.com/neo4j/neo4j-go-driver/v5/neo4j"
	"log"
)

type NewDbOpt struct {
	Username string
	Password string
	DbName   string
}

func NewDb(opt NewDbOpt) (*sqlx.DB, error) {
	db, err := sqlx.Connect("postgres", fmt.Sprintf("user=%s password=%s dbname=%s sslmode=disable", opt.Username, opt.Password, opt.DbName))
	if err != nil {
		fmt.Println("error while connecting to DB", err)
		return nil, err
	}

	return db, err
}

func NewNeo4jDriver() (*neo4j.DriverWithContext, error) {
	ctx := context.Background()
	dbUri := "bolt://localhost:7687"
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

	return &driver, nil
}
