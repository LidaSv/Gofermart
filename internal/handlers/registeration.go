package handlers

import "C"
import (
	"context"
	"crypto/md5"
	"encoding/hex"
	"encoding/json"
	"errors"
	"log"
	"net/http"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/LidaSv/Gofermart.git/internal/cookie"
)

const (
	typeContentType     = "Content-Type"
	bodyContentTypeJSON = "application/json"
)

type Config struct {
	DBconn    *pgxpool.Pool
	AccrualSA string
}

type User struct {
	Login    string `json:"login"`
	Password string `json:"password"`
}

func (c *Config) UsersRegister(w http.ResponseWriter, r *http.Request) {
	w.Header().Set(typeContentType, bodyContentTypeJSON)

	var user User
	err := json.NewDecoder(r.Body).Decode(&user)
	if err != nil {
		switch {
		case user.Login == "":
			http.Error(w, "Invalid request format. Need to enter login", http.StatusBadRequest)
			log.Println("UsersRegister: Invalid request format. Need to enter login")
			return
		case user.Password == "":
			http.Error(w, "Invalid request format. Need to enter password", http.StatusInternalServerError)
			log.Println("UsersRegister: Invalid request format. Need to enter password")
			return
		}
	}

	var count int
	err = c.DBconn.QueryRow(context.Background(),
		`select count(*) from users where login = $1`,
		user.Login).Scan(&count)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		log.Println("UsersRegister: select count(*) from users: ", err)
		return
	}
	if count > 0 {
		http.Error(w, "Login already taken", http.StatusConflict)
		log.Println("UsersRegister: Login already taken")
		return
	}

	hash := md5.Sum([]byte(user.Password))
	hashedPass := hex.EncodeToString(hash[:])

	_, err = c.DBconn.Exec(context.Background(),
		`insert into users (login, password) values ($1, $2)`,
		user.Login, hashedPass)
	if err != nil {
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		log.Println("UsersRegister: insert into users (login, password): ", err)
		return
	}

	cookie := cookie.SetCookie(user.Login)
	http.SetCookie(w, &cookie)

	w.WriteHeader(http.StatusOK)
	w.Write([]byte("User successfully registered and authenticated"))
}

func (c *Config) UsersLogin(w http.ResponseWriter, r *http.Request) {
	w.Header().Set(typeContentType, bodyContentTypeJSON)

	var user User
	err := json.NewDecoder(r.Body).Decode(&user)
	if err != nil {
		switch {
		case user.Login == "":
			http.Error(w, "Invalid request format. Need to enter login", http.StatusBadRequest)
			log.Println("UsersRegister: Invalid request format. Need to enter login")
			return
		case user.Password == "":
			http.Error(w, "Invalid request format. Need to enter password", http.StatusInternalServerError)
			log.Println("UsersRegister: Invalid request format. Need to enter password")
			return
		}
	}

	hash := md5.Sum([]byte(user.Password))
	hashedPass := hex.EncodeToString(hash[:])

	var count int
	err = c.DBconn.QueryRow(context.Background(),
		`select count(*) 
			from users 
			where login = $1 and password = $2`,
		user.Login, hashedPass).Scan(&count)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		log.Println("UsersLogin: select count(*) from users: ", err)
		return
	}
	if errors.Is(err, pgx.ErrNoRows) {
		http.Error(w, "Invalid login or password", http.StatusUnauthorized)
		log.Println("UsersLogin: cnt = 0")
		return
	}

	cookie := cookie.SetCookie(user.Login)
	http.SetCookie(w, &cookie)

	w.WriteHeader(http.StatusOK)
	w.Write([]byte("User successfully authenticated"))
}
