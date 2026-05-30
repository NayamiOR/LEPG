package errors

import (
	"errors"
	"fmt"
	"strings"
)

var (
	ErrInvalidMagic     = errors.New("protocol: invalid magic number")
	ErrChecksumMismatch = errors.New("protocol: checksum mismatch")
	ErrPayloadTooLarge  = errors.New("protocol: payload size exceeds limit")
	ErrUnexpectedEOF    = errors.New("protocol: unexpected end of frame") // 区别于 io.EOF
	ErrInvalidVersion    = errors.New("protocol: unsupported version")
	ErrHandshakeRejected = errors.New("handshake: server rejected connection")
)

// ConfigNotSetError 配置未设置错误
type ConfigNotSetError struct {
	MissingFields []string
}

func (e *ConfigNotSetError) Error() string {
	return "Missing configs: " + strings.Join(e.MissingFields, ", ")
}

// NewConfigNotSetError 创建配置未设置错误
func NewConfigNotSetError(fields []string) error {
	return &ConfigNotSetError{MissingFields: fields}
}

type ConfigInvalidError struct {
	Field  string
	Reason string
}

func (e *ConfigInvalidError) Error() string {
	return fmt.Sprintf("invalid config '%s': %s", e.Field, e.Reason)
}

func NewConfigInvalidError(field string, reason string) error {
	return &ConfigInvalidError{Field: field, Reason: reason}
}

// ConfigValidationErrors 配置验证错误聚合
type ConfigValidationErrors struct {
	Errors []error
}

func (e *ConfigValidationErrors) Error() string {
	if len(e.Errors) == 0 {
		return "validation failed"
	}

	var msgs []string
	for _, err := range e.Errors {
		msgs = append(msgs, err.Error())
	}
	return "validation failed:\n  - " + strings.Join(msgs, "\n  - ")
}

// Unwrap 返回第一个错误，支持 errors.Is/As
func (e *ConfigValidationErrors) Unwrap() []error {
	return e.Errors
}

// NewConfigValidationErrors 创建验证错误聚合
func NewConfigValidationErrors(errs []error) error {
	if len(errs) == 0 {
		return nil
	}
	if len(errs) == 1 {
		return errs[0]
	}
	return &ConfigValidationErrors{Errors: errs}
}

// Wrap 包装错误，添加上下文
func Wrap(err error, message string) error {
	if err == nil {
		return nil
	}
	return fmt.Errorf("%s: %w", message, err)
}
