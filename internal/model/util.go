package model

import (
	"database/sql/driver"
	"encoding/json"
	"github.com/jackc/pgx/v5/pgtype"
)

type TextArray []string

// Keep collection fields stable in JSON, including values read from SQL NULL.
func (a TextArray) MarshalJSON() ([]byte, error) {
	if a == nil {
		return []byte("[]"), nil
	}
	return json.Marshal([]string(a))
}

func (a TextArray) Value() (driver.Value, error) {
	if a == nil {
		return "{}", nil
	}
	encoded, err := pgtype.NewMap().Encode(pgtype.TextArrayOID, pgtype.TextFormatCode, []string(a), nil)
	if err != nil {
		return nil, err
	}
	return string(encoded), nil
}

func (a *TextArray) Scan(value any) error {
	var values []string
	if err := pgtype.NewMap().SQLScanner(&values).Scan(value); err != nil {
		return err
	}
	*a = TextArray(values)
	return nil
}
