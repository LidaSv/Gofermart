package server

import (
	"context"
	"errors"
	"flag"
	"github.com/jackc/pgx/v5/pgxpool"
	"log"
	"net/http"
	"os"
	"os/signal"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/LidaSv/Gofermart.git/internal/handlers"
)

func ConnectDB(dbURL *string) (*pgxpool.Pool, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	conn, err := pgxpool.New(ctx, *dbURL)
	if err != nil {
		return nil, err
	}
	return conn, nil
}

func AddServer() {

	FlagServerAddress := flag.String("a", "localhost:8080", "a string")
	AccrualSystemAddress := flag.String("f", "FlagServerAddress", "a string")
	FlagDatabaseDsn := flag.String("d", "host=localhost port=6422 user=postgres password=123 dbname=postgres", "a string")
	flag.Parse()

	db, err := ConnectDB(FlagDatabaseDsn)
	if err != nil {
		log.Fatal("Failed to connect to the database:", err)
		return
	}
	defer db.Close()

	var s handlers.Config
	s.DBconn = db
	s.AccrualSA = *AccrualSystemAddress

	ctx := context.Background()
	_, err = db.Exec(ctx,
		`create table if not exists users (
			id      	bigint primary key generated always as identity,
			login   	text not null unique,
			password 	text not null
			)`)
	if err != nil {
		log.Fatal("Failed to create users table:", err)
	}

	_, err = db.Exec(ctx,
		`create table if not exists orders (
			user_token 	text not null,
			id_order 	text not null unique,
			event_time 	timestamptz not null
			)`)
	if err != nil {
		log.Fatal("Failed to create orders table:", err)
	}

	_, err = db.Exec(ctx,
		`create table if not exists balance (
			user_token 			text not null,
			id_order 			text not null unique,
			processed_at 		timestamptz not null,
    		total_balance_score	numeric(14,2),
			order_balance_score float64,
			total_write_off 	float64
			)`)
	if err != nil {
		log.Fatal("Failed to create balance table:", err)
	}

	r := chi.NewRouter()

	r.Route("/api/user", func(r chi.Router) {
		r.Post("/register", s.UsersRegister)
		r.Post("/login", s.UsersLogin)
		r.Post("/orders", s.UsersOrdersDown)
		r.Get("/orders", s.UsersOrdersGet)
		r.Get("/balance", s.UsersBalance)
		r.Post("/balance/withdraw", s.UsersBalanceWithdraw)
		r.Get("/withdrawals", s.UsersWithdrawals)
	})

	server := &http.Server{
		Addr:              *FlagServerAddress,
		Handler:           r,
		ReadHeaderTimeout: 5 * time.Second,
	}

	chErrors := make(chan error)

	go func() {
		err := server.ListenAndServe()
		if !errors.Is(err, http.ErrServerClosed) {
			chErrors <- err
		}
	}()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt)

	select {
	case <-stop:
		signal.Stop(stop)
		err := server.Shutdown(context.Background())
		if err != nil {
			log.Fatal(err)
		}
	case <-chErrors:
		err := server.Shutdown(context.Background())
		if err != nil {
			log.Fatal(err)
		}
	}
}
