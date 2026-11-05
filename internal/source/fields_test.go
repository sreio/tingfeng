package source

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
	"tingfeng/internal/domain"
	"tingfeng/internal/notify"
	"tingfeng/internal/secure"
)

func TestMultipleFieldsAndMessage(t *testing.T) {
	var calls atomic.Int64
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		if r.URL.Path == "/page" {
			w.Write([]byte(`<span class="buy">90441</span><span class="sell" data-value="88390"></span>`))
		} else {
			w.Write([]byte(`{"pronoinfo":[{"buyprice":"90441","sellprice":"88390"}]}`))
		}
	}))
	defer srv.Close()
	f := New(secure.Network{AllowPrivate: true, Timeout: time.Second}, 2<<20)
	calc := &domain.Calculation{Operation: "divide", Operand: "100", Decimals: 2}
	s := domain.Source{Type: "api", URL: srv.URL, Method: "GET", Fields: []domain.SourceField{
		{Key: "buyprice", Label: "买入价", Path: "/pronoinfo/0/buyprice", Unit: "元/克", Calculation: calc},
		{Key: "sellprice", Label: "卖出价", Path: "/pronoinfo/0/sellprice", Unit: "元/克", Calculation: calc},
	}}
	got, err := f.Fetch(context.Background(), s)
	if err != nil || calls.Load() != 1 {
		t.Fatal(err, calls.Load())
	}
	obs := got.Observation
	if string(obs.Value) != `{"buyprice":"904.41","sellprice":"883.90"}` || len(obs.Fields) != 2 || string(obs.Fields[0].Raw) != `"90441"` {
		t.Fatal(obs)
	}
	rule := domain.Rule{Trigger: "periodic", Template: `{{.Monitor}} | 买入 {{.Fields.buyprice}}{{.Units.buyprice}} | 卖出 {{.Fields.sellprice}}{{.Units.sellprice}} | 原值 {{.RawFields.buyprice}}`}
	payload, err := notify.Payload(domain.Monitor{Name: "测试行情", Source: s}, rule, obs, "", nil, time.UTC)
	if err != nil || payload.Text != "测试行情 | 买入 904.41元/克 | 卖出 883.90元/克 | 原值 90441" {
		t.Fatal(payload.Text, err)
	}
	rule.Template = ""
	payload, err = notify.Payload(domain.Monitor{Name: "测试行情", Source: s}, rule, obs, "", nil, time.UTC)
	if err != nil || !strings.Contains(payload.Text, "卖出价：883.90元/克") {
		t.Fatal(payload.Text, err)
	}
	rule.Template = `{{.Fields.missing}}`
	if _, err = notify.Payload(domain.Monitor{Source: s}, rule, obs, "", nil, time.UTC); err == nil {
		t.Fatal("missing template variable silently accepted")
	}
	s.Fields[1].Path = "/missing"
	if _, err = f.Fetch(context.Background(), s); err == nil || !strings.Contains(err.Error(), "卖出价") {
		t.Fatal("partial field failure was hidden", err)
	}
	s.Type = "page"
	s.URL = srv.URL + "/page"
	s.Fields[0].Selector = ".buy"
	s.Fields[1].Selector = ".sell"
	s.Fields[1].Attribute = "data-value"
	got, err = f.Fetch(context.Background(), s)
	if err != nil || got.Observation.Fields[1].Display != "883.90" {
		t.Fatal(got, err)
	}
}
