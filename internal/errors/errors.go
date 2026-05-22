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
	ErrInvalidVersion   = errors.New("protocol: unsupported version")
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

// Wrap 包装错误，添加上下文
func Wrap(err error, message string) error {
	if err == nil {
		return nil
	}
	return fmt.Errorf("%s: %w", message, err)
}
