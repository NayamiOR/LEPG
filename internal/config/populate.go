package config

import (
	"fmt"
	"LEPG/internal/config/provider"
	"reflect"
	"strconv"
	"strings"
)

// Validatable 配置验证接口
type Validatable interface {
	Validate() error
}

// Source 表示配置来源的 bitmask
type Source uint

const (
	SourceDefault Source = 1 << iota
	SourceEnv
	SourceFile
	SourceFlag
)

// sourceOf 通过类型断言判断 provider 对应的 Source
func sourceOf(p IProvider) Source {
	switch p.(type) {
	case *provider.DefaultProvider:
		return SourceDefault
	case *provider.EnvProvider:
		return SourceEnv
	case *provider.FileProvider:
		return SourceFile
	case *provider.FlagProvider:
		return SourceFlag
	default:
		return 0
	}
}

// parseSources 解析 sources tag 字符串为 Source bitmask
func parseSources(tag string) Source {
	if tag == "" {
		return 0
	}
	var s Source
	for _, part := range strings.Split(tag, ",") {
		switch strings.TrimSpace(part) {
		case "default":
			s |= SourceDefault
		case "env":
			s |= SourceEnv
		case "file":
			s |= SourceFile
		case "flag":
			s |= SourceFlag
		}
	}
	return s
}

// PopulateFromProvider 通过反射读取 struct 的 config/default/sources tag，
// 从 provider 链填充值。递归处理子结构体。
//
// 规则：
//   - 无 sources tag → 跳过该字段
//   - 有 sources tag → 严格只从白名单中的 provider 读取
//   - 无 config tag 但 Kind==Struct 且子字段有 config tag → 递归
//   - 无 config tag 且非 struct（如 slice、pointer）→ 跳过
//
// 前提：ProviderChain.providers 切片存储顺序为
// [DefaultProvider, EnvProvider, FileProvider, FlagProvider]（低→高优先级）。
// 本函数反向遍历（index len-1 → 0），保证高优先级先命中。
func PopulateFromProvider(cfg any, p IProvider) error {
	chain, ok := p.(*ProviderChain)
	if !ok {
		return fmt.Errorf("PopulateFromProvider: expected *ProviderChain, got %T", p)
	}

	val := reflect.ValueOf(cfg)
	if val.Kind() != reflect.Ptr || val.Elem().Kind() != reflect.Struct {
		return fmt.Errorf("PopulateFromProvider: cfg must be a pointer to struct")
	}

	return populateStruct(val.Elem(), chain)
}

func populateStruct(val reflect.Value, chain *ProviderChain) error {
	typ := val.Type()

	for i := 0; i < typ.NumField(); i++ {
		field := typ.Field(i)
		fieldVal := val.Field(i)

		// 跳过未导出字段
		if !field.IsExported() {
			continue
		}

		sourcesTag := field.Tag.Get("sources")
		configKey := field.Tag.Get("config")

		// 无 sources tag → 跳过（不参与 provider 链读取）
		if sourcesTag == "" {
			// 如果是 struct，检查子字段是否有 config tag，有则递归
			if fieldVal.Kind() == reflect.Struct && hasConfigFields(fieldVal.Type()) {
				if err := populateStruct(fieldVal, chain); err != nil {
					return err
				}
			}
			continue
		}

		// 有 sources tag 但无 config tag → 报错
		if configKey == "" {
			return fmt.Errorf("PopulateFromProvider: field %s has sources tag but no config tag", field.Name)
		}

		sources := parseSources(sourcesTag)
		if err := populateField(fieldVal, configKey, sources, field.Tag.Get("default"), chain); err != nil {
			return fmt.Errorf("field %s: %w", field.Name, err)
		}

		// 如果字段是 struct 且已填充，也递归处理其子字段
		if fieldVal.Kind() == reflect.Struct && hasConfigFields(fieldVal.Type()) {
			if err := populateStruct(fieldVal, chain); err != nil {
				return err
			}
		}
	}

	return nil
}

// populateField 从 provider 链中读取单个字段的值
func populateField(fieldVal reflect.Value, key string, sources Source, defaultTag string, chain *ProviderChain) error {
	// 反向遍历 providers（高→低优先级）
	for i := len(chain.providers) - 1; i >= 0; i-- {
		p := chain.providers[i]
		src := sourceOf(p)

		// 跳过不在白名单的 provider
		if sources&src == 0 {
			continue
		}

		switch fieldVal.Kind() {
		case reflect.String:
			if p.IsSet(key) {
				fieldVal.SetString(p.GetString(key))
				return nil
			}
		case reflect.Int:
			if p.IsSet(key) {
				fieldVal.SetInt(int64(p.GetInt(key)))
				return nil
			}
		case reflect.Bool:
			if p.IsSet(key) {
				fieldVal.SetBool(p.GetBool(key))
				return nil
			}
		}
	}

	// 所有 provider 都未命中，尝试 default tag
	if defaultTag != "" && sources&SourceDefault != 0 {
		return setFromDefaultTag(fieldVal, defaultTag)
	}

	return nil
}

// setFromDefaultTag 从 default tag 值设置字段
func setFromDefaultTag(fieldVal reflect.Value, defaultTag string) error {
	switch fieldVal.Kind() {
	case reflect.String:
		fieldVal.SetString(defaultTag)
	case reflect.Int:
		n, err := strconv.Atoi(defaultTag)
		if err != nil {
			return fmt.Errorf("invalid default int value %q: %w", defaultTag, err)
		}
		fieldVal.SetInt(int64(n))
	case reflect.Bool:
		b, err := strconv.ParseBool(defaultTag)
		if err != nil {
			return fmt.Errorf("invalid default bool value %q: %w", defaultTag, err)
		}
		fieldVal.SetBool(b)
	}
	return nil
}

// hasConfigFields 检查 struct 类型中是否有任何字段（或嵌套字段）带有 config tag
func hasConfigFields(t reflect.Type) bool {
	for i := 0; i < t.NumField(); i++ {
		field := t.Field(i)
		if field.Tag.Get("config") != "" {
			return true
		}
		if field.Type.Kind() == reflect.Struct && hasConfigFields(field.Type) {
			return true
		}
	}
	return false
}

// ExtractDefaults 从 struct 的 config+default tag 提取默认值，
// 返回 flat map[string]any，用于 DefaultProvider 和 initCmd。
//
// 规则：
//   - 同时有 config 和 default tag（且 default 值非空）→ 加入 map
//   - Kind == Struct → 递归提取子字段的默认值
//   - 无 default tag 的字段 → 不加入 map（init 不生成该 key）
func ExtractDefaults(cfgs ...any) map[string]any {
	result := make(map[string]any)
	for _, cfg := range cfgs {
		val := reflect.ValueOf(cfg)
		if val.Kind() != reflect.Ptr || val.Elem().Kind() != reflect.Struct {
			continue
		}
		extractFromStruct(val.Elem(), result)
	}
	return result
}

func extractFromStruct(val reflect.Value, result map[string]any) {
	typ := val.Type()

	for i := 0; i < typ.NumField(); i++ {
		field := typ.Field(i)
		configKey := field.Tag.Get("config")
		defaultVal := field.Tag.Get("default")

		if configKey != "" && defaultVal != "" {
			result[configKey] = parseDefault(defaultVal, field.Type.Kind())
			continue
		}

		// 递归处理子结构体
		if val.Field(i).Kind() == reflect.Struct {
			extractFromStruct(val.Field(i), result)
		}
	}
}

// parseDefault 将 default tag 字符串解析为对应类型的值
func parseDefault(s string, kind reflect.Kind) any {
	switch kind {
	case reflect.Int:
		if n, err := strconv.Atoi(s); err == nil {
			return n
		}
	case reflect.Bool:
		if b, err := strconv.ParseBool(s); err == nil {
			return b
		}
	}
	return s // string 或无法解析时返回原字符串
}
