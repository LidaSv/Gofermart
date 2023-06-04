package repository

import (
	"context"
	"database/sql"
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

type DecodeAccrualOrders struct {
	Order   string  `json:"order"`
	Status  string  `json:"status"`
	Accrual float64 `json:"accrual"`
}

func TotalWriteOff(conn *pgxpool.Pool, tk string) (float64, error) {
	var totalWriteOff sql.NullFloat64
	err := conn.QueryRow(context.Background(),
		`select max(total_write_off)
			from balance 
			where user_token = $1`,
		tk).Scan(&totalWriteOff)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		log.Println("TotalWriteOff: select max: ", err)
		return 0, errors.New("internal server error. Select total_write_off")
	} else if errors.Is(err, pgx.ErrNoRows) {
		return 0, nil
	}
	if !totalWriteOff.Valid {
		return 0, nil
	}
	return totalWriteOff.Float64, nil
}

func LoadedOrderNumbers(conn *pgxpool.Pool, accrualSA, tk string) (int, []AccrualOrders, float64, error) {
	rows, err := conn.Query(context.Background(),
		`select id_order, event_time
			from orders
			where user_token = $1
			order by event_time desc`,
		tk)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		log.Println("LoadedOrderNumbers: select id_order, event_time: ", err)
		return http.StatusInternalServerError, nil, 0,
			errors.New(`internal server error. Select id_order, event_time`)
	}
	if errors.Is(err, pgx.ErrNoRows) {
		log.Println("LoadedOrderNumbers: pgx.ErrNoRows")
		return http.StatusNoContent, nil, 0, errors.New("no data to answer")
	}
	defer rows.Close()

	var AccrualURL string
	if strings.HasSuffix(accrualSA, "/") {
		AccrualURL = accrualSA + "api/orders/"
	} else {
		AccrualURL = accrualSA + "/api/orders/"
	}

	var orders []AccrualOrders
	var balanceScore float64
	for rows.Next() {
		var accrual AccrualOrders
		var accrualDecode DecodeAccrualOrders
		err := rows.Scan(&accrual.NumberOrder, &accrual.UploadedAtTime)
		if err != nil {
			log.Println("LoadedOrderNumbers: scan rows: ", err)
			return http.StatusInternalServerError, nil, 0,
				errors.New("internal server error. Scan AccrualOrders")
		}

		//AccrualURL+accrual.NumberOrder
		status, accrual, balanceScore1, err := GetHTTP(AccrualURL+accrual.NumberOrder, accrualDecode, accrual, balanceScore)
		if err != nil {
			log.Println("LoadedOrderNumbers: Get /api/orders/{number}: ", err)
			return status, nil, 0, err
		}
		orders = append(orders, accrual)
		balanceScore = balanceScore1
	}

	if orders == nil {
		log.Println("LoadedOrderNumbers: no data to answer: ", err)
		return http.StatusNoContent, nil, 0, errors.New("no data to answer")
	}

	return http.StatusOK, orders, balanceScore, nil
}

func GetHTTP(AccrualURL string, accrualDecode DecodeAccrualOrders, accrual AccrualOrders, balanceScore float64) (int, AccrualOrders, float64, error) {
	res, err := http.Get(AccrualURL)
	if err != nil && !errors.Is(io.EOF, err) {
		log.Println("LoadedOrderNumbers: Get /api/orders/{number}: ", err)
		return http.StatusInternalServerError, accrual, balanceScore,
			errors.New("internal server error. Get /api/orders/number")
	}

	if res.StatusCode == http.StatusNoContent {
		return http.StatusNoContent, accrual, balanceScore, errors.New("no data to answer in res.StatusCode")
	}

	if res.StatusCode == http.StatusTooManyRequests {
		time.Sleep(3 * time.Second)
		GetHTTP(AccrualURL, accrualDecode, accrual, balanceScore)
	}

	if !errors.Is(io.EOF, err) {
		err = json.NewDecoder(res.Body).Decode(&accrualDecode)
		if err != nil && !errors.Is(io.EOF, err) {
			log.Println("LoadedOrderNumbers: NewDecoder: ", err)
			return http.StatusInternalServerError, accrual, balanceScore,
				errors.New("internal server error. NewDecoder")
		}
	}

	if errors.Is(io.EOF, err) {
		log.Println("LoadedOrderNumbers: no data to answer: ", err)
		return http.StatusNoContent, accrual, balanceScore, errors.New("no data to answer in Get")
	}
	defer res.Body.Close()

	if !errors.Is(io.EOF, err) {
		accrual.UploadedAt = accrual.UploadedAtTime.Format(time.RFC3339)
		if accrualDecode.Status == "REGISTERED" {
			accrual.Status = "NEW"
		} else {
			accrual.Status = accrualDecode.Status
		}

		balanceScore += accrualDecode.Accrual
		accrual.Order = ""
	}
	return 0, accrual, balanceScore, nil
}
