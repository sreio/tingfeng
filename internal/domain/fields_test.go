package domain

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestDecimalCalculation(t *testing.T) {
	for _, tt := range []struct {
		raw, op, operand, want string
		digits                 int
	}{
		{`"90441"`, "divide", "100", "904.41", 2},
		{`"88390"`, "divide", "100", "883.90", 2},
		{`"090441"`, "divide", "100", "904.41", 2},
		{`0.1`, "add", "0.2", "0.30", 2},
		{`"1.005"`, "round", "", "1.01", 2},
		{`"-1.005"`, "round", "", "-1.01", 2},
		{`"-0.001"`, "round", "", "0.00", 2},
		{`"9007199254740993"`, "add", "1", "9007199254740994", 0},
		{`1e3`, "multiply", "0.01", "10.000", 3},
		{`"10"`, "subtract", "0.1", "9.90", 2},
	} {
		t.Run(tt.raw+"_"+tt.op, func(t *testing.T) {
			got, err := Calculate(json.RawMessage(tt.raw), &Calculation{Operation: tt.op, Operand: tt.operand, Decimals: tt.digits})
			if err != nil || Display(got) != tt.want {
				t.Fatalf("got %s, %v; want %s", got, err, tt.want)
			}
		})
	}
	for _, raw := range []string{`null`, `true`, `"¥90441"`, `[]`, `"1/2"`, `"1e99999999"`} {
		if _, err := Calculate(json.RawMessage(raw), &Calculation{Operation: "divide", Operand: "100", Decimals: 2}); err == nil {
			t.Fatalf("invalid value accepted: %s", raw)
		}
	}
	if _, err := Calculate(json.RawMessage(`"99"`), &Calculation{Operation: "divide", Operand: "0", Decimals: 2}); err == nil {
		t.Fatal("division by zero accepted")
	}
}

func TestFieldConfigurationValidation(t *testing.T) {
	s := Source{Type: "api", Fields: []SourceField{{Key: "price", Label: "价格"}}}
	if err := ValidateFields(s); err != nil {
		t.Fatal(err)
	}
	s.Fields = append(s.Fields, s.Fields[0])
	if err := ValidateFields(s); err == nil {
		t.Fatal("duplicate variables accepted")
	}
	s.Fields = s.Fields[:1]
	for _, key := range []string{"__proto__", "constructor", "bad-key", "1price", strings.Repeat("a", 33)} {
		s.Fields[0].Key = key
		if err := ValidateFields(s); err == nil {
			t.Fatal("invalid variable accepted", key)
		}
	}
	s.Fields[0].Key = "price"
	if err := ValidateRuleField(Rule{Field: "removed"}, s); err == nil {
		t.Fatal("deleted field reference accepted")
	}
}
