package log

import (
	"fmt"

	"go.uber.org/zap/zapcore"
)

var (
	_levelToColor = map[zapcore.Level]Color{
		zapcore.DebugLevel:  Magenta,
		zapcore.InfoLevel:   Blue,
		zapcore.WarnLevel:   Yellow,
		zapcore.ErrorLevel:  Red,
		zapcore.DPanicLevel: Red,
		zapcore.PanicLevel:  Red,
		zapcore.FatalLevel:  Red,
	}
	_unknownLevelColor = Red

	_levelToColorStrings = make(map[zapcore.Level]string, len(_levelToColor))
)

type Color uint8

//noinspection GoUnusedConst
const (
	Black Color = iota + 30
	Red
	Green
	Yellow
	Blue
	Magenta
	Cyan
	White
)

func (config *logConfig) initColor() {
	for level, color := range _levelToColor {
		lcs := level.String()

		if config.EnableCapitalLevel {
			lcs = level.CapitalString()
		}

		if config.EnableLevelTruncation {
			lcs = lcs[:4]
		}
		_levelToColorStrings[level] = color.Add(lcs)
	}
}

func (c Color) Add(s string) string {
	return fmt.Sprintf("\x1b[%dm%s\x1b[0m", uint8(c), s)
}
