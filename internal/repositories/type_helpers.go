package repositories

import (
	"time"

	"github.com/jackc/pgx/v5/pgtype"
)

func timestamptzFromTime(t time.Time) pgtype.Timestamptz {
	if t.IsZero() {
		return pgtype.Timestamptz{}
	}
	return pgtype.Timestamptz{
		Time:  t.UTC(),
		Valid: true,
	}
}

func textFromString(value string) pgtype.Text {
	if value == "" {
		return pgtype.Text{}
	}
	return pgtype.Text{
		String: value,
		Valid:  true,
	}
}

func textFromNullableString(value string) pgtype.Text {
	if value == "" {
		return pgtype.Text{}
	}
	return pgtype.Text{
		String: value,
		Valid:  true,
	}
}
