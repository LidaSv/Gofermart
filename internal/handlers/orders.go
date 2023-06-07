package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/jackc/pgx/v5"
	"log"
	"net/http"
	"strconv"

	"github.com/LidaSv/Gofermart.git/internal/repository"
)

func (c *Config) UsersOrdersDown(w http.ResponseWriter, r *http.Request) {
	w.Header().Set(typeContentType, "text/plain")
	tk, err := r.Cookie("token")
	if err != nil {
		http.Error(w, "User not authenticated", http.StatusUnauthorized)
		log.Println("UsersOrdersDown: User not authenticated: ", err)
		return
	}

	var idOrder int
	err = json.NewDecoder(r.Body).Decode(&idOrder)
	if err != nil {
		http.Error(w, "Invalid request format", http.StatusBadRequest)
		log.Println("UsersOrdersDown: Invalid request format: ", err)
		return
	}
	if idOrder == 0 {
		http.Error(w, "Invalid order format", http.StatusBadRequest)
		log.Println("UsersOrdersDown: Invalid order format")
		return
	}

	checkNumber := repository.CalculateLuhn(idOrder / 10)
	if checkNumber != idOrder%10 {
		http.Error(w, "Invalid order number", http.StatusUnprocessableEntity)
		log.Println("UsersOrdersDown: Invalid order number")
		return
	}

	var typeOrder string
	err = c.DBconn.QueryRow(context.Background(),
		`select case when user_token = $1 then 'user order' else 'other order' end type_order 
			from orders 
			where id_order = $2`,
		tk.Value, strconv.Itoa(idOrder)).Scan(&typeOrder)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		http.Error(w, "Internal server error. Select case", http.StatusInternalServerError)
		log.Println("UsersOrdersDown: Select case: ", err)
		return
	}

	switch typeOrder {
	case "other order":
		w.WriteHeader(http.StatusConflict)
		w.Write([]byte("Order has already been uploaded by another user"))
		return
	case "user order":
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("Order has already been uploaded by this user"))
		return
	default:
		_, err = c.DBconn.Exec(context.Background(),
			`insert into orders (user_token, id_order, event_time) 
				values ($1, $2, now())`,
			tk.Value, strconv.Itoa(idOrder))
		if err != nil {
			http.Error(w, "Internal server error. Insert into orders", http.StatusInternalServerError)
			log.Println("UsersOrdersDown: Insert into orders: ", err)
			return
		}
		w.WriteHeader(http.StatusAccepted)
		w.Write([]byte("New order accepted for processing"))
	}
}

func (c *Config) UsersOrdersGet(w http.ResponseWriter, r *http.Request) {
	w.Header().Set(typeContentType, "application/json")
	tk, err := r.Cookie("token")
	if err != nil {
		http.Error(w, "User not authenticated", http.StatusUnauthorized)
		log.Println("UsersOrdersGet: User not authenticated: ", err)
		return
	}

	ordersFull, newErr := repository.LoadedOrderNumbers(c.DBconn, c.AccrualSA, tk.Value)
	if newErr != nil && !errors.Is(newErr, repository.ErrNoData) {
		w.WriteHeader(http.StatusInternalServerError)
		fmt.Fprint(w, newErr)
		log.Println("UsersOrdersGet: internal server error: newErr: ", newErr)
		return
	}

	orders := ordersFull.Accrual

	if errors.Is(newErr, repository.ErrNoData) {
		orders, err = repository.NoData(c.DBconn, tk.Value, orders)
		if err != nil {
			switch err {
			case repository.ErrInternalServer:
				w.WriteHeader(http.StatusInternalServerError)
				fmt.Fprint(w, err)
				log.Println("UsersOrdersGet: internal server error: errDB: ", err)
				return
			case errors.New(`no data to answer`):
				w.WriteHeader(http.StatusNoContent)
				fmt.Fprint(w, err)
				log.Println("UsersOrdersGet: no data to answer: errDB: ", err)
				return
			}
		}
	}

	ordersMarshal, err := json.MarshalIndent(orders, "", "  ")
	if err != nil {
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		log.Println("UsersOrdersGet: json.MarshalIndent: ", err)
		return
	}

	w.WriteHeader(http.StatusOK)
	fmt.Fprint(w, string(ordersMarshal))
}
