package load

import "time"

type Result struct {
	StartTime         *time.Time `json:"start_time"`
	EndTime           *time.Time `json:"end_time"`
	TotalTransactions uint       `json:"total_transaction_count"`
	NodeCount         uint       `json:"node_count"`
	AchievedTps       float64    `json:"achieved_tps"`
}
