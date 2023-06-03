package server

import (
	"context"
	"errors"
	"flag"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"time"

	"github.com/caarlos0/env/v6"
	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/LidaSv/Gofermart.git/internal/handlers"
)

type Configs struct {
	RunAddress           string `env:"RUN_ADDRESS" envDefault:"localhost:8080"`
	AccrualSystemAddress string `env:"ACCRUAL_SYSTEM_ADDRESS"`
	DatabaseURI          string `env:"DATABASE_URI"`
	//envDefault:"host=localhost port=6422 user=postgres password=123 dbname=postgres"
}

func ConnectDB(dbURL string) (*pgxpool.Pool, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	conn, err := pgxpool.New(ctx, dbURL)
	if err != nil {
		return nil, err
	}
	return conn, nil
}

func AddServer() {

	var cfg Configs
	err := env.Parse(&cfg)
	if err != nil {
		log.Fatal(err)
	}

	FlagServerAddress := flag.String("a", cfg.RunAddress, "a string")
	AccrualSystemAddress := flag.String("r", cfg.RunAddress, "a string")
	FlagDatabaseDsn := flag.String("d", cfg.DatabaseURI, "a string")
	flag.Parse()

	db, err := ConnectDB(*FlagDatabaseDsn)
	if err != nil {
		log.Println("Failed to connect to the database:", err)
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
			order_balance_score float64,
			total_write_off 	float64
			)`)
	if err != nil {
		log.Println("Failed to create balance table:", err)
		return
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

	replacer := strings.NewReplacer("https://", "", "http://", "")
	ServerAdd := replacer.Replace(*FlagServerAddress)

	if ServerAdd[len(ServerAdd)-1:] == "/" {
		ServerAdd = ServerAdd[:len(ServerAdd)-1]
	}

	server := &http.Server{
		Addr:              ServerAdd,
		Handler:           r,
		ReadHeaderTimeout: time.Second,
		ReadTimeout:       time.Duration(5) * time.Second,
		WriteTimeout:      time.Duration(5) * time.Second,
		IdleTimeout:       time.Duration(5) * time.Second,
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
			log.Fatal("<-stop", err)
		}
		s.DBconn.Close()
	case <-chErrors:
		err := server.Shutdown(context.Background())
		if err != nil {
			log.Fatal("<-chErrors", err)
		}
		s.DBconn.Close()
	}
}
