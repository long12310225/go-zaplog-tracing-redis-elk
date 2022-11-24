package zaplog

import (
	"context"
	"os"
	"strings"
	"time"

	"github.com/go-redis/redis/v8"
	"github.com/long12310225/go-zaplog-tracing-redis-elk/conf"
	"github.com/long12310225/go-zaplog-tracing-redis-elk/fileout"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

type Log struct {
	logger *zap.Logger
}

//var Log *zap.Logger //全局日志

func parseLevel(lvl string) zapcore.Level {
	switch strings.ToLower(lvl) {
	case "panic", "dpanic":
		return zapcore.PanicLevel
	case "fatal":
		return zapcore.FatalLevel
	case "error":
		return zapcore.ErrorLevel
	case "warn", "warning":
		return zapcore.WarnLevel
	case "info":
		return zapcore.InfoLevel
	case "debug":
		return zapcore.DebugLevel
	default:
		return zapcore.DebugLevel
	}
}
func NewRedisWriter(key string, cli *redis.Client) *RedisWriter {
	return &RedisWriter{
		cli: cli, listKey: key,
	}
}

// 为 logger 提供写入 redis 队列的 io 接口
type RedisWriter struct {
	cli     *redis.Client
	listKey string
}

func (w *RedisWriter) Write(p []byte) (int, error) {
	n, err := w.cli.RPush(context.Background(), w.listKey, p).Result()
	w.cli.Expire(context.Background(), w.listKey, 24*time.Hour)
	return int(n), err
}
func getRedisWriter(redisAddr, redisPass string, redisDB int) zapcore.WriteSyncer {
	cli := redis.NewClient(&redis.Options{
		Addr:     redisAddr,
		Password: redisPass,
		DB:       redisDB,
	})
	writerRedis := NewRedisWriter("ELK_LOG", cli)
	return zapcore.AddSync(writerRedis)
}

// 创建日志
func New(opts ...conf.Option) *Log {
	o := &conf.Options{
		LogPath:     conf.LogPath,
		LogName:     conf.LogName,
		LogLevel:    conf.LogLevel,
		MaxSize:     conf.MaxSize,
		MaxAge:      conf.MaxAge,
		Stacktrace:  conf.Stacktrace,
		IsStdOut:    conf.IsStdOut,
		ProjectName: conf.ProjectName,
		RedisAddr:   conf.RedisAddr,
		RedisPass:   conf.RedisPass,
		RedisDB:     conf.RedisDB,
	}
	for _, opt := range opts {
		opt(o)
	}
	writers := []zapcore.WriteSyncer{fileout.NewRollingFile(o.LogPath, o.LogName, o.MaxSize, o.MaxAge)}
	if o.IsStdOut == "yes" {
		writers = append(writers, os.Stdout)
	}
	if len(o.RedisAddr) > 0 {
		writers = append(writers, getRedisWriter(o.RedisAddr, o.RedisPass, o.RedisDB))
	}
	logger := newZapLogger(parseLevel(o.LogLevel), parseLevel(o.Stacktrace), zapcore.NewMultiWriteSyncer(writers...))
	zap.RedirectStdLog(logger)
	logger = logger.With(zap.String("project", o.ProjectName)) //加上项目名称
	return &Log{logger: logger}
}

func newZapLogger(level, stacktrace zapcore.Level, output zapcore.WriteSyncer) *zap.Logger {
	encCfg := zapcore.EncoderConfig{
		TimeKey:        "@timestamp",
		LevelKey:       "level",
		NameKey:        "app",
		CallerKey:      "caller",
		MessageKey:     "msg",
		StacktraceKey:  "stacktrace",
		LineEnding:     zapcore.DefaultLineEnding,
		EncodeCaller:   zapcore.ShortCallerEncoder,
		EncodeDuration: zapcore.NanosDurationEncoder,
		//EncodeTime: func(t time.Time, enc zapcore.PrimitiveArrayEncoder) {
		//	enc.AppendString(t.Format("2006-01-02 15:04:05.000"))
		//},
		EncodeTime: zapcore.ISO8601TimeEncoder,
	}

	var encoder zapcore.Encoder
	dyn := zap.NewAtomicLevel()
	//encCfg.EncodeLevel = zapcore.LowercaseLevelEncoder
	//encoder = zapcore.NewJSONEncoder(encCfg) // zapcore.NewConsoleEncoder(encCfg)
	dyn.SetLevel(level)
	encCfg.EncodeLevel = zapcore.LowercaseLevelEncoder
	encoder = zapcore.NewJSONEncoder(encCfg)

	return zap.New(zapcore.NewCore(encoder, output, dyn), zap.AddCaller(), zap.AddStacktrace(stacktrace), zap.AddCallerSkip(2))
}
