package server

import (
	"context"
	"errors"
	"flag"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/LidaSv/Gofermart.git/internal/handlers"
	"github.com/caarlos0/env/v6"
	"github.com/go-chi/chi/v5"
)

type Configs struct {
	RunAddress           string `env:"RUN_ADDRESS" envDefault:"localhost:8080"`
	AccrualSystemAddress string `env:"ACCRUAL_SYSTEM_ADDRESS" envDefault:"http://localhost:8080"`
	DatabaseURI          string `env:"DATABASE_URI" envDefault:"host=localhost port=6422 user=postgres password=123 dbname=postgres"`
	//envDefault:"host=localhost port=6422 user=postgres password=123 dbname=postgres"
}

func AddServer() {

	var cfg Configs
	err := env.Parse(&cfg)
	if err != nil {
		log.Fatal(err)
	}

	FlagRunAddress := flag.String("a", cfg.RunAddress, "a string")
	FlagAccrualSystemAddress := flag.String("r", cfg.AccrualSystemAddress, "a string")
	FlagDatabaseURI := flag.String("d", cfg.DatabaseURI, "a string")
	flag.Parse()

	db, err := CreateTables(*FlagDatabaseURI)
	if err != nil {
		log.Fatal(err)
	}

	var s handlers.Config
	s.AccrualSA = *FlagAccrualSystemAddress
	s.DBconn = db

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

	serverPath, serverExists := os.LookupEnv("RUN_ADDRESS")

	var ServerAdd string
	if serverExists {
		ServerAdd = serverPath
	} else {
		ServerAdd = *FlagRunAddress
	}

	if ServerAdd[len(ServerAdd)-1:] == "/" {
		ServerAdd = ServerAdd[:len(ServerAdd)-1]
	}

	listener, err := net.Listen("tcp", ServerAdd)
	if err != nil {
		log.Printf("Listen on address %s: %v", ServerAdd, err)
		return
	}

	server := http.Server{
		Handler:           r,
		ReadHeaderTimeout: time.Second,
		ReadTimeout:       time.Duration(5) * time.Second,
		WriteTimeout:      time.Duration(5) * time.Second,
		IdleTimeout:       time.Duration(5) * time.Second,
	}

	chErrors := make(chan error)

	go func() {
		err := server.Serve(listener)
		if !errors.Is(err, http.ErrServerClosed) {
			chErrors <- err
		}
	}()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGTERM, syscall.SIGINT)

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
