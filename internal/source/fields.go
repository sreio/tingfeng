package source

import (
	"encoding/json"
	"fmt"
	"strings"
	"tingfeng/internal/domain"

	"github.com/andybalholm/cascadia"
	"golang.org/x/net/html"
)

func collectFields(obs *domain.Observation, specs []domain.SourceField, extract func(domain.SourceField) (any, error)) error {
	values := map[string]json.RawMessage{}
	for _, spec := range specs {
		raw, err := extract(spec)
		if err != nil {
			return fmt.Errorf("字段「%s」：%w", spec.Label, err)
		}
		encoded, err := json.Marshal(raw)
		if err != nil || len(encoded) > 32768 {
			return fmt.Errorf("字段「%s」结果过大，请选择更具体的字段", spec.Label)
		}
		value, err := domain.Calculate(encoded, spec.Calculation)
		if err != nil {
			return fmt.Errorf("字段「%s」：%w", spec.Label, err)
		}
		values[spec.Key] = value
		obs.Fields = append(obs.Fields, domain.FieldValue{
			Key: spec.Key, Label: spec.Label, Unit: spec.Unit,
			Raw: encoded, Value: value, Display: domain.Display(value),
		})
	}
	obs.Value, _ = json.Marshal(values)
	obs.Display = obs.Fields[0].Display
	return nil
}

func pageValue(root *html.Node, selector, attribute string) (string, error) {
	if selector == "" {
		selector = "body"
	}
	matcher, err := cascadia.Compile(selector)
	if err != nil {
		return "", fmt.Errorf("CSS 选择器无效")
	}
	nodes := cascadia.QueryAll(root, matcher)
	if len(nodes) == 0 {
		return "", fmt.Errorf("页面没有匹配的元素，请检查选择器；JavaScript 渲染页面需要使用其数据接口")
	}
	parts := []string{}
	for _, n := range nodes {
		if len(parts) >= 100 {
			break
		}
		if attribute != "" {
			for _, a := range n.Attr {
				if a.Key == attribute {
					parts = append(parts, a.Val)
				}
			}
		} else {
			parts = append(parts, nodeText(n))
		}
	}
	if len(parts) == 0 {
		return "", fmt.Errorf("匹配元素中没有指定属性")
	}
	return strings.TrimSpace(strings.Join(parts, "\n")), nil
}
