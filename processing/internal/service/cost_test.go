package service

import (
	"math/big"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"
)

func makeNumeric(value string) pgtype.Numeric {
	var n pgtype.Numeric
	if err := n.Scan(value); err != nil {
		panic("makeNumeric: " + err.Error())
	}
	return n
}

func assertNumericEquals(t *testing.T, got pgtype.Numeric, expected string) {
	t.Helper()
	if !got.Valid {
		t.Fatal("expected valid numeric result")
	}
	gotStr := numericToBigFloat(got).Text('f', 8)
	if gotStr != expected {
		t.Errorf("expected %s, got %s", expected, gotStr)
	}
}

func TestCalculateCost_BasicArithmetic(t *testing.T) {
	// GPT-4o: input $2.50/1M, output $10.00/1M
	inputPrice := makeNumeric("0.0000025")
	outputPrice := makeNumeric("0.00001")

	result := calculateCost(1000, 500, inputPrice, outputPrice)
	// 1000 * 0.0000025 + 500 * 0.00001 = 0.0025 + 0.005 = 0.0075
	assertNumericEquals(t, result, "0.00750000")
}

func TestCalculateCost_ZeroTokens(t *testing.T) {
	inputPrice := makeNumeric("0.0000025")
	outputPrice := makeNumeric("0.00001")

	result := calculateCost(0, 0, inputPrice, outputPrice)
	assertNumericEquals(t, result, "0.00000000")
}

func TestCalculateCost_LargeTokenCounts(t *testing.T) {
	// 100k input, 50k output
	inputPrice := makeNumeric("0.00003")
	outputPrice := makeNumeric("0.00006")

	result := calculateCost(100_000, 50_000, inputPrice, outputPrice)
	// 100000 * 0.00003 + 50000 * 0.00006 = 3.0 + 3.0 = 6.0
	assertNumericEquals(t, result, "6.00000000")
}

func TestCalculateCost_OnlyInputTokens(t *testing.T) {
	inputPrice := makeNumeric("0.0000025")
	outputPrice := makeNumeric("0.00001")

	result := calculateCost(1000, 0, inputPrice, outputPrice)
	assertNumericEquals(t, result, "0.00250000")
}

func TestNumericToBigFloat_InvalidNumeric(t *testing.T) {
	n := pgtype.Numeric{Valid: false}
	f := numericToBigFloat(n)
	if f.Text('f', 8) != "0.00000000" {
		t.Errorf("expected 0 for invalid numeric, got %s", f.Text('f', 8))
	}
}

func TestNumericToBigFloat_NilInt(t *testing.T) {
	n := pgtype.Numeric{Valid: true, Int: nil, Exp: 0}
	f := numericToBigFloat(n)
	if f.Text('f', 8) != "0.00000000" {
		t.Errorf("expected 0 for nil int, got %s", f.Text('f', 8))
	}
}

func TestNumericToBigFloat_PositiveExponent(t *testing.T) {
	// 5 * 10^2 = 500
	n := pgtype.Numeric{Valid: true, Int: big.NewInt(5), Exp: 2}
	f := numericToBigFloat(n)
	if f.Text('f', 0) != "500" {
		t.Errorf("expected 500, got %s", f.Text('f', 0))
	}
}

func TestNumericToBigFloat_NegativeExponent(t *testing.T) {
	// 25 * 10^-3 = 0.025
	n := pgtype.Numeric{Valid: true, Int: big.NewInt(25), Exp: -3}
	f := numericToBigFloat(n)
	if f.Text('f', 3) != "0.025" {
		t.Errorf("expected 0.025, got %s", f.Text('f', 3))
	}
}
