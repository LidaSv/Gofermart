package repository

import (
	"context"
	"github.com/jackc/pgx/v5"
	"log"
)

func InitDBConn(ctx context.Context) (db *pgx.Conn, err error) {
	url := "host=localhost port=6422 user=postgres password=123 dbname=postgres"

	db, err = pgx.Connect(ctx, url)
	if err != nil {
		log.Fatal("Unable to connect to database:", err)
		return nil, err
	}

	_, err = db.Exec(ctx,
		`create table if not exists users(
			id      bigint primary key generated always as identity,
			login   varchar(200) not null unique,
			hashed_password varchar(200) not null
		);`)
	if err != nil {
		log.Fatal("create: ", err)
	}
	//defer db.Close(ctx)

	return db, nil
}
