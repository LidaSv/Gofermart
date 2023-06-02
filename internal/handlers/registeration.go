package handlers

import "C"
import (
	"context"
	"crypto/md5"
	"encoding/hex"
	"encoding/json"
	"github.com/jackc/pgx/v5/pgxpool"
	"net/http"
	"net/url"
	"time"
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
			w.WriteHeader(http.StatusBadRequest)
			w.Write([]byte("Invalid request format. Need to enter login"))
			return
		case user.Password == "":
			w.WriteHeader(http.StatusBadRequest)
			w.Write([]byte("Invalid request format. Need to enter password"))
			return
		}
	}

	var count int
	err = c.DBconn.QueryRow(context.Background(),
		`select count(*) from users where login = $1`,
		user.Login).Scan(&count)
	if err != nil {
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}
	if count > 0 {
		http.Error(w, "Login already taken", http.StatusConflict)
		return
	}

	hash := md5.Sum([]byte(user.Password))
	hashedPass := hex.EncodeToString(hash[:])

	_, err = c.DBconn.Exec(context.Background(),
		`insert into users (login, password) values ($1, $2)`,
		user.Login, hashedPass)
	if err != nil {
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	time64 := time.Now().Unix()
	timeInt := string(time64)
	token := user.Login + user.Password + timeInt
	hashToken := md5.Sum([]byte(token))
	hashedToken := hex.EncodeToString(hashToken[:])
	//a.cache[hashedToken] = user
	livingTime := 60 * time.Minute
	expiration := time.Now().Add(livingTime)
	cookie := http.Cookie{Name: "token", Value: url.QueryEscape(hashedToken), Expires: expiration}
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
			w.WriteHeader(http.StatusBadRequest)
			w.Write([]byte("Invalid request format. Need to enter login"))
			return
		case user.Password == "":
			w.WriteHeader(http.StatusBadRequest)
			w.Write([]byte("Invalid request format. Need to enter password"))
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
	if err != nil {
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}
	if count == 0 {
		http.Error(w, "Invalid login or password", http.StatusUnauthorized)
		return
	}

	time64 := time.Now().Unix()
	timeInt := string(time64)
	token := user.Login + user.Password + timeInt
	hashToken := md5.Sum([]byte(token))
	hashedToken := hex.EncodeToString(hashToken[:])
	//a.cache[hashedToken] = user
	livingTime := 60 * time.Minute
	expiration := time.Now().Add(livingTime)
	cookie := http.Cookie{Name: "token", Value: url.QueryEscape(hashedToken), Expires: expiration}
	http.SetCookie(w, &cookie)

	w.WriteHeader(http.StatusOK)
	w.Write([]byte("User successfully authenticated"))

}

//func (a app) authorized(next http.Handler) http.Handler {
//	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
//		token, err := readCookie("token", r)
//		if err != nil {
//			http.Redirect(w, r, "/login", http.StatusSeeOther)
//			return
//		}
//		if _, ok := a.cache[token]; !ok {
//			http.Redirect(w, r, "/login", http.StatusSeeOther)
//			return
//		}
//		next.ServeHTTP(w, r)
//	})
//}

//func readCookie(name string, r *http.Request) (value string, err error) {
//	if name == "" {
//		return value, errors.New("you are trying to read empty cookie")
//	}
//	cookie, err := r.Cookie(name)
//	if err != nil {
//		return value, err
//	}
//	str := cookie.Value
//	value, _ = url.QueryUnescape(str)
//	return value, err
//}
