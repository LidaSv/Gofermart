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

type DecodeAccrualOrders struct {
	Order   string  `json:"order"`
	Status  string  `json:"status"`
	Accrual float64 `json:"accrual,omitempty"`
}

func TotalWriteOff(conn *pgxpool.Pool, tk string) (float64, error) {
	var totalWriteOff float64
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
	return totalWriteOff, nil
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
		AccrualURL = accrualSA
	} else {
		AccrualURL = accrualSA + "/"
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

		res, err := http.Get(AccrualURL + accrual.NumberOrder)
		if err != nil {
			log.Println("LoadedOrderNumbers: Get /api/orders/{number}: ", err)
			return http.StatusInternalServerError, nil, 0,
				errors.New("internal server error. Get /api/orders/number")
		}

		//dec := json.NewDecoder(res.Body)
		//for {
		//	t, err := dec.Token()
		//	if err == io.EOF {
		//		break
		//	}
		//	if err != nil {
		//		log.Fatal(err)
		//	}
		//	log.Printf("%T: %v", t, t)
		//	if dec.More() {
		//		log.Printf(" (more)")
		//	}
		//	log.Printf("\n")
		//}

		err = json.NewDecoder(res.Body).Decode(&accrualDecode)
		if err != nil && !errors.Is(io.EOF, err) {
			log.Println("LoadedOrderNumbers: NewDecoder: ", err)
			return http.StatusInternalServerError, nil, 0,
				errors.New("internal server error. NewDecoder")
		}
		defer res.Body.Close()

		if !errors.Is(io.EOF, err) {
			accrual.UploadedAt = accrual.UploadedAtTime.Format(time.RFC3339)
			if accrualDecode.Status == "REGISTERED" {
				accrual.Status = "NEW"
			} else {
				accrual.Status = accrualDecode.Status
			}

			//num, err := accrualDecode.Accrual.Float64()
			//if err != nil {
			//	return http.StatusInternalServerError, nil, 0,
			//		errors.New("internal server error. strconv.ParseFloat")
			//}
			balanceScore += accrualDecode.Accrual
			accrual.Accrual = accrualDecode.Accrual
			accrual.Order = ""
			orders = append(orders, accrual)
		}
	}

	if orders == nil {
		log.Println("LoadedOrderNumbers: no data to answer: ", err)
		return http.StatusNoContent, nil, 0, errors.New("no data to answer")
	}

	return http.StatusOK, orders, balanceScore, nil
}
