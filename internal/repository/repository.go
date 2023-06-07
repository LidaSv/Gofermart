package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"io"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

var (
	ErrNoData         = errors.New(`no data to answer`)
	ErrInternalServer = errors.New(`internal server error`)
)

type AccrualOrdersWithBalance struct {
	Accrual      []AccrualOrders
	BalanceScore float64
}

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
		return 0, ErrInternalServer
	}
	if errors.Is(err, pgx.ErrNoRows) || !totalWriteOff.Valid {
		return 0, nil
	}
	return totalWriteOff.Float64, nil
}

func LoadedOrderNumbers(conn *pgxpool.Pool, accrualSA, tk string) (AccrualOrdersWithBalance, error) {
	rows, err := conn.Query(context.Background(),
		`select id_order, event_time
			from orders
			where user_token = $1
			order by event_time desc`,
		tk)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		log.Println("LoadedOrderNumbers: select id_order, event_time: ", err)
		return AccrualOrdersWithBalance{}, ErrInternalServer
	}
	if errors.Is(err, pgx.ErrNoRows) {
		log.Println("LoadedOrderNumbers: pgx.ErrNoRows")
		return AccrualOrdersWithBalance{}, ErrNoData
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
		err := rows.Scan(&accrual.NumberOrder, &accrual.UploadedAtTime)
		if err != nil {
			log.Println("LoadedOrderNumbers: scan rows: ", err)
			return AccrualOrdersWithBalance{}, ErrInternalServer
		}

		accrual, err = GetHTTP(AccrualURL+accrual.NumberOrder, accrual)
		if err != nil {
			log.Println("LoadedOrderNumbers: Get /api/orders/{number}: ", err)
			return AccrualOrdersWithBalance{}, err
		}
		orders = append(orders, accrual)
		balanceScore += accrual.Accrual
	}

	if orders == nil {
		log.Println("LoadedOrderNumbers: no data to answer: ", err)
		return AccrualOrdersWithBalance{}, ErrNoData
	}

	var aowb AccrualOrdersWithBalance
	aowb.Accrual = orders
	aowb.BalanceScore = balanceScore

	return aowb, nil
}

func GetHTTP(AccrualURL string, accrual AccrualOrders) (AccrualOrders, error) {
	var accrualDecode DecodeAccrualOrders
	res, err := http.Get(AccrualURL)
	if err != nil && !errors.Is(io.EOF, err) {
		log.Println("LoadedOrderNumbers: http.Get(AccrualURL): ", err)
		return accrual, ErrInternalServer
	}

	if res.StatusCode == http.StatusNoContent || errors.Is(io.EOF, err) {
		log.Println("no data to answer in res.StatusCode: ", err)
		return accrual, ErrNoData
	}

	for res.StatusCode == http.StatusTooManyRequests {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		t := time.NewTimer(3 * time.Second)
		defer t.Stop()

		select {
		case <-t.C:
			GetHTTP(AccrualURL, accrual)
		case <-ctx.Done():
			log.Println("Waiting for connection")
			return accrual, ErrInternalServer
		}
	}

	if !errors.Is(io.EOF, err) {
		err = json.NewDecoder(res.Body).Decode(&accrualDecode)
		if err != nil && !errors.Is(io.EOF, err) {
			log.Println("LoadedOrderNumbers: NewDecoder: ", err)
			return accrual, ErrInternalServer
		}
	}
	defer res.Body.Close()

	if !errors.Is(io.EOF, err) {
		accrual.UploadedAt = accrual.UploadedAtTime.Format(time.RFC3339)
		accrual.Accrual = accrualDecode.Accrual
		if accrualDecode.Status == "REGISTERED" {
			accrual.Status = "NEW"
		} else {
			accrual.Status = accrualDecode.Status
		}

		accrual.Order = ""
	}

	return accrual, nil
}

func NoData(conn *pgxpool.Pool, tk string, orders []AccrualOrders) ([]AccrualOrders, error) {
	rows, err := conn.Query(context.Background(),
		`select id_order, event_time
			from orders 
			where user_token = $1`,
		tk)

	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		log.Println("UsersOrdersGet: select id_order, event_time: ", err)
		return nil, ErrInternalServer
	}

	if errors.Is(err, pgx.ErrNoRows) {
		log.Println("UsersOrdersGet: errors.Is(err, pgx.ErrNoRows): ", err)
		return nil, ErrNoData
	}
	defer rows.Close()

	for rows.Next() {
		var idOrder string
		var uploadedAt time.Time
		var order AccrualOrders

		err := rows.Scan(&idOrder, &uploadedAt)
		if err != nil {
			log.Println("LoadedOrderNumbers: scan rows: ", err)
			return nil, ErrInternalServer
		}

		order.NumberOrder = idOrder
		order.UploadedAt = uploadedAt.Format(time.RFC3339)
		order.Status = "NEW"
		orders = append(orders, order)
	}
	return orders, nil
}
