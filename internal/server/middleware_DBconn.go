package server

import (
	"context"
	"github.com/LidaSv/Gofermart.git/internal/handlers"
	"github.com/jackc/pgx/v5/pgxpool"
	"log"
)

func ConnectDB(dbURL string) (*pgxpool.Pool, error) {
	ctx := context.Background()

	conn, err := pgxpool.New(ctx, dbURL)
	if err != nil {
		return nil, err
	}
	return conn, nil
}

func CreateTables(FlagDatabaseURI string) {

	db, err := ConnectDB(FlagDatabaseURI)
	if err != nil {
		log.Println("Failed to connect to the database:", err)
		return
	}
	defer db.Close()

	var s handlers.Config
	s.DBconn = db

	ctx := context.Background()

	_, err = db.Exec(ctx,
		`create table if not exists users (
			id      	bigint primary key generated always as identity,
			login   	text not null unique,
			password 	text not null
			)`)
	if err != nil {
		log.Println("Failed to create users table:", err)
		return
	}

	_, err = db.Exec(ctx,
		`create table if not exists orders (
			user_token 	text not null,
			id_order 	text not null unique,
			event_time 	timestamptz not null
			)`)
	if err != nil {
		log.Println("Failed to create orders table:", err)
		return
	}

	_, err = db.Exec(ctx,
		`create table if not exists balance (
			user_token 			text not null,
			id_order 			text not null unique,
			processed_at 		timestamptz not null,
    		total_balance_score	numeric(14,2),
			order_balance_score numeric(14,2),
			total_write_off 	numeric(14,2)
			)`)
	if err != nil {
		log.Println("Failed to create balance table:", err)
		return
	}
}
