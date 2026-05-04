package db

import (
	"database/sql/driver"
	"encoding/json"
	"fmt"
)

type Status string

const (
	StatusOpen     Status = "open"
	StatusClosed   Status = "closed"
	StatusAccepted Status = "accepted"
	StatusReviewed Status = "reviewed"
)

type EventData struct {
	Name    string `json:"name,omitempty"`
	Status  Status `json:"status,omitempty"`
	Comment string `json:"comment,omitempty"`
}

func (e EventData) String() string {
	b, _ := json.Marshal(e)
	bs := string(b)
	if bs == "{}" {
		return ""
	}
	return bs
}

func (e *EventData) Scan(value any) error {
	if value == nil || value == "" {
		return nil
	}

	var byt []byte
	switch v := value.(type) {
	case []byte:
		byt = v
	case string:
		byt = []byte(v)
	default:
		return fmt.Errorf("cannot scan %T into EventData", value)
	}

	if len(byt) == 0 {
		return nil
	}

	return json.Unmarshal(byt, e)
}

func (e EventData) Value() (driver.Value, error) {
	return json.Marshal(e)
}
