package errors

import "errors"

var (
	ErrInvalidMagic     = errors.New("protocol: invalid magic number")
	ErrChecksumMismatch = errors.New("protocol: checksum mismatch")
	ErrPayloadTooLarge  = errors.New("protocol: payload size exceeds limit")
	ErrUnexpectedEOF    = errors.New("protocol: unexpected end of frame") // 区别于 io.EOF
	ErrInvalidVersion   = errors.New("protocol: unsupported version")
)
