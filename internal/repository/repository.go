package repository

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"io"
	"log"
	"net/http"
	"strings"
	"time"
)

type AccrualOrders struct {
	Order          string    `json:"order,omitempty"`
	NumberOrder    string    `json:"number"`
	Status         string    `json:"status"`
	Accrual        float64   `json:"accrual,omitempty"`
	UploadedAt     string    `json:"uploaded_at"`
	UploadedAtTime time.Time `json:"-"`
}

func TotalWriteOff(conn *pgxpool.Pool, tk string) (error, float64) {
	var totalWriteOff float64
	err := conn.QueryRow(context.Background(),
		`select total_write_off
			from balance 
			where processed_at = (select max(processed_at) 
			                      from balance 
			                      where user_token = $1)`,
		tk).Scan(&totalWriteOff)
	if err != nil && err != pgx.ErrNoRows {
		log.Print(err)
		return errors.New("Internal server error. Select total_write_off"), 0
	}
	return nil, totalWriteOff
}

func LoadedOrderNumbers(conn *pgxpool.Pool, accrualSA, tk string) (error, int, []AccrualOrders, float64) {
	rows, err := conn.Query(context.Background(),
		`select id_order, event_time
			from orders
			where user_token = $1
			order by event_time desc`,
		tk)
	if err != nil && err != pgx.ErrNoRows {
		return errors.New(`Internal server error. Select id_order, event_time`),
			http.StatusInternalServerError, nil, 0
	}
	if err == pgx.ErrNoRows {
		return errors.New("No data to answer"), http.StatusNoContent, nil, 0
	}
	defer rows.Close()

	var AccrualURL string
	if strings.HasSuffix(accrualSA, "/") {
		AccrualURL = accrualSA
	} else {
		AccrualURL = accrualSA + "/"
	}

	var orders []AccrualOrders
	var balanceScore float64
	for rows.Next() {
		var accrual, accrualDecode AccrualOrders
		err := rows.Scan(&accrual.NumberOrder, &accrual.UploadedAtTime)
		if err != nil {
			log.Println(err)
			return errors.New("Internal server error. Scan AccrualOrders"),
				http.StatusInternalServerError, nil, 0
		}

		res, err := http.Get(AccrualURL + accrual.NumberOrder)
		if err != nil {
			log.Print("Internal server error. Get /api/orders/{number}")
			return errors.New("Internal server error. Get /api/orders/number"),
				http.StatusInternalServerError, nil, 0
		}
		err = json.NewDecoder(res.Body).Decode(&accrualDecode)
		if err != nil && err != io.EOF {
			log.Println(err)
			return errors.New("Internal server error. NewDecoder"),
				http.StatusInternalServerError, nil, 0
		}

		if err != io.EOF {
			accrual.UploadedAt = accrual.UploadedAtTime.Format(time.RFC3339)
			if accrualDecode.Status == "REGISTERED" {
				accrual.Status = "NEW"
			} else {
				accrual.Status = accrualDecode.Status
			}
			balanceScore += accrualDecode.Accrual
			accrual.Accrual = accrualDecode.Accrual
			accrual.Order = ""
			orders = append(orders, accrual)
		}
	}

	if orders == nil {
		return errors.New("No data to answer"), http.StatusNoContent, nil, 0
	}

	return nil, http.StatusOK, orders, balanceScore
}
