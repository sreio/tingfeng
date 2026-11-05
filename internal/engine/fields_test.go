package engine

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"tingfeng/internal/domain"
)

func TestCalculatedFieldThresholdAndHistory(t *testing.T) {
	x := setup(t)
	ctx := context.Background()
	c := x.e.Config()
	c.Monitors[0].Source.Fields = []domain.SourceField{
		{Key: "rawprice", Label: "原始价格", Path: "price"},
		{Key: "price", Label: "换算价格", Path: "price", Unit: "元", Calculation: &domain.Calculation{Operation: "divide", Operand: "100", Decimals: 2}},
	}
	r := &c.Monitors[0].Rules[0]
	r.Trigger = "threshold"
	r.Field = "price"
	r.Threshold = 1.30
	r.Cooldown = 0
	r.Template = `{{.Fields.price}}{{.Units.price}}; {{.Previous}} → {{.Value}}`
	if _, err := x.e.SaveConfig(ctx, c); err != nil {
		t.Fatal(err)
	}
	if _, err := x.e.Check(ctx, "mon"); err != nil {
		t.Fatal(err)
	}
	x.price.Store(131)
	if _, err := x.e.Check(ctx, "mon"); err != nil {
		t.Fatal(err)
	}
	deliveries, err := x.s.Deliveries(ctx, "mon", "", 1, 25)
	if err != nil || deliveries.Total != 1 || deliveries.Items[0].Payload.Text != "1.31元; 1.29 → 1.31" {
		t.Fatal(deliveries, err)
	}
	history, err := x.s.Records(ctx, "mon", 1, 25)
	if err != nil || len(history.Items[0].Fields) != 2 || history.Items[0].Fields[1].Display != "1.31" {
		t.Fatal(history, err)
	}
	c = x.e.Config()
	c.Monitors[0].Source.Fields[1].Unit = "千元"
	if _, err = x.e.SaveConfig(ctx, c); err != nil {
		t.Fatal(err)
	}
	history, err = x.s.Records(ctx, "mon", 1, 25)
	if err != nil || history.Items[0].Fields[1].Unit != "元" {
		t.Fatal("historical unit was changed", history, err)
	}
}

func TestChangeRuleChoosesFields(t *testing.T) {
	a := domain.Observation{Value: json.RawMessage(`{"a":"1","b":"2"}`), Fields: []domain.FieldValue{
		{Key: "a", Value: json.RawMessage(`"1"`)}, {Key: "b", Value: json.RawMessage(`"2"`)},
	}}
	b := domain.Clone(a)
	b.Value = json.RawMessage(`{"a":"1","b":"3"}`)
	b.Fields[1].Value = json.RawMessage(`"3"`)
	for _, tt := range []struct {
		field   string
		changed bool
	}{{"", true}, {"a", false}, {"b", true}} {
		r := domain.Rule{Trigger: "change", Field: tt.field}
		if got := ValueChanged(ruleValue(r, a), ruleValue(r, b)); got != tt.changed {
			t.Fatal(tt, got)
		}
	}
	if !strings.Contains(string(ruleValue(domain.Rule{Trigger: "threshold"}, a)), "1") {
		t.Fatal("default threshold no longer uses first field")
	}
}
