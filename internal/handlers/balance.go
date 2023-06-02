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
	"time"
)

type WithdrawalRequest struct {
	Order string  `json:"order"`
	Score float64 `json:"sum"`
}

type AllBalance struct {
	Current   float64 `json:"current"`
	Withdrawn float64 `json:"withdrawn"`
}

type Withdrawals struct {
	Order       string    `json:"order"`
	Withdrawn   float64   `json:"sum"`
	EventTime   time.Time `json:"-"`
	ProcessedAt string    `json:"processed_at"`
}

func (c *Config) UsersBalance(w http.ResponseWriter, r *http.Request) {
	w.Header().Set(typeContentType, "application/json")
	tk, err := r.Cookie("token")
	if err != nil {
		http.Error(w, "User not authenticated", http.StatusUnauthorized)
		return
	}

	err, totalWriteOff := repository.TotalWriteOff(c.DBconn, tk.Value)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		fmt.Fprint(w, err)
		return
	}

	newErr, status, _, balanceScore := repository.LoadedOrderNumbers(c.DBconn, c.AccrualSA, tk.Value)
	if newErr != nil {
		w.WriteHeader(status)
		fmt.Fprint(w, newErr)
		log.Println(newErr)
		return
	}

	s := AllBalance{Current: balanceScore, Withdrawn: totalWriteOff}

	balance, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
	fmt.Fprint(w, string(balance))
}

func (c *Config) UsersBalanceWithdraw(w http.ResponseWriter, r *http.Request) {
	w.Header().Set(typeContentType, "application/json")
	tk, err := r.Cookie("token")
	if err != nil {
		http.Error(w, "User not authenticated", http.StatusUnauthorized)
		return
	}

	var req WithdrawalRequest
	err = json.NewDecoder(r.Body).Decode(&req)
	if err != nil {
		http.Error(w, "Invalid withdrawal amount. UsersBalanceWithdraw Decoder", http.StatusInternalServerError)
		return
	}
	if req.Order == "" {
		http.Error(w, "Invalid order format", http.StatusBadRequest)
		return
	}

	num, err := strconv.Atoi(req.Order)
	if err != nil {
		http.Error(w, "Internal server error. Select case", http.StatusInternalServerError)
		return
	}
	checkNumber := repository.CalculateLuhn(num / 10)
	if checkNumber != num%10 {
		http.Error(w, "Invalid order number", http.StatusUnprocessableEntity)
		return
	}

	err, totalWriteOff := repository.TotalWriteOff(c.DBconn, tk.Value)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		fmt.Fprint(w, err)
	}

	newErr, status, _, balanceScore := repository.LoadedOrderNumbers(c.DBconn, c.AccrualSA, tk.Value)
	if err != nil {
		w.WriteHeader(status)
		fmt.Fprint(w, newErr)
	}

	if balanceScore-totalWriteOff-req.Score <= 0 {
		http.Error(w, "Insufficient funds in the account", http.StatusPaymentRequired)
		return
	}

	_, err = c.DBconn.Exec(context.Background(),
		`insert into balance (user_token, id_order, processed_at, 
                    total_balance_score, order_balance_score, total_write_off) 
				values ($1, $2, now(), $4, $5, $6)`,
		tk.Value, req.Order, balanceScore-totalWriteOff, req.Score, totalWriteOff+req.Score)
	if err != nil {
		http.Error(w, "Internal server error. Insert into balance", http.StatusInternalServerError)
		log.Print(err)
		return
	}

	w.WriteHeader(http.StatusOK)
	w.Write([]byte("Successful request processing"))
}

func (c *Config) UsersWithdrawals(w http.ResponseWriter, r *http.Request) {
	w.Header().Set(typeContentType, "application/json")
	tk, err := r.Cookie("token")
	if err != nil {
		http.Error(w, "User not authenticated", http.StatusUnauthorized)
		return
	}

	rows, err := c.DBconn.Query(context.Background(),
		`select id_order, order_balance_score, processed_at
			from balance 
			where user_token = $1
			order by 3 desc`,
		tk.Value)
	if err != nil && err != pgx.ErrNoRows {
		http.Error(w, `Internal server error. Select id_order, event_time`, http.StatusInternalServerError)
		log.Print(err)
		return
	}
	if err == pgx.ErrNoRows {
		http.Error(w, "No data to answer", http.StatusNoContent)
		return
	}
	defer rows.Close()

	var withdrawals []Withdrawals
	for rows.Next() {
		var writeoff Withdrawals
		err := rows.Scan(&writeoff.Order, &writeoff.Withdrawn, &writeoff.EventTime)
		if err != nil {
			log.Println(err)
			http.Error(w, "Internal server error. Scan AccrualOrders", http.StatusInternalServerError)
			return
		}

		writeoff.ProcessedAt = writeoff.EventTime.Format(time.RFC3339)
		withdrawals = append(withdrawals, writeoff)
	}

	withdrawalsMarshal, err := json.MarshalIndent(withdrawals, "", "  ")
	if err != nil {
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
	fmt.Fprint(w, string(withdrawalsMarshal))
}
