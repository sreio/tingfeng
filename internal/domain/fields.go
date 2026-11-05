package domain

import (
	"encoding/json"
	"fmt"
	"math/big"
	"regexp"
	"strings"
)

type SourceField struct {
	Key         string       `json:"key"`
	Label       string       `json:"label"`
	Path        string       `json:"path"`
	Selector    string       `json:"selector"`
	Attribute   string       `json:"attribute"`
	Unit        string       `json:"unit"`
	Calculation *Calculation `json:"calculation,omitempty"`
}

type Calculation struct {
	Operation string `json:"operation"`
	Operand   string `json:"operand"`
	Decimals  int    `json:"decimals"`
}

type FieldValue struct {
	Key     string          `json:"key"`
	Label   string          `json:"label"`
	Unit    string          `json:"unit"`
	Raw     json.RawMessage `json:"raw"`
	Value   json.RawMessage `json:"value"`
	Display string          `json:"display"`
}

var fieldKey = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9_]{0,31}$`)
var decimal = regexp.MustCompile(`^[+-]?[0-9]{1,30}(?:\.[0-9]{1,18})?(?:[eE][+-]?[0-9]{1,2})?$`)

func validFieldKey(s string) bool {
	return fieldKey.MatchString(s) && s != "constructor" && s != "prototype"
}

func decimalValue(s string) (*big.Rat, error) {
	s = strings.TrimSpace(s)
	if !decimal.MatchString(s) {
		return nil, fmt.Errorf("需要有效的十进制数值")
	}
	// A decimal point prevents leading-zero strings from being interpreted as octal.
	if !strings.ContainsAny(s, ".eE") {
		s += ".0"
	}
	n, ok := new(big.Rat).SetString(s)
	if !ok {
		return nil, fmt.Errorf("需要有效的十进制数值")
	}
	return n, nil
}

func ValidateFields(s Source) error {
	if len(s.Fields) > 16 {
		return fmt.Errorf("每个来源最多提取 16 个字段")
	}
	if s.Type == "rss" && len(s.Fields) != 0 {
		return fmt.Errorf("RSS 使用订阅条目，无需配置提取字段")
	}
	seen := map[string]bool{}
	for _, f := range s.Fields {
		if !validFieldKey(f.Key) || seen[f.Key] {
			return fmt.Errorf("字段变量名须以字母开头，使用字母、数字或下划线，最多 32 位且不能重复")
		}
		seen[f.Key] = true
		if strings.TrimSpace(f.Label) == "" || len([]rune(f.Label)) > 100 || len(f.Path) > 256 || len(f.Selector) > 512 || len(f.Attribute) > 100 || len(f.Unit) > 30 {
			return fmt.Errorf("字段名称不能为空，字段配置不能超过长度上限")
		}
		if err := ValidateCalculation(f.Calculation); err != nil {
			return fmt.Errorf("字段「%s」：%w", f.Label, err)
		}
	}
	return nil
}

func ValidateCalculation(c *Calculation) error {
	if c == nil {
		return nil
	}
	if !integer(c.Decimals, 0, 12) {
		return fmt.Errorf("小数位数为 0～12")
	}
	switch c.Operation {
	case "round":
		return nil
	case "divide", "multiply", "add", "subtract":
		n, err := decimalValue(c.Operand)
		if err != nil {
			return fmt.Errorf("换算参数需要有效的十进制数值")
		}
		if c.Operation == "divide" && n.Sign() == 0 {
			return fmt.Errorf("除数不能为 0")
		}
	default:
		return fmt.Errorf("请选择有效的换算方式")
	}
	return nil
}

func Calculate(raw json.RawMessage, c *Calculation) (json.RawMessage, error) {
	if c == nil {
		return raw, nil
	}
	if err := ValidateCalculation(c); err != nil {
		return nil, err
	}
	text := string(raw)
	if len(raw) > 0 && raw[0] == '"' {
		if err := json.Unmarshal(raw, &text); err != nil {
			return nil, fmt.Errorf("数值格式无效")
		}
	}
	n, err := decimalValue(text)
	if err != nil {
		return nil, fmt.Errorf("提取结果不是可换算的数值，请选择纯数字字段")
	}
	if c.Operation != "round" {
		operand, _ := decimalValue(c.Operand)
		switch c.Operation {
		case "divide":
			n.Quo(n, operand)
		case "multiply":
			n.Mul(n, operand)
		case "add":
			n.Add(n, operand)
		case "subtract":
			n.Sub(n, operand)
		}
	}
	value := n.FloatString(c.Decimals)
	if strings.Trim(value, "-0.") == "" {
		value = strings.TrimPrefix(value, "-")
	}
	// Decimal strings preserve precision and trailing zeros through browsers and SQLite.
	return json.Marshal(value)
}

func (s Source) UnitFor(key string) string {
	if len(s.Fields) == 0 {
		return s.Unit
	}
	for i, f := range s.Fields {
		if f.Key == key || key == "" && i == 0 {
			return f.Unit
		}
	}
	return ""
}

func (o Observation) Select(key string) (json.RawMessage, string) {
	if len(o.Fields) == 0 && key == "" {
		return o.Value, o.Display
	}
	for i, f := range o.Fields {
		if f.Key == key || key == "" && i == 0 {
			return f.Value, f.Display
		}
	}
	return nil, ""
}

func ValidateRuleField(r Rule, s Source) error {
	if r.Field == "" {
		return nil
	}
	for _, f := range s.Fields {
		if f.Key == r.Field {
			return nil
		}
	}
	return fmt.Errorf("提醒引用的字段已不存在，请重新选择判断字段")
}
