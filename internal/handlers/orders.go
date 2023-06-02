package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/LidaSv/Gofermart.git/internal/repository"
	"github.com/jackc/pgx/v5"
	"log"
	"net/http"
	"strconv"
)

func (c *Config) UsersOrdersDown(w http.ResponseWriter, r *http.Request) {
	w.Header().Set(typeContentType, "text/plain")
	tk, err := r.Cookie("token")
	if err != nil {
		http.Error(w, "User not authenticated", http.StatusUnauthorized)
		return
	}

	var idOrder int
	err = json.NewDecoder(r.Body).Decode(&idOrder)
	if err != nil {
		http.Error(w, "Invalid request format", http.StatusBadRequest)
		return
	}
	if idOrder == 0 {
		http.Error(w, "Invalid order format", http.StatusBadRequest)
		return
	}

	checkNumber := repository.CalculateLuhn(idOrder / 10)
	if checkNumber != idOrder%10 {
		http.Error(w, "Invalid order number", http.StatusUnprocessableEntity)
		return
	}

	var typeOrder string
	err = c.DBconn.QueryRow(context.Background(),
		`select case when user_token = $1 then 'user order' else 'other order' end type_order 
			from orders 
			where id_order = $2`,
		tk.Value, strconv.Itoa(idOrder)).Scan(&typeOrder)
	if err != nil && err != pgx.ErrNoRows {
		http.Error(w, "Internal server error. Select case", http.StatusInternalServerError)
		log.Print(err)
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
			log.Print(err)
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
		return
	}

	newErr, status, orders, _ := repository.LoadedOrderNumbers(c.DBconn, c.AccrualSA, tk.Value)
	if newErr != nil {
		w.WriteHeader(status)
		fmt.Fprint(w, newErr)
		log.Println(newErr)
		return
	}

	ordersMarshal, err := json.MarshalIndent(orders, "", "  ")
	if err != nil {
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	w.WriteHeader(status)
	fmt.Fprint(w, string(ordersMarshal))
}
