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
	AddCallerSkip    int

	// enable the truncation of the level text to 4 characters.
	EnableLevelTruncation bool
	EnableErrorStacktrace bool
	TimestampFormat       string
	EnableCapitalLevel    bool
	atomicLevel           zap.AtomicLevel
	Name                  string
	Fields                []zap.Field

	*lumberjack.Logger
}

func init() {
	config := &logConfig{
		Logger: &lumberjack.Logger{
			LocalTime: true,
		},
	}

	logger = config.newLogger().Sugar()
}

func (config *logConfig) Init() {
	logger = config.newLogger().Sugar()
}

func (config *logConfig) New() *zap.SugaredLogger {
	return config.newLogger().Sugar()
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

func (config *logConfig) newLogger() *zap.Logger {
	if config.CrashLogFilename != "" {
		writeCrashLog(config.CrashLogFilename)
	}

	config.atomicLevel = zap.NewAtomicLevelAt(config.Level)
	config.initColor()

	cores := []zapcore.Core{
		zapcore.NewCore(
			encoder.NewTextEncoder(config.encoderConfig()),
			zapcore.Lock(os.Stdout),
			config.atomicLevel,
		)}

	if config.Filename != "" {
		cores = append(cores, config.fileCore())
	}

	if config.ErrorLogFilename != "" {
		cores = append(cores, config.errorFileCore())
	}

	core := zapcore.NewTee(cores...)

	var options []zap.Option

	if config.EnableLineNumber {
		options = append(options, zap.AddCaller(), zap.AddCallerSkip(config.AddCallerSkip))
	}

	if config.EnableErrorStacktrace {
		options = append(options, zap.AddStacktrace(zapcore.ErrorLevel))
	}

	zapLog := zap.New(core, options...)

	if config.Name != "" {
		zapLog = zapLog.Named(fmt.Sprintf("[%s]", config.Name))
	}

	return zapLog.With(config.Fields...)
}

func (config *logConfig) encoderConfig() zapcore.EncoderConfig {
	el := config.encodeLevel
	if config.EnableColors {
		el = config.encodeColorLevel
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
			if config.TimestampFormat != "" {
				tf = config.TimestampFormat
			}
			enc.AppendString(t.Format(fmt.Sprintf("[%s]", tf)))
		},
		EncodeDuration: zapcore.StringDurationEncoder,
		EncodeCaller: func(caller zapcore.EntryCaller, enc zapcore.PrimitiveArrayEncoder) {
			enc.AppendString(trimCallerFilePath(caller))
		},
	}
}

func (config *logConfig) fileCore() zapcore.Core {
	return zapcore.NewCore(
		encoder.NewTextEncoder(config.encoderConfig()),
		// zapcore.NewMultiWriteSyncer(
		// 	zapcore.Lock(os.Stdout),
		// 	config.fileWriteSyncer(),
		// ),
		config.fileWriteSyncer(config.Filename),
		config.atomicLevel,
	)
}

func (config *logConfig) errorFileCore() zapcore.Core {
	return zapcore.NewCore(
		encoder.NewTextEncoder(config.encoderConfig()),

		config.fileWriteSyncer(config.ErrorLogFilename),
		zap.LevelEnablerFunc(func(lvl zapcore.Level) bool {
			return lvl >= zapcore.ErrorLevel
		}),
	)
}

func (config *logConfig) encodeLevel(l zapcore.Level, enc zapcore.PrimitiveArrayEncoder) {
	levelString := l.CapitalString()

	if config.EnableLevelTruncation {
		levelString = levelString[:4]
	}

	enc.AppendString(fmt.Sprintf("[%s]", levelString))
}

func (config *logConfig) encodeColorLevel(l zapcore.Level, enc zapcore.PrimitiveArrayEncoder) {
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

func (config *logConfig) fileWriteSyncer(fileName string) zapcore.WriteSyncer {
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
		MaxSize:          config.MaxSize, // 单个日志文件大小（MB）
		MaxBackups:       config.MaxBackups,
		MaxAge:           config.MaxAge, // 保留多少天的日志
		LocalTime:        config.LocalTime,
		Compress:         config.Compress,
		BackupTimeFormat: config.BackupTimeFormat,
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

func With(l *zap.SugaredLogger, args ...interface{}) *zap.SugaredLogger {
	return l.With(args...)
}
