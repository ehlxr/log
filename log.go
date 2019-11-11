package log

import (
	"fmt"
	"github.com/ehlxr/lumberjack"
	"log"
	"os"
	"path"
	"strings"
	"time"

	"github.com/ehlxr/log/bufferpool"
	"github.com/ehlxr/log/crash"
	"github.com/ehlxr/log/encoder"
	"github.com/robfig/cron/v3"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

var logger *zap.SugaredLogger

const (
	DebugLevel  = zapcore.DebugLevel
	InfoLevel   = zapcore.InfoLevel
	WarnLevel   = zapcore.WarnLevel
	ErrorLevel  = zapcore.ErrorLevel
	DPanicLevel = zapcore.DPanicLevel
	PanicLevel  = zapcore.PanicLevel
	FatalLevel  = zapcore.FatalLevel
)

type logConfig struct {
	Level            zapcore.Level
	EnableColors     bool
	CrashLogFilename string
	ErrorLogFilename string
	EnableLineNumber bool

	// enable the truncation of the level text to 4 characters.
	EnableLevelTruncation bool
	EnableErrorStacktrace bool
	TimestampFormat       string
	EnableCapitalLevel    bool
	atomicLevel           zap.AtomicLevel
	Name                  string

	*lumberjack.Logger
}

func init() {
	lc := &logConfig{
		Logger: &lumberjack.Logger{
			LocalTime: true,
		},
	}

	logger = lc.newLogger().Sugar()
}

func (lc *logConfig) Init() {
	logger = lc.newLogger().Sugar()
}

func NewLogConfig() *logConfig {
	return &logConfig{
		Level:                 DebugLevel,
		EnableColors:          true,
		CrashLogFilename:      "./logs/crash.log",
		ErrorLogFilename:      "./logs/error.log",
		EnableLineNumber:      true,
		EnableLevelTruncation: true,
		EnableErrorStacktrace: true,
		TimestampFormat:       "2006-01-02 15:04:05.000",
		EnableCapitalLevel:    true,
		Logger: &lumberjack.Logger{
			Filename:         "./logs/log.log",
			MaxSize:          200,
			MaxAge:           0,
			MaxBackups:       30,
			LocalTime:        true,
			Compress:         false,
			BackupTimeFormat: "2006-01-02",
		},
	}
}

func (lc *logConfig) newLogger() *zap.Logger {
	if lc.CrashLogFilename != "" {
		writeCrashLog(lc.CrashLogFilename)
	}

	lc.atomicLevel = zap.NewAtomicLevelAt(lc.Level)
	lc.initColor()

	cores := []zapcore.Core{
		zapcore.NewCore(
			encoder.NewTextEncoder(lc.encoderConfig()),
			zapcore.Lock(os.Stdout),
			lc.atomicLevel,
		)}

	if lc.Filename != "" {
		cores = append(cores, lc.fileCore())
	}

	if lc.ErrorLogFilename != "" {
		cores = append(cores, lc.errorFileCore())
	}

	core := zapcore.NewTee(cores...)

	var options []zap.Option

	if lc.EnableLineNumber {
		options = append(options, zap.AddCaller(), zap.AddCallerSkip(1))
	}

	if lc.EnableErrorStacktrace {
		options = append(options, zap.AddStacktrace(zapcore.ErrorLevel))
	}

	zapLog := zap.New(core, options...)

	if lc.Name != "" {
		zapLog = zapLog.Named(fmt.Sprintf("[%s]", lc.Name))
	}

	return zapLog
}

func (lc *logConfig) encoderConfig() zapcore.EncoderConfig {
	el := lc.encodeLevel
	if lc.EnableColors {
		el = lc.encodeColorLevel
	}

	return zapcore.EncoderConfig{
		TimeKey:       "T",
		LevelKey:      "L",
		NameKey:       "N",
		CallerKey:     "C",
		MessageKey:    "M",
		StacktraceKey: "S",
		LineEnding:    zapcore.DefaultLineEnding,
		EncodeLevel:   el,
		EncodeTime: func(t time.Time, enc zapcore.PrimitiveArrayEncoder) {
			tf := time.RFC3339
			if lc.TimestampFormat != "" {
				tf = lc.TimestampFormat
			}
			enc.AppendString(t.Format(fmt.Sprintf("[%s]", tf)))
		},
		EncodeDuration: zapcore.StringDurationEncoder,
		EncodeCaller: func(caller zapcore.EntryCaller, enc zapcore.PrimitiveArrayEncoder) {
			enc.AppendString(trimCallerFilePath(caller))
		},
	}
}

func (lc *logConfig) fileCore() zapcore.Core {
	return zapcore.NewCore(
		encoder.NewTextEncoder(lc.encoderConfig()),
		// zapcore.NewMultiWriteSyncer(
		// 	zapcore.Lock(os.Stdout),
		// 	lc.fileWriteSyncer(),
		// ),
		lc.fileWriteSyncer(lc.Filename),
		lc.atomicLevel,
	)
}

func (lc *logConfig) errorFileCore() zapcore.Core {
	return zapcore.NewCore(
		encoder.NewTextEncoder(lc.encoderConfig()),

		lc.fileWriteSyncer(lc.ErrorLogFilename),
		zap.LevelEnablerFunc(func(lvl zapcore.Level) bool {
			return lvl >= zapcore.ErrorLevel
		}),
	)
}

func (lc *logConfig) encodeLevel(l zapcore.Level, enc zapcore.PrimitiveArrayEncoder) {
	levelString := l.CapitalString()

	if lc.EnableLevelTruncation {
		levelString = levelString[:4]
	}

	enc.AppendString(fmt.Sprintf("[%s]", levelString))
}

func (lc *logConfig) encodeColorLevel(l zapcore.Level, enc zapcore.PrimitiveArrayEncoder) {
	s, ok := _levelToColorStrings[l]
	if !ok {
		s = _unknownLevelColor.Add(l.CapitalString())
	}

	enc.AppendString(fmt.Sprintf("[%s]", s))
}

func trimCallerFilePath(ec zapcore.EntryCaller) string {
	if !ec.Defined {
		return "undefined"
	}
	// Find the last separator.
	idx := strings.LastIndexByte(ec.File, '/')
	if idx == -1 {
		return ec.FullPath()
	}

	buf := bufferpool.Get()
	buf.AppendString(ec.File[idx+1:])
	buf.AppendByte(':')
	buf.AppendInt(int64(ec.Line))
	caller := buf.String()
	buf.Free()

	return fmt.Sprintf(" %s", caller)
}

func (lc *logConfig) fileWriteSyncer(fileName string) zapcore.WriteSyncer {
	// go get github.com/lestrrat-go/file-rotatelogs
	// writer, err := rotatelogs.New(
	// 	name+".%Y%m%d",
	// 	rotatelogs.WithLinkName(name),             // 生成软链，指向最新日志文件
	// 	rotatelogs.WithMaxAge(7*24*time.Hour),     // 文件最大保存时间
	// 	rotatelogs.WithRotationTime(24*time.Hour), // 日志切割时间间隔
	// )
	// if err != nil {
	// 	log.Fatalf("config normal logger file error. %v", errors.WithStack(err))
	// }

	writer := &lumberjack.Logger{
		Filename:         fileName,
		MaxSize:          lc.MaxSize, // 单个日志文件大小（MB）
		MaxBackups:       lc.MaxBackups,
		MaxAge:           lc.MaxAge, // 保留多少天的日志
		LocalTime:        lc.LocalTime,
		Compress:         lc.Compress,
		BackupTimeFormat: lc.BackupTimeFormat,
	}

	// Rotating log files daily
	runner := cron.New(cron.WithSeconds(), cron.WithLocation(time.Local))
	_, _ = runner.AddFunc("0 0 0 * * ?", func() {
		_ = writer.Rotate(time.Now().AddDate(0, 0, -1))
	})
	go runner.Run()

	return zapcore.AddSync(writer)
}

func writeCrashLog(file string) {
	err := os.MkdirAll(path.Dir(file), os.ModePerm)
	if err != nil {
		log.Fatalf("make crash log dir error. %v",
			err)
	}

	crash.NewCrashLog(file)
}

func Fields(args ...interface{}) {
	logger = logger.With(args...)
}
