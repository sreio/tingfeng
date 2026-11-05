package httpapi

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"
	"time"
	"tingfeng/internal/config"
	"tingfeng/internal/domain"
	"tingfeng/internal/engine"
	"tingfeng/internal/notify"
	"tingfeng/internal/secure"
	"tingfeng/internal/source"
	"tingfeng/internal/storage"
)

func TestNamedFieldPreviewAndTestDelivery(t *testing.T) {
	store, err := storage.Open(t.TempDir(), true)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	network := secure.Network{AllowPrivate: true, Timeout: time.Second}
	eng, err := engine.New(store, source.New(network, 2<<20), notify.New(network), 1)
	if err != nil {
		t.Fatal(err)
	}
	server := New(eng, config.Config{}, fstest.MapFS{})
	call := func(handler http.HandlerFunc, body any, want int) []byte {
		t.Helper()
		raw, _ := json.Marshal(body)
		request := httptest.NewRequest("POST", "/", bytes.NewReader(raw))
		request.Header.Set("Content-Type", "application/json")
		recorder := httptest.NewRecorder()
		handler(recorder, request)
		if recorder.Code != want {
			t.Fatalf("status %d, want %d: %s", recorder.Code, want, recorder.Body.String())
		}
		return recorder.Body.Bytes()
	}
	field := domain.SourceField{Key: "price", Label: "价格", Unit: "元/克"}
	rule := domain.Rule{ID: "rule", Trigger: "change", Template: `{{.Fields.price}}{{.Units.price}}`}
	obs := domain.Observation{Display: "904.41", Fields: []domain.FieldValue{{Key: "price", Label: "价格", Unit: "元/克", Value: json.RawMessage(`"904.41"`), Raw: json.RawMessage(`"90441"`), Display: "904.41"}}}
	body := map[string]any{"name": "测试", "source": domain.Source{Type: "api", Fields: []domain.SourceField{field}}, "rule": rule, "observation": obs}
	var payload domain.Payload
	if err = json.Unmarshal(call(server.messagePreview, body, 200), &payload); err != nil || payload.Text != "904.41元/克" {
		t.Fatal(payload, err)
	}
	rule.Template = `{{.Fields.absent}}`
	body["rule"] = rule
	call(server.messagePreview, body, 400)
	rule.Template = ""
	rule.Field = "absent"
	body["rule"] = rule
	call(server.messagePreview, body, 400)
	hook := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		if !strings.Contains(string(raw), `"price":"904.41"`) {
			t.Errorf("test delivery lost named variables: %s", raw)
		}
		w.WriteHeader(204)
	}))
	defer hook.Close()
	call(server.testChannel, map[string]any{
		"channel": domain.Channel{ID: "hook", Name: "fixture", Type: "webhook", URL: hook.URL, Method: "POST", BodyTemplate: `{"price":{{json .Fields.price}}}`},
		"payload": payload,
	}, 200)
}
