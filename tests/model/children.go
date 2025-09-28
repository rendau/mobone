package model

type Contact struct {
	Phone string `json:"phone"`
	Email string `json:"email"`
}

type ContactEdit struct {
	Phone *string `json:"phone,omitempty"`
	Email *string `json:"email,omitempty"`
}
