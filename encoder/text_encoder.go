package encoder

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"math"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/ehlxr/log/bufferpool"

	"go.uber.org/zap/buffer"
	"go.uber.org/zap/zapcore"
)

const _hex = "0123456789abcdef"

var (
	_arrayEncoderPool = sync.Pool{
		New: func() interface{} {
			return &arrayEncoder{elems: make([]interface{}, 0, 2)}
		}}

	_textPool = sync.Pool{New: func() interface{} {
		return &textEncoder{}
	}}
)

func getTextEncoder() *textEncoder {
	return _textPool.Get().(*textEncoder)
}
func getSliceEncoder() *arrayEncoder {
	return _arrayEncoderPool.Get().(*arrayEncoder)
}

func putSliceEncoder(e *arrayEncoder) {
	e.elems = e.elems[:0]
	_arrayEncoderPool.Put(e)
}

type textEncoder struct {
	*zapcore.EncoderConfig
	buf            *buffer.Buffer
	spaced         bool // include spaces after colons and commas
	openNamespaces int

	// for encoding generic values by reflection
	reflectBuf *buffer.Buffer
	reflectEnc *json.Encoder
}

func NewTextEncoder(cfg zapcore.EncoderConfig) zapcore.Encoder {
	return &textEncoder{
		EncoderConfig: &cfg,
		buf:           bufferpool.Get(),
		spaced:        true,
	}
}

func (enc textEncoder) EncodeEntry(ent zapcore.Entry, fields []zapcore.Field) (*buffer.Buffer, error) {
	line := bufferpool.Get()

	// If this ever becomes a performance bottleneck, we can implement
	// ArrayEncoder for our plain-text format.
	arr := getSliceEncoder()
	if enc.TimeKey != "" && enc.EncodeTime != nil {
		enc.EncodeTime(ent.Time, arr)
	}

	if enc.LevelKey != "" && enc.EncodeLevel != nil {
		enc.EncodeLevel(ent.Level, arr)
	}

	if ent.LoggerName != "" && enc.NameKey != "" {
		nameEncoder := enc.EncodeName
		if nameEncoder == nil {
			// Fall back to FullNameEncoder for backward compatibility.
			nameEncoder = zapcore.FullNameEncoder
		}

		nameEncoder(ent.LoggerName, arr)
	}

	if ent.Caller.Defined && enc.CallerKey != "" && enc.EncodeCaller != nil {
		enc.EncodeCaller(ent.Caller, arr)
	}

	for i := range arr.elems {
		// if i > 0 {
		// line.AppendByte('\t')
		// }
		_, _ = fmt.Fprint(line, arr.elems[i])
	}

	putSliceEncoder(arr)

	// Add any structured context.
	enc.writeContext(line, fields)

	// Add the message itself.
	if enc.MessageKey != "" {
		// c.addTabIfNecessary(line)
		line.AppendString(" - ")
		line.AppendString(ent.Message)
	}

	// If there's no stacktrace key, honor that; this allows users to force
	// single-line output.
	if ent.Stack != "" && enc.StacktraceKey != "" {
		line.AppendByte('\n')
		line.AppendString(ent.Stack)
	}

	if enc.LineEnding != "" {
		line.AppendString(enc.LineEnding)
	} else {
		line.AppendString(zapcore.DefaultLineEnding)
	}

	return line, nil
}

func (enc textEncoder) writeContext(line *buffer.Buffer, extra []zapcore.Field) {
	context := enc.Clone().(*textEncoder)
	defer context.buf.Free()

	addFields(context, extra)
	// context.closeOpenNamespaces()
	if context.buf.Len() == 0 {
		return
	}

	// c.addTabIfNecessary(line)
	// line.AppendByte('{')
	line.AppendByte(' ')
	_, _ = line.Write(context.buf.Bytes())
	// line.AppendByte('}')
}

func (enc textEncoder) addTabIfNecessary(line *buffer.Buffer) {
	if line.Len() > 0 {
		line.AppendByte('\t')
	}
}

func (enc *textEncoder) resetReflectBuf() {
	if enc.reflectBuf == nil {
		enc.reflectBuf = bufferpool.Get()
		enc.reflectEnc = json.NewEncoder(enc.reflectBuf)

		// For consistency with our custom JSON encoder.
		enc.reflectEnc.SetEscapeHTML(false)
	} else {
		enc.reflectBuf.Reset()
	}
}

func (enc *textEncoder) AddArray(key string, arr zapcore.ArrayMarshaler) error {
	enc.addKey(key)

	enc.addElementSeparator()
	enc.buf.AppendByte('[')
	// TODO:
	// err := arr.MarshalLogArray(enc)
	enc.buf.AppendByte(']')
	// return err
	return nil
}
func (enc *textEncoder) AddObject(key string, obj zapcore.ObjectMarshaler) error {
	enc.addKey(key)
	enc.addElementSeparator()
	enc.buf.AppendByte('{')
	err := obj.MarshalLogObject(enc)
	enc.buf.AppendByte('}')
	return err
}
func (enc *textEncoder) AddBinary(key string, val []byte) {
	enc.AddString(key, base64.StdEncoding.EncodeToString(val))
}
func (enc *textEncoder) AddByteString(key string, val []byte) {
	enc.addKey(key)
	enc.addElementSeparator()
	enc.buf.AppendByte('"')
	enc.safeAddByteString(val)
	enc.buf.AppendByte('"')
}
func (enc *textEncoder) AddBool(key string, val bool) {
	enc.addKey(key)
	enc.addElementSeparator()
	enc.buf.AppendBool(val)
}

//noinspection GoRedundantConversion
func (enc *textEncoder) AddComplex128(key string, val complex128) {
	enc.addKey(key)
	enc.addElementSeparator()
	// Cast to a platform-independent, fixed-size type.
	r, i := float64(real(val)), float64(imag(val))
	enc.buf.AppendByte('"')
	// Because we're always in a quoted string, we can use strconv without
	// special-casing NaN and +/-Inf.
	enc.buf.AppendFloat(r, 64)
	enc.buf.AppendByte('+')
	enc.buf.AppendFloat(i, 64)
	enc.buf.AppendByte('i')
	enc.buf.AppendByte('"')
}
func (enc *textEncoder) AddDuration(key string, val time.Duration) {
	enc.addKey(key)
	cur := enc.buf.Len()
	// TODO:
	// enc.EncodeDuration(val, enc)
	if cur == enc.buf.Len() {
		// User-supplied EncodeDuration is a no-op. Fall back to nanoseconds to keep
		// JSON valid.
		enc.addElementSeparator()
		enc.buf.AppendInt(int64(val))
	}
}
func (enc *textEncoder) AddFloat64(key string, val float64) {
	enc.addKey(key)
	enc.appendFloat(val, 64)
	enc.buf.AppendByte(']')
}
func (enc *textEncoder) AddInt64(key string, val int64) {
	enc.addKey(key)
	enc.addElementSeparator()
	enc.buf.AppendInt(val)
	enc.buf.AppendByte(']')
}
func (enc *textEncoder) AddReflected(key string, obj interface{}) error {
	enc.resetReflectBuf()
	err := enc.reflectEnc.Encode(obj)
	if err != nil {
		return err
	}
	enc.reflectBuf.TrimNewline()
	enc.addKey(key)
	_, err = enc.buf.Write(enc.reflectBuf.Bytes())
	return err
}
func (enc *textEncoder) OpenNamespace(key string) {
	enc.addKey(key)
	enc.buf.AppendByte('{')
	enc.openNamespaces++
}
func (enc *textEncoder) AddString(key, val string) {
	enc.addKey(key)
	enc.addElementSeparator()
	// enc.buf.AppendByte('"')
	enc.safeAddString(val)
	// enc.buf.AppendByte('"')
	enc.buf.AppendByte(']')
}
func (enc *textEncoder) AddTime(key string, val time.Time) {
	enc.addKey(key)
	cur := enc.buf.Len()
	// TODO:
	// enc.EncodeTime(val, enc)
	if cur == enc.buf.Len() {
		// User-supplied EncodeTime is a no-op. Fall back to nanos since epoch to keep
		// output JSON valid.
		enc.addElementSeparator()
		enc.buf.AppendInt(val.UnixNano())
	}
}
func (enc *textEncoder) AddUint64(key string, val uint64) {
	enc.addKey(key)
	enc.addElementSeparator()
	enc.buf.AppendUint(val)
	enc.buf.AppendByte(']')
}

//noinspection GoRedundantConversion
func (enc *textEncoder) AddComplex64(k string, v complex64) { enc.AddComplex128(k, complex128(v)) }
func (enc *textEncoder) AddFloat32(k string, v float32)     { enc.AddFloat64(k, float64(v)) }
func (enc *textEncoder) AddInt(k string, v int)             { enc.AddInt64(k, int64(v)) }
func (enc *textEncoder) AddInt32(k string, v int32)         { enc.AddInt64(k, int64(v)) }
func (enc *textEncoder) AddInt16(k string, v int16)         { enc.AddInt64(k, int64(v)) }
func (enc *textEncoder) AddInt8(k string, v int8)           { enc.AddInt64(k, int64(v)) }
func (enc *textEncoder) AddUint(k string, v uint)           { enc.AddUint64(k, uint64(v)) }
func (enc *textEncoder) AddUint32(k string, v uint32)       { enc.AddUint64(k, uint64(v)) }
func (enc *textEncoder) AddUint16(k string, v uint16)       { enc.AddUint64(k, uint64(v)) }
func (enc *textEncoder) AddUint8(k string, v uint8)         { enc.AddUint64(k, uint64(v)) }
func (enc *textEncoder) AddUintptr(k string, v uintptr)     { enc.AddUint64(k, uint64(v)) }

func (enc *textEncoder) Clone() zapcore.Encoder {
	clone := enc.clone()
	_, _ = clone.buf.Write(enc.buf.Bytes())
	return clone
}

func (enc *textEncoder) clone() *textEncoder {
	clone := getTextEncoder()
	clone.EncoderConfig = enc.EncoderConfig
	clone.spaced = enc.spaced
	clone.openNamespaces = enc.openNamespaces
	clone.buf = bufferpool.Get()
	return clone
}

func (enc *textEncoder) addKey(key string) {
	enc.addElementSeparator()
	// enc.buf.AppendByte('"')
	enc.buf.AppendByte('[')
	enc.safeAddString(key)
	// enc.buf.AppendByte('"')
	enc.buf.AppendByte(':')
	if enc.spaced {
		enc.buf.AppendByte(' ')
	}
}

func (enc *textEncoder) addElementSeparator() {
	last := enc.buf.Len() - 1
	if last < 0 {
		return
	}
	switch enc.buf.Bytes()[last] {
	case '{', '[', ':', ',', ' ':
		return
	default:
		enc.buf.AppendByte(',')
		if enc.spaced {
			enc.buf.AppendByte(' ')
		}
	}
}

func (enc *textEncoder) appendFloat(val float64, bitSize int) {
	enc.addElementSeparator()
	switch {
	case math.IsNaN(val):
		enc.buf.AppendString(`"NaN"`)
	case math.IsInf(val, 1):
		enc.buf.AppendString(`"+Inf"`)
	case math.IsInf(val, -1):
		enc.buf.AppendString(`"-Inf"`)
	default:
		enc.buf.AppendFloat(val, bitSize)
	}
}

func (enc *textEncoder) safeAddString(s string) {
	for i := 0; i < len(s); {
		if enc.tryAddRuneSelf(s[i]) {
			i++
			continue
		}
		r, size := utf8.DecodeRuneInString(s[i:])
		if enc.tryAddRuneError(r, size) {
			i++
			continue
		}
		enc.buf.AppendString(s[i : i+size])
		i += size
	}
}

func (enc *textEncoder) safeAddByteString(s []byte) {
	for i := 0; i < len(s); {
		if enc.tryAddRuneSelf(s[i]) {
			i++
			continue
		}
		r, size := utf8.DecodeRune(s[i:])
		if enc.tryAddRuneError(r, size) {
			i++
			continue
		}
		_, _ = enc.buf.Write(s[i : i+size])
		i += size
	}
}

func (enc *textEncoder) tryAddRuneSelf(b byte) bool {
	if b >= utf8.RuneSelf {
		return false
	}
	if 0x20 <= b && b != '\\' && b != '"' {
		enc.buf.AppendByte(b)
		return true
	}
	switch b {
	case '\\', '"':
		enc.buf.AppendByte('\\')
		enc.buf.AppendByte(b)
	case '\n':
		enc.buf.AppendByte('\\')
		enc.buf.AppendByte('n')
	case '\r':
		enc.buf.AppendByte('\\')
		enc.buf.AppendByte('r')
	case '\t':
		enc.buf.AppendByte('\\')
		enc.buf.AppendByte('t')
	default:
		// Encode bytes < 0x20, except for the escape sequences above.
		enc.buf.AppendString(`\u00`)
		enc.buf.AppendByte(_hex[b>>4])
		enc.buf.AppendByte(_hex[b&0xF])
	}
	return true
}

func (enc *textEncoder) tryAddRuneError(r rune, size int) bool {
	if r == utf8.RuneError && size == 1 {
		enc.buf.AppendString(`\ufffd`)
		return true
	}
	return false
}

func addFields(enc zapcore.ObjectEncoder, fields []zapcore.Field) {
	for i := range fields {
		fields[i].AddTo(enc)
	}
}

type arrayEncoder struct {
	elems []interface{}
}

func (s *arrayEncoder) AppendArray(v zapcore.ArrayMarshaler) error {
	enc := &arrayEncoder{}
	err := v.MarshalLogArray(enc)
	s.elems = append(s.elems, enc.elems)

	return err
}

func (s *arrayEncoder) AppendObject(v zapcore.ObjectMarshaler) error {
	m := zapcore.NewMapObjectEncoder()
	err := v.MarshalLogObject(m)
	s.elems = append(s.elems, m.Fields)
	return err
}

func (s *arrayEncoder) AppendReflected(v interface{}) error {
	s.elems = append(s.elems, v)
	return nil
}

func (s *arrayEncoder) AppendBool(v bool)              { s.elems = append(s.elems, v) }
func (s *arrayEncoder) AppendByteString(v []byte)      { s.elems = append(s.elems, string(v)) }
func (s *arrayEncoder) AppendComplex128(v complex128)  { s.elems = append(s.elems, v) }
func (s *arrayEncoder) AppendComplex64(v complex64)    { s.elems = append(s.elems, v) }
func (s *arrayEncoder) AppendDuration(v time.Duration) { s.elems = append(s.elems, v) }
func (s *arrayEncoder) AppendFloat64(v float64)        { s.elems = append(s.elems, v) }
func (s *arrayEncoder) AppendFloat32(v float32)        { s.elems = append(s.elems, v) }
func (s *arrayEncoder) AppendInt(v int)                { s.elems = append(s.elems, v) }
func (s *arrayEncoder) AppendInt64(v int64)            { s.elems = append(s.elems, v) }
func (s *arrayEncoder) AppendInt32(v int32)            { s.elems = append(s.elems, v) }
func (s *arrayEncoder) AppendInt16(v int16)            { s.elems = append(s.elems, v) }
func (s *arrayEncoder) AppendInt8(v int8)              { s.elems = append(s.elems, v) }
func (s *arrayEncoder) AppendString(v string)          { s.elems = append(s.elems, v) }
func (s *arrayEncoder) AppendTime(v time.Time)         { s.elems = append(s.elems, v) }
func (s *arrayEncoder) AppendUint(v uint)              { s.elems = append(s.elems, v) }
func (s *arrayEncoder) AppendUint64(v uint64)          { s.elems = append(s.elems, v) }
func (s *arrayEncoder) AppendUint32(v uint32)          { s.elems = append(s.elems, v) }
func (s *arrayEncoder) AppendUint16(v uint16)          { s.elems = append(s.elems, v) }
func (s *arrayEncoder) AppendUint8(v uint8)            { s.elems = append(s.elems, v) }
func (s *arrayEncoder) AppendUintptr(v uintptr)        { s.elems = append(s.elems, v) }
