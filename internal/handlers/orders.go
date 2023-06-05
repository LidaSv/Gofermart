package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/LidaSv/Gofermart.git/internal/repository"
	"github.com/jackc/pgx/v5"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"
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

	status, orders, _, newErr := repository.LoadedOrderNumbers(c.DBconn, c.AccrualSA, tk.Value)
	if newErr != nil && status != http.StatusNoContent {
		w.WriteHeader(status)
		fmt.Fprint(w, newErr)
		log.Println("UsersOrdersGet: newErr: ", newErr)
		return
	}

	if status == http.StatusNoContent {
		rows, err := c.DBconn.Query(context.Background(),
			`select id_order, event_time
			from orders 
			where user_token = $1`,
			tk.Value)

		if err != nil && !errors.Is(err, pgx.ErrNoRows) {
			log.Println("UsersOrdersGet: select id_order, event_time: ", err)
			http.Error(w, "Internal server error. Select idOrder", http.StatusInternalServerError)
			return
		}

		if errors.Is(err, pgx.ErrNoRows) {
			log.Println("UsersOrdersGet: errors.Is(err, pgx.ErrNoRows): ", err)
			http.Error(w, "Internal server error. Select idOrder", http.StatusNoContent)
			return
		}
		defer rows.Close()

		for rows.Next() {
			var idOrder string
			var uploadedAt time.Time
			var order repository.AccrualOrders
			err := rows.Scan(&idOrder, &uploadedAt)

			if err != nil {
				log.Println("LoadedOrderNumbers: scan rows: ", err)
				http.Error(w, "Internal server error. Scan idOrder", http.StatusInternalServerError)
				return
			}

			var totalBalanceScore float64
			err = c.DBconn.QueryRow(context.Background(),
				`select total_balance_score
					from balance 
					where id_order = $1 and user_token=$2`,
				idOrder, tk.Value).Scan(&totalBalanceScore)
			if err != nil && !errors.Is(err, pgx.ErrNoRows) {
				log.Println("UsersOrdersGet: select order_balance_score: ", err)
				http.Error(w, "Internal server error. select order_balance_score", http.StatusInternalServerError)
				return
			}

			if errors.Is(err, pgx.ErrNoRows) {
				log.Println("no data")
			}

			var AccrualURL string
			if strings.HasSuffix(c.AccrualSA, "/") {
				AccrualURL = c.AccrualSA + "api/orders/"
			} else {
				AccrualURL = c.AccrualSA + "/api/orders/"
			}
			_, _, balanceScore1, err := repository.GetHTTP(AccrualURL+idOrder, order, 0)

			order.Accrual = balanceScore1
			log.Println("BALANCE: ", balanceScore1)
			order.NumberOrder = idOrder
			order.UploadedAt = uploadedAt.Format(time.RFC3339)
			order.Status = "NEW"
			orders = append(orders, order)
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
