package httpapi

import "testing"

func TestCSVFormulaEscaping(t *testing.T) {
	for _, v := range []string{"=1+1", "+SUM(A1)", "@SUM(A1)", "-1+2", "\t=1", "\r=1", "\n=1"} {
		if csvSafe(v) != "'"+v {
			t.Fatal("unescaped spreadsheet formula")
		}
	}
	for _, v := range []string{"131.75", "gold", "中文", `{"items":2}`} {
		if csvSafe(v) != v {
			t.Fatal("ordinary value changed")
		}
	}
}
