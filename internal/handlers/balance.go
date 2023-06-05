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
		log.Println("UsersBalance: User not authenticated: ", err)
		return
	}

	totalWriteOff, err := repository.TotalWriteOff(c.DBconn, tk.Value)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		fmt.Fprint(w, err)
		log.Println("UsersBalance: repository.TotalWriteOff(c.DBconn, tk.Value): ", err)
		return
	}

	status, _, balanceScore, newErr := repository.LoadedOrderNumbers(c.DBconn, c.AccrualSA, tk.Value)
	if newErr != nil {
		w.WriteHeader(status)
		fmt.Fprint(w, newErr)
		log.Println("UsersBalance: newErr: ", newErr)
		return
	}

	s := AllBalance{Current: balanceScore - totalWriteOff, Withdrawn: totalWriteOff}

	balance, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		log.Println("UsersBalance: MarshalIndent: ", err)
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
		log.Println("UsersBalanceWithdraw: User not authenticated: ", err)
		return
	}

	var req WithdrawalRequest
	err = json.NewDecoder(r.Body).Decode(&req)
	if err != nil {
		http.Error(w, "Invalid withdrawal amount. UsersBalanceWithdraw Decoder", http.StatusInternalServerError)
		log.Println("UsersBalanceWithdraw: NewDecoder: ", err)
		return
	}
	if req.Order == "" {
		http.Error(w, "Invalid order format", http.StatusBadRequest)
		log.Println("UsersBalanceWithdraw: Order = nil")
		return
	}

	num, err := strconv.Atoi(req.Order)
	if err != nil {
		http.Error(w, "Internal server error. Atoi", http.StatusInternalServerError)
		log.Println("UsersBalanceWithdraw: Atoi: ", err)
		return
	}
	checkNumber := repository.CalculateLuhn(num / 10)
	if checkNumber != num%10 {
		http.Error(w, "Invalid order number", http.StatusUnprocessableEntity)
		log.Println("UsersBalanceWithdraw: Invalid order number")
		return
	}

	totalWriteOff, err := repository.TotalWriteOff(c.DBconn, tk.Value)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		fmt.Fprint(w, err)
		log.Println("UsersBalanceWithdraw: repository.TotalWriteOff(c.DBconn, tk.Value): ", err)
		return
	}

	status, _, balanceScore, newErr := repository.LoadedOrderNumbers(c.DBconn, c.AccrualSA, tk.Value)
	if newErr != nil {
		w.WriteHeader(status)
		fmt.Fprint(w, newErr)
		log.Println("UsersBalanceWithdraw: newErr: ", newErr)
		return
	}

	if balanceScore-totalWriteOff-req.Score <= 0 {
		http.Error(w, "Insufficient funds in the account", http.StatusPaymentRequired)
		log.Println("UsersBalanceWithdraw: Insufficient funds in the account")
		return
	}

	_, err = c.DBconn.Exec(context.Background(),
		`insert into balance (user_token, id_order, processed_at, 
                    total_balance_score, order_balance_score, total_write_off) 
				values ($1, $2, now(), $3, $4, $5)`,
		tk.Value, req.Order, balanceScore-totalWriteOff, req.Score, totalWriteOff+req.Score)
	if err != nil {
		http.Error(w, "Internal server error. Insert into balance", http.StatusInternalServerError)
		log.Println("UsersBalanceWithdraw: Insert into balance: ", err)
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
		log.Println("UsersWithdrawals: User not authenticated: ", err)
		return
	}

	rows, err := c.DBconn.Query(context.Background(),
		`select id_order, order_balance_score, processed_at
			from balance 
			where user_token = $1
			order by 3 desc`,
		tk.Value)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		http.Error(w, `Internal server error. Select id_order, event_time`, http.StatusInternalServerError)
		log.Println("UsersWithdrawals: User not authenticated: Select id_order, event_time: ", err)
		return
	}
	if errors.Is(err, pgx.ErrNoRows) {
		http.Error(w, "No data to answer", http.StatusNoContent)
		log.Println("UsersWithdrawals: No data to answer")
		return
	}
	defer rows.Close()

	var withdrawals []Withdrawals
	for rows.Next() {
		var writeoff Withdrawals
		err := rows.Scan(&writeoff.Order, &writeoff.Withdrawn, &writeoff.EventTime)
		if err != nil {
			log.Println("UsersWithdrawals: Scan AccrualOrders: ", err)
			http.Error(w, "Internal server error. Scan AccrualOrders", http.StatusInternalServerError)
			return
		}

		writeoff.ProcessedAt = writeoff.EventTime.Format(time.RFC3339)
		withdrawals = append(withdrawals, writeoff)
	}

	withdrawalsMarshal, err := json.MarshalIndent(withdrawals, "", "  ")
	if err != nil {
		log.Println("UsersWithdrawals: MarshalIndent: ", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
	fmt.Fprint(w, string(withdrawalsMarshal))
}
