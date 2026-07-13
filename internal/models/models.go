package models

import "time"

type User struct {
	ID                 int64     `json:"id"`
	Email              string    `json:"email,omitempty"`
	Name               string    `json:"name,omitempty"`
	Balance            float64   `json:"balance"`
	Goal               float64   `json:"-"`
	Spent              float64   `json:"-"`
	LowBalanceNotified bool      `json:"low_balance_notified,omitempty"`
	CreatedAt          time.Time `json:"created_at,omitempty"`
	UpdatedAt          time.Time `json:"updated_at,omitempty"`
}

func (u *User) ComputeBalance() { u.Balance = u.Goal - u.Spent }
