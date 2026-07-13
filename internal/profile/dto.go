package profile

type PatchUserRequest struct {
	Email   *string  `json:"email,omitempty"`
	Name    *string  `json:"name,omitempty"`
	Balance *float64 `json:"balance,omitempty"`
	Goal    *float64 `json:"goal,omitempty"`
	Spent   *float64 `json:"spent,omitempty"`
}
