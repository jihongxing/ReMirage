package threat

import (
	"fmt"
	"os"
	"strings"

	"go.yaml.in/yaml/v2"
)

// IngressPolicyConfig YAML 配置结构
type IngressPolicyConfig struct {
	IngressPolicy struct {
		Rules []PolicyRuleConfig `yaml:"rules"`
	} `yaml:"ingress_policy"`
}

// PolicyRuleConfig YAML 中的策略规则配置
type PolicyRuleConfig struct {
	Condition string         `yaml:"condition"`
	Action    string         `yaml:"action"`
	Params    map[string]int `yaml:"params,omitempty"`
	Priority  int            `yaml:"priority"`
}

// LoadPolicyFromConfig 从 YAML 配置文件加载策略
func LoadPolicyFromConfig(path string) (*IngressPolicy, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("读取配置文件失败: %w", err)
	}
	return LoadPolicyFromBytes(data)
}

// LoadPolicyFromBytes 从 YAML 字节加载策略
func LoadPolicyFromBytes(data []byte) (*IngressPolicy, error) {
	var cfg IngressPolicyConfig
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("解析 YAML 失败: %w", err)
	}

	rules, err := convertRules(cfg.IngressPolicy.Rules)
	if err != nil {
		return nil, err
	}

	return NewIngressPolicy(rules), nil
}

// convertRules 将配置规则转换为策略规则
func convertRules(configs []PolicyRuleConfig) ([]PolicyRule, error) {
	rules := make([]PolicyRule, 0, len(configs))
	for _, c := range configs {
		action, err := parseAction(c.Action)
		if err != nil {
			return nil, fmt.Errorf("规则 %q: %w", c.Condition, err)
		}
		rules = append(rules, PolicyRule{
			Condition: c.Condition,
			Action:    action,
			Params:    c.Params,
			Priority:  c.Priority,
		})
	}
	return rules, nil
}

// parseAction 解析动作字符串
func parseAction(s string) (IngressAction, error) {
	switch strings.ToLower(s) {
	case "pass":
		return ActionPass, nil
	case "observe":
		return ActionObserve, nil
	case "throttle":
		return ActionThrottle, nil
	case "trap":
		return ActionTrap, nil
	case "drop":
		return ActionDrop, nil
	default:
		return ActionPass, fmt.Errorf("未知动作: %s", s)
	}
}
