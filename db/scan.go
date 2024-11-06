package db

import (
	"database/sql"
	"fmt"
)

func Scan1[T any](rows *sql.Rows) (*T, error) {
	if !rows.Next() {
		return nil, fmt.Errorf("not found")
	}
	var ret T
	if err := rows.Scan(&ret); err != nil {
		return nil, fmt.Errorf("not scannable: %w", err)
	}
	return &ret, nil
}

func Scan2[T any, U any](rows *sql.Rows) (*T, *U, error) {
	if !rows.Next() {
		return nil, nil, fmt.Errorf("not found")
	}
	var (
		ret1 T
		ret2 U
	)
	if err := rows.Scan(&ret1, &ret2); err != nil {
		return nil, nil, fmt.Errorf("not scannable: %w", err)
	}
	return &ret1, &ret2, nil
}

func Scan3NoNext[T any, U any, V any](rows *sql.Rows) (*T, *U, *V, error) {
	var (
		ret1 T
		ret2 U
		ret3 V
	)
	if err := rows.Scan(&ret1, &ret2, &ret3); err != nil {
		return nil, nil, nil, fmt.Errorf("not scannable: %w", err)
	}
	return &ret1, &ret2, &ret3, nil
}
