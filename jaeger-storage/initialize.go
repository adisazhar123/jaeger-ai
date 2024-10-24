package main

import (
	"fmt"
	"github.com/jmoiron/sqlx"
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
