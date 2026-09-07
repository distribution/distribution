package s3

import (
	"context"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/smithy-go/logging"
	"github.com/distribution/distribution/v3/internal/dcontext"
)

type sdkLogger struct {
	logger dcontext.Logger
}

func newSDKLogger(ctx context.Context, mode aws.ClientLogMode) logging.Logger {
	if mode == 0 {
		return logging.Nop{}
	}
	return sdkLogger{}.WithContext(ctx)
}

func (sdkLogger) WithContext(ctx context.Context) logging.Logger {
	return sdkLogger{logger: dcontext.GetLoggerWithField(ctx, "storage.driver", driverName)}
}

func (l sdkLogger) Logf(classification logging.Classification, format string, args ...any) {
	switch classification {
	case logging.Warn:
		l.logger.Warnf(format, args...)
	case logging.Debug:
		l.logger.Debugf(format, args...)
	}
}
